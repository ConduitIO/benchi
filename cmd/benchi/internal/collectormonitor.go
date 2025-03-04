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
	"time"

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
	// interval is the time interval to refresh the model.
	interval time.Duration
}

// collectorMonitorModelID serves as a unique id generator for CollectorMonitorModel.
var collectorMonitorModelID atomic.Int32

type CollectorMonitorModelMsg struct {
	id      int32
	samples map[string][]float64
}

// NewCollectorMonitorModel creates a new CollectorMonitorModel.
func NewCollectorMonitorModel(collector metrics.Collector, numSamples int) CollectorMonitorModel {
	return CollectorMonitorModel{
		id:         collectorMonitorModelID.Add(1),
		collector:  collector,
		samples:    metricsToMap(collector.Metrics(), numSamples),
		numSamples: numSamples,
		interval:   time.Second,
	}
}

func (m CollectorMonitorModel) Init() tea.Cmd {
	return m.scheduleRefreshCmd()
}

func (m CollectorMonitorModel) scheduleRefreshCmd() tea.Cmd {
	return tea.Tick(m.interval, func(time.Time) tea.Msg {
		mm := m.collector.Metrics()
		samples := metricsToMap(mm, m.numSamples)

		return CollectorMonitorModelMsg{id: m.id, samples: samples}
	})
}

func (m CollectorMonitorModel) Update(msg tea.Msg) (CollectorMonitorModel, tea.Cmd) {
	collectorMsg, ok := msg.(CollectorMonitorModelMsg)
	if !ok || collectorMsg.id != m.id {
		return m, nil
	}

	m.samples = collectorMsg.samples
	return m, m.scheduleRefreshCmd()
}

// View renders the spark line.
func (m CollectorMonitorModel) View() string {
	s := ""
	for name, samples := range m.samples {
		s += fmt.Sprintf("- %s: %s: ", m.collector.Name(), name)
		if len(samples) > 0 {
			s += fmt.Sprintf("%.2f\t", samples[len(samples)-1])
			s += sparkline(samples)
		} else {
			s += "N/A"
		}

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

func metricsToMap(mm map[string][]metrics.Metric, size int) map[string][]float64 {
	samples := make(map[string][]float64)
	for name, metric := range mm {
		l := min(size, len(metric))
		samples[name] = make([]float64, l)
		for i, m := range metric[len(metric)-l:] {
			samples[name][i] = m.Value
		}
	}
	return samples
}
