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

package kafka

import (
	"fmt"
	"log/slog"

	"github.com/conduitio/benchi/metrics"
	"github.com/conduitio/benchi/metrics/prometheus"
	"github.com/go-viper/mapstructure/v2"
)

const Type = "kafka"

// Register registers the Kafka collector with the metrics system.
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

	// Remove topics from settings, the prometheus collector doesn't know about
	// this key.
	delete(settings, "topics")

	var queries []map[string]any
	for _, topic := range cfg.Topics {
		queries = append(queries, []map[string]any{
			{
				"name":     fmt.Sprintf("msg-bytes-per-second[%s]", topic),
				"query":    fmt.Sprintf("rate(kafka_server_total_bytes_in_per_sec_per_topic{topic=%q}[2s])", topic),
				"unit":     "B/s",
				"interval": "1s",
			},
			{
				"name":     fmt.Sprintf("msg-rate-per-second[%s]", topic),
				"query":    fmt.Sprintf("rate(kafka_server_messages_in_per_sec_per_topic_total{topic=%q}[2s])", topic),
				"unit":     "msg/s",
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
