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
	"log/slog"

	"github.com/conduitio/benchi/metrics"
	"github.com/conduitio/benchi/metrics/prometheus"
)

const Type = "conduit"

func init() { metrics.RegisterCollector(NewCollector) }

type Collector struct {
	prometheus.Collector
}

func (c *Collector) Type() string {
	return Type
}

func (c *Collector) Configure(settings map[string]any) error {
	settings["queries"] = []map[string]any{
		{
			"name":     "msg-rate-per-second",
			"query":    "rate(conduit_pipeline_execution_duration_seconds_count[2s])",
			"interval": "1s",
		},
	}
	return c.Collector.Configure(settings)
}

func NewCollector(logger *slog.Logger, name string) *Collector {
	return &Collector{
		Collector: *prometheus.NewCollector(logger, name),
	}
}
