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

package metrics

import (
	"context"
	"fmt"
	"log/slog"
)

type Collector interface {
	// Name returns the name of the collector.
	Name() string
	// Type returns the type of the collector.
	Type() string
	// Configure sets up the collector with the provided settings.
	Configure(settings map[string]any) error
	// View returns a string representation of the collected metrics. This is
	// used to display the metrics in the CLI as the test is running.
	// TODO View() string

	// Run continuously runs the collection process until the context is
	// cancelled. The function should block until the context is cancelled, an
	// error occurs, or the Stop function is called.
	Run(ctx context.Context) error
	// Stop should stop the collection process. This is called when the test is
	// complete. The function should block until Run has returned or the context
	// is cancelled.
	Stop(context.Context) error
	// Flush should flush all collected metrics to the specified directory. This
	// is called after the collector has been stopped.
	Flush(ctx context.Context, dir string) error
}

type NewCollectorFunc[T Collector] func(logger *slog.Logger, name string) T

var collectors = make(map[string]NewCollectorFunc[Collector])

// RegisterCollector registers a new collector type with the metrics package.
func RegisterCollector[T Collector](newFunc NewCollectorFunc[T]) {
	// Dry run the function to ensure it works and to get the type.
	t := newFunc(slog.Default(), "foo")
	if t.Name() != "foo" {
		panic("collector name must be 'foo'")
	}
	collectorType := t.Type()

	collectors[collectorType] = func(logger *slog.Logger, name string) Collector {
		logger = logger.With("collector", name, "type", collectorType)
		return newFunc(logger, name)
	}
}

func NewCollector(logger *slog.Logger, name string, collectorType string) (Collector, error) {
	newFunc, ok := collectors[collectorType]
	if !ok {
		return nil, fmt.Errorf("unknown collector type: %s", collectorType)
	}
	return newFunc(logger, name), nil
}
