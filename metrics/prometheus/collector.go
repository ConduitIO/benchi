// Copyright © 2025 Meroxa, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package prometheus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/conduitio/benchi/metrics"
	"github.com/davecgh/go-spew/spew"
	"github.com/mitchellh/mapstructure"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/tsdb"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

const Type = "prometheus"

func init() { metrics.RegisterCollector(NewCollector) }

type Collector struct {
	logger *slog.Logger
	name   string

	cfg      Config
	runStart time.Time
	runEnd   time.Time

	tsdb          *tsdb.DB
	scrapeManager *scrape.Manager
	promqlEngine  *promql.Engine

	mu      sync.Mutex
	results promql.Matrix
}

func NewCollector(logger *slog.Logger, name string) *Collector {
	return &Collector{
		logger: logger,
		name:   name,
	}
}

func (p *Collector) Name() string {
	return p.name
}

func (p *Collector) Type() string {
	return Type
}

func (p *Collector) Stop(context.Context) error {
	var errs []error

	p.scrapeManager.Stop()
	errs = append(errs, p.promqlEngine.Close())
	errs = append(errs, p.tsdb.Close())

	return errors.Join(errs...)
}

func (p *Collector) Flush(_ context.Context, dir string) error {
	f, err := os.Create(filepath.Join(dir, p.Name()+"-prometheus.csv"))
	if err != nil {
		return fmt.Errorf("error creating file: %w", err)
	}
	defer f.Close()

	p.mu.Lock()
	defer p.mu.Unlock()

	// TODO: This is a temporary solution to dump the results to a file.
	spew.Fdump(f, p.results)
	return nil
}

func (p *Collector) Configure(settings map[string]any) (err error) {
	p.cfg = defaultConfig
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
		ErrorUnused:      true,
		WeaklyTypedInput: true,
		Result:           &p.cfg,
		TagName:          "yaml",
	})
	if err != nil {
		return err
	}

	err = dec.Decode(settings)
	if err != nil {
		return err
	}

	registry := prometheus.NewRegistry()

	// Try parsing the URL to ensure it's valid.
	_, err = p.cfg.parseURL()
	if err != nil {
		return fmt.Errorf("error parsing URL: %w", err)
	}

	dataPath, err := os.MkdirTemp("", fmt.Sprintf("*-benchi-prometheus-%s", p.Name()))
	if err != nil {
		return fmt.Errorf("error creating temporary directory: %w", err)
	}

	db, err := tsdb.Open(
		dataPath,
		p.logger.With("collector.prometheus", "tsdb"),
		registry,
		tsdb.DefaultOptions(),
		nil,
	)
	if err != nil {
		return fmt.Errorf("error opening storage: %w", err)
	}
	defer func() {
		if err != nil {
			if dbErr := db.Close(); dbErr != nil {
				p.logger.Error("Failed to close storage", "err", dbErr)
			}
		}
	}()

	scrapeManager, err := scrape.NewManager(
		&scrape.Options{
			// Need to set the reload interval to a small value to ensure that
			// the scrape manager starts scraping immediately and not after 5
			// seconds (default).
			DiscoveryReloadInterval: model.Duration(time.Millisecond * 100),
		},
		p.logger.With("collector.prometheus", "scrape manager"),
		nil,
		db,
		registry,
	)
	if err != nil {
		return fmt.Errorf("error creating scrape manager: %w", err)
	}

	promCfg, err := config.Load(`
global:
  scrape_interval: 1s
scrape_configs:
  - job_name: benchi
`, p.logger)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	err = scrapeManager.ApplyConfig(promCfg)
	if err != nil {
		return fmt.Errorf("error applying config: %w", err)
	}

	promqlEngineOpts := promql.EngineOpts{
		Logger:             p.logger.With("collector.prometheus", "query engine"),
		Reg:                registry,
		MaxSamples:         50000000,
		Timeout:            p.cfg.ScrapeInterval,
		ActiveQueryTracker: NewSequentialQueryTracker(),
		NoStepSubqueryIntervalFn: func(_ int64) int64 {
			return int64(time.Duration(promCfg.GlobalConfig.EvaluationInterval) / time.Millisecond)
		},
		// EnableAtModifier and EnableNegativeOffset have to be
		// always on for regular PromQL as of Prometheus v2.33.
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
		EnablePerStepStats:   false,
	}

	promqlEngine := promql.NewEngine(promqlEngineOpts)
	promqlEngine.SetQueryLogger(QueryLogger{p.logger.Handler()})

	// Check that the query is valid.
	// TODO loop over all queries
	now := time.Now()
	q, err := promqlEngine.NewRangeQuery(
		context.Background(),
		db,
		promql.NewPrometheusQueryOpts(false, 0),
		p.cfg.Queries[0].QueryString,
		now.Add(-p.cfg.Queries[0].Interval),
		now,
		p.cfg.Queries[0].Interval,
	)
	if err != nil {
		return fmt.Errorf("invalid query: %w", err)
	}
	q.Cancel()
	q.Close()

	p.tsdb = db
	p.scrapeManager = scrapeManager
	p.promqlEngine = promqlEngine

	return nil
}

func (p *Collector) Run(ctx context.Context) error {
	p.runStart = time.Now()
	defer func() {
		p.runEnd = time.Now()
	}()

	// Ignore parsing error, we validated it in Configure.
	targetURL, _ := p.cfg.parseURL()

	labels := model.LabelSet{
		model.AddressLabel:     model.LabelValue(targetURL.Host),
		model.SchemeLabel:      model.LabelValue(targetURL.Scheme),
		model.MetricsPathLabel: model.LabelValue(targetURL.Path),
	}

	ch := make(chan map[string][]*targetgroup.Group, 1)
	ch <- map[string][]*targetgroup.Group{
		"benchi": {
			&targetgroup.Group{
				Targets: []model.LabelSet{labels},
				Labels:  model.LabelSet{},
				Source:  "benchi",
			},
		},
	}

	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return p.scrapeManager.Run(ch)
	})
	group.Go(func() error {
		<-ctx.Done()
		p.scrapeManager.Stop()
		if err := p.tsdb.Close(); err != nil {
			return fmt.Errorf("error closing storage: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		rateLimit := rate.NewLimiter(rate.Every(p.cfg.ScrapeInterval), 1)
		for {
			err := rateLimit.Wait(ctx)
			if err != nil {
				return err
			}
			err = p.execQuery(ctx)
			if err != nil {
				return fmt.Errorf("error executing prometheus query: %w", err)
			}
		}
	})

	return group.Wait()
}

// execQuery executes the query and sends the result to the output channel.
// It returns the query object so that it can be closed when the next query is
// executed.
func (p *Collector) execQuery(ctx context.Context) error {
	end := p.runEnd
	if end.IsZero() {
		end = time.Now()
	}
	// TODO maybe do an instant query instead of a range query
	q, err := p.promqlEngine.NewRangeQuery(
		ctx,
		p.tsdb,
		promql.NewPrometheusQueryOpts(false, 0),
		p.cfg.Queries[0].QueryString,
		p.runStart,
		end,
		p.cfg.Queries[0].Interval,
	)
	if err != nil {
		return fmt.Errorf("invalid query: %w", err)
	}
	r := q.Exec(ctx)
	if r.Err != nil {
		return fmt.Errorf("error executing query: %w", r.Err)
	}
	defer q.Close()
	m, err := r.Matrix()
	if err != nil {
		return fmt.Errorf("error fetching result matrix: %w", r.Err)
	}
	if len(m) == 0 {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.results = m

	return nil
}
