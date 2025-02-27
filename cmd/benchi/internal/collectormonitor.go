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

package internal

import (
	"fmt"
	"math"
	"strings"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/conduitio/benchi/metrics"
)

// CollectorMonitorModel is a model that renders the information of a collector.
type CollectorMonitorModel struct {
	id int32
	// collector is the metrics collector to monitor.
	collector metrics.Collector
	// samples to render.
	samples map[string][]float64
	// numSamples is the number of samples to keep.
	numSamples int
}

// collectorMonitorModelID serves as a unique id generator for CollectorMonitorModel.
var collectorMonitorModelID atomic.Int32

type CollectorMonitorModelMsg struct {
	id     int32
	metric metrics.Metric
	done   bool
}

// NewCollectorMonitorModel creates a new CollectorMonitorModel.
func NewCollectorMonitorModel(collector metrics.Collector, numSamples int) CollectorMonitorModel {
	return CollectorMonitorModel{
		id:         containerMonitorModelID.Add(1),
		collector:  collector,
		samples:    make(map[string][]float64),
		numSamples: numSamples,
	}
}

func (m CollectorMonitorModel) Init() tea.Cmd {
	return m.nextMetricCmd()
}

func (m CollectorMonitorModel) nextMetricCmd() tea.Cmd {
	return func() tea.Msg {
		metric, ok := <-m.collector.Out()
		if !ok {
			return CollectorMonitorModelMsg{id: m.id, done: true}
		}
		return CollectorMonitorModelMsg{id: m.id, metric: metric}
	}
}

func (m CollectorMonitorModel) Update(msg tea.Msg) (CollectorMonitorModel, tea.Cmd) {
	collectorMsg, ok := msg.(CollectorMonitorModelMsg)
	if !ok || collectorMsg.id != m.id || collectorMsg.done {
		return m, nil
	}

	samples, ok := m.samples[collectorMsg.metric.Name]
	if !ok {
		samples = make([]float64, 0, m.numSamples)
	}
	samples = append(samples, collectorMsg.metric.Value)
	if len(samples) > m.numSamples {
		samples = samples[1:]
	}
	m.samples[collectorMsg.metric.Name] = samples
	return m, m.nextMetricCmd()
}

// View renders the spark line.
func (m CollectorMonitorModel) View() string {
	s := ""
	for name, samples := range m.samples {
		s += fmt.Sprintf("- %s: %s: %.2f\t", m.collector.Name(), name, samples[len(samples)-1])
		s += sparkline(samples)
		s += "\n"
	}
	return s
}

var levels = []rune("▁▂▃▄▅▆▇█")

// sparkline draws a sparkline from the given data.
func sparkline(data []float64) string {
	if len(data) == 0 {
		return ""
	}
	minSample := data[0]
	maxSample := data[0]
	for _, y := range data {
		minSample = min(minSample, y)
		maxSample = max(maxSample, y)
	}
	if minSample == maxSample {
		return strings.Repeat(string(levels[len(levels)/2]), len(data))
	}

	line := make([]rune, len(data))
	for i, v := range data {
		j := int(math.Round((v - minSample) / (maxSample - minSample) * float64(len(levels)-1)))
		line[i] = levels[j]
	}

	return string(line)
}
