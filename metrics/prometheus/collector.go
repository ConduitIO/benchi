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
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/conduitio/benchi/metrics"
	"github.com/mitchellh/mapstructure"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/sourcegraph/conc/pool"
)

const Type = "prometheus"

// Register registers the Prometheus collector with the metrics registry.
func Register() {
	metrics.RegisterCollector(NewCollector)
}

type Collector struct {
	logger *slog.Logger
	name   string

	cfg      Config
	runStart time.Time

	tsdb          *tsdb.DB
	scrapeManager *scrape.Manager
	promqlEngine  *promql.Engine

	mu      sync.Mutex
	results map[string]promql.Matrix
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

func (p *Collector) Metrics() map[string][]metrics.Metric {
	out := make(map[string][]metrics.Metric)

	p.mu.Lock()
	defer p.mu.Unlock()
	for name, m := range p.results {
		out[name] = p.promqlMatrixToMetrics(m)
	}
	return out
}

func (p *Collector) Configure(settings map[string]any) (err error) {
	cfg, err := p.parseConfig(settings)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	registry := prometheus.NewRegistry()
	promCfg := p.prometheusConfig(cfg)

	db, dbCleanup, err := p.initTsdb(registry)
	if err != nil {
		return fmt.Errorf("failed to initialize tsdb: %w", err)
	}
	defer dbCleanup()

	scrapeManager, err := p.initScrapeManager(registry, promCfg, db)
	if err != nil {
		return fmt.Errorf("failed to initialize scrape manager: %w", err)
	}

	promqlEngine := p.initPromqlEngine(registry, cfg, promCfg)
	err = p.validateQueries(promqlEngine, db, cfg.Queries)
	if err != nil {
		return fmt.Errorf("failed to validate queries: %w", err)
	}

	p.cfg = cfg
	p.tsdb = db
	p.scrapeManager = scrapeManager
	p.promqlEngine = promqlEngine
	p.results = make(map[string]promql.Matrix)
	for _, queryCfg := range cfg.Queries {
		p.results[queryCfg.Name] = nil
	}

	return nil
}

func (p *Collector) parseConfig(settings map[string]any) (Config, error) {
	cfg := defaultConfig
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
		ErrorUnused:      true,
		WeaklyTypedInput: true,
		Result:           &cfg,
		TagName:          "yaml",
	})
	if err != nil {
		return Config{}, fmt.Errorf("failed to create decoder: %w", err)
	}

	err = dec.Decode(settings)
	if err != nil {
		return Config{}, fmt.Errorf("failed to decode settings: %w", err)
	}

	// Try parsing the URL to ensure it's valid.
	_, err = cfg.parseURL()
	if err != nil {
		return Config{}, fmt.Errorf("failed to parse URL: %w", err)
	}

	return cfg, nil
}

func (p *Collector) prometheusConfig(cfg Config) *config.Config {
	promCfg, err := config.Load("", p.logger)
	if err != nil {
		panic(err) // Empty config is valid, this error should never occur.
	}

	promCfg.GlobalConfig.ScrapeInterval = model.Duration(cfg.ScrapeInterval)
	promCfg.GlobalConfig.ScrapeTimeout = model.Duration(cfg.ScrapeInterval)

	scrapeCfg := config.DefaultScrapeConfig
	scrapeCfg.JobName = "benchi"
	scrapeCfg.ScrapeInterval = promCfg.GlobalConfig.ScrapeInterval
	scrapeCfg.ScrapeTimeout = promCfg.GlobalConfig.ScrapeTimeout

	promCfg.ScrapeConfigs = append(promCfg.ScrapeConfigs, &scrapeCfg)
	return promCfg
}

func (p *Collector) initTsdb(registry *prometheus.Registry) (*tsdb.DB, func(), error) {
	dataPath, err := os.MkdirTemp("", fmt.Sprintf("*-benchi-prometheus-%s", p.Name()))
	if err != nil {
		return nil, nil, fmt.Errorf("error creating temporary directory: %w", err)
	}

	db, err := tsdb.Open(
		dataPath,
		p.logger.With("collector.prometheus", "tsdb"),
		registry,
		tsdb.DefaultOptions(),
		nil,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("error opening storage: %w", err)
	}
	return db, func() {
		if err != nil {
			if dbErr := db.Close(); dbErr != nil {
				p.logger.Error("Failed to close storage", "err", dbErr)
			}
		}
	}, nil
}

