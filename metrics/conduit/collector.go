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

package conduit

import (
	"fmt"
	"log/slog"

	"github.com/conduitio/benchi/metrics"
	"github.com/conduitio/benchi/metrics/prometheus"
	"github.com/go-viper/mapstructure/v2"
)

const Type = "conduit"

// Register registers the Conduit collector with the metrics system.
// This function should be called explicitly by the application.
func Register() {
	metrics.RegisterCollector(NewCollector)
}

type Collector struct {
	prometheus.Collector
}

func (c *Collector) Type() string {
	return Type
}

func (c *Collector) Configure(settings map[string]any) error {
	cfg, err := c.parseConfig(settings)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Remove pipelines from settings, the prometheus collector doesn't know about
	// this key.
	delete(settings, "pipelines")

	var queries []map[string]any
	for _, pipeline := range cfg.Pipelines {
		queries = append(queries, []map[string]any{
			{
				"name":     fmt.Sprintf("msg-rate-per-second[%s]", pipeline),
				"query":    fmt.Sprintf("rate(conduit_pipeline_execution_duration_seconds_count{pipeline_name=%q}[2s])", pipeline),
				"unit":     "msg/s",
				"interval": "1s",
			},
			{
				"name":     fmt.Sprintf("msg-megabytes-in-per-second[%s]", pipeline),
				"query":    fmt.Sprintf(`rate(conduit_connector_bytes_sum{pipeline_name=%q,type="source"}[2s])/1048576`, pipeline),
				"unit":     "MB/s",
				"interval": "1s",
			},
			{
				"name":     fmt.Sprintf("msg-megabytes-out-per-second[%s]", pipeline),
				"query":    fmt.Sprintf(`rate(conduit_connector_bytes_sum{pipeline_name=%q,type="destination"}[2s])/1048576`, pipeline),
				"unit":     "MB/s",
				"interval": "1s",
			},
		}...)
	}

	if settings["queries"] != nil {
		settingsQueries, ok := settings["queries"].([]map[string]any)
		if ok {
			queries = append(queries, settingsQueries...)
		}
	}
	settings["queries"] = queries

	//nolint:wrapcheck // The prometheus collector is responsible for wrapping the error.
	return c.Collector.Configure(settings)
}

func (c *Collector) parseConfig(settings map[string]any) (Config, error) {
	var cfg Config
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
		ErrorUnused:      false,
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

	return cfg, nil
}

func NewCollector(logger *slog.Logger, name string) *Collector {
	return &Collector{
		Collector: *prometheus.NewCollector(logger, name),
	}
}
