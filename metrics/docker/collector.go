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

package docker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/conduitio/benchi/metrics"
	"github.com/docker/docker/client"
)

const Type = "docker"

// Register registers the Kafka collector with the metrics system.
// This function should be called explicitly by the application.
func Register(dockerClient client.APIClient) {
	metrics.RegisterCollector(func(logger *slog.Logger, name string) *Collector {
		return NewCollector(logger, name, dockerClient)
	})
}

type Collector struct {
	logger *slog.Logger
	name   string

	cfg              Config
	dockerClient     client.APIClient
	containerIndices map[string]int

	mu      sync.Mutex
	results []metrics.Results
}

func NewCollector(logger *slog.Logger, name string, dockerClient client.APIClient) *Collector {
	return &Collector{
		logger:       logger,
		name:         name,
		dockerClient: dockerClient,
	}
}

func (c *Collector) Name() string {
	return c.name
}

func (c *Collector) Type() string {
	return Type
}

func (c *Collector) Configure(settings map[string]any) error {
	cfg, err := parseConfig(settings)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	c.cfg = cfg

	numMetrics := len(sampleCollectors)
	c.results = make([]metrics.Results, len(cfg.Containers)*numMetrics)
	for i, name := range cfg.Containers {
		for j, sc := range sampleCollectors {
			c.results[i*numMetrics+j].Name = sc.name(name)
			c.results[i*numMetrics+j].Unit = sc.unit
		}
	}

	c.containerIndices = make(map[string]int)
	for i, container := range cfg.Containers {
		c.containerIndices[container] = i
	}

	return nil
}

func (c *Collector) Run(ctx context.Context) error {
	out := make(chan stats, 100+len(c.cfg.Containers))
	var wg sync.WaitGroup

	for _, container := range c.cfg.Containers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			collect(ctx, c.logger, c.dockerClient, container, out)
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	var firstError error
	for s := range out {
		if s.Err != nil {
			c.logger.Error("Error collecting stats", "container", s.Container, "error", s.Err)
			if firstError == nil {
				firstError = s.Err
			}
		}

		c.storeStatsEntry(s.statsEntry)
	}

	return firstError
}

func (c *Collector) Results() []metrics.Results {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]metrics.Results, len(c.results))
	copy(out, c.results)
	return out
}

func (c *Collector) storeStatsEntry(entry statsEntry) {
	index, ok := c.containerIndices[entry.Name]
	if !ok {
		c.logger.Warn("Ignoring stats for container", "container", entry.Name)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Get the subset of the results slice that is relevant to the container.
	numMetrics := len(sampleCollectors)
	results := c.results[index*numMetrics : (index+1)*numMetrics]

	for i, sc := range sampleCollectors {
		results[i].Samples = append(results[i].Samples, sc.sample(entry))
	}
}

// sampleCollector is a helper struct to collect a sample from a statsEntry.
type sampleCollector struct {
	name   func(container string) string
	unit   string
	sample func(statsEntry) metrics.Sample
}

// sampleCollectors contains the actual metric definitions and the logic to
// collect them.
var sampleCollectors = []sampleCollector{
	{
		name: func(container string) string {
			return fmt.Sprintf("cpu_percentage[%s]", container)
		},
		unit: "%",
		sample: func(entry statsEntry) metrics.Sample {
			return metrics.Sample{At: entry.CollectedAt, Value: entry.CPUPercentage}
		},
	},
	{
		name: func(container string) string {
			return fmt.Sprintf("memory_usage[%s]", container)
		},
		unit: "MB",
		sample: func(entry statsEntry) metrics.Sample {
			return metrics.Sample{At: entry.CollectedAt, Value: entry.Memory / 1048576}
		},
	},
}