func (p *Collector) initScrapeManager(registry *prometheus.Registry, promCfg *config.Config, db storage.Appendable) (*scrape.Manager, error) {
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
		return nil, fmt.Errorf("error creating scrape manager: %w", err)
	}

	err = scrapeManager.ApplyConfig(promCfg)
	if err != nil {
		return nil, fmt.Errorf("error applying config: %w", err)
	}

	return scrapeManager, nil
}

func (p *Collector) initPromqlEngine(registry *prometheus.Registry, cfg Config, promCfg *config.Config) *promql.Engine {
	promqlEngineOpts := promql.EngineOpts{
		Logger:             p.logger.With("collector.prometheus", "query engine"),
		Reg:                registry,
		MaxSamples:         50000000,
		Timeout:            cfg.ScrapeInterval*5 + time.Second,
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

	return promqlEngine
}

func (p *Collector) validateQueries(promqlEngine *promql.Engine, db storage.Queryable, queries []QueryConfig) error {
	for _, queryCfg := range queries {
		// Check that the query is valid.
		now := time.Now()
		q, err := promqlEngine.NewRangeQuery(
			context.Background(),
			db,
			promql.NewPrometheusQueryOpts(false, 0),
			queryCfg.QueryString,
			now.Add(-queryCfg.Interval),
			now,
			queryCfg.Interval,
		)
		if err != nil {
			return fmt.Errorf("invalid query %s: %w", queryCfg.Name, err)
		}
		q.Cancel()
		q.Close()
	}
	return nil
}

func (p *Collector) Run(ctx context.Context) error {
	p.runStart = time.Now()

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

	wg := pool.New().WithErrors().WithContext(ctx).WithCancelOnError()
	wg.Go(func(context.Context) error {
		return p.scrapeManager.Run(ch)
	})
	wg.Go(func(ctx context.Context) error {
		<-ctx.Done()
		p.scrapeManager.Stop()
		return nil
	})
	wg.Go(func(ctx context.Context) error {
		ticker := time.NewTicker(p.cfg.ScrapeInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}

			// TODO loop over all queries
			err := p.execQuery(ctx, p.cfg.Queries[0])
			if err != nil {
				return fmt.Errorf("error executing prometheus query: %w", err)
			}
		}
	})

	err := wg.Wait()

	if err := p.promqlEngine.Close(); err != nil {
		p.logger.Error("Failed to close Prometheus query engine", "error", err)
	}
	if err := p.tsdb.Close(); err != nil {
		p.logger.Error("Failed to close Prometheus storage", "error", err)
	}

	return err //nolint:wrapcheck // Errors are wrapped inside the goroutines.
}

// execQuery executes the query and sends the result to the output channel.
// It returns the query object so that it can be closed when the next query is
// executed.
func (p *Collector) execQuery(ctx context.Context, queryCfg QueryConfig) error {
	q, err := p.promqlEngine.NewRangeQuery(
		ctx,
		p.tsdb,
		promql.NewPrometheusQueryOpts(false, 0),
		queryCfg.QueryString,
		p.runStart,
		time.Now(),
		queryCfg.Interval,
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
		p.logger.Debug("No data returned from query")
		return nil
	}
	if len(m) > 1 {
		// TODO add support for multiple series
		p.logger.Warn("Query returned multiple series, only first will be used", "series-count", len(m))
		m = m[:1]
	}

	p.logger.Debug("Query returned data", "series-count", len(m))

	p.mu.Lock()
	defer p.mu.Unlock()
	p.results[queryCfg.Name] = m

	return nil
}

func (p *Collector) promqlMatrixToMetrics(m promql.Matrix) []metrics.Metric {
	if len(m) == 0 {
		return nil
	}
	series := m[0] // TODO add support for multiple series
	out := make([]metrics.Metric, len(series.Floats))
	for i, sample := range series.Floats {
		out[i] = metrics.Metric{
			At:    time.UnixMilli(sample.T),
			Value: sample.F,
		}
	}
	return out
}
