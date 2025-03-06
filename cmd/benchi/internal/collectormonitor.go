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

	// results collected by the collector.
	results []metrics.Results

	// numSamples is the number of samples to keep.
	numSamples int
	// interval is the time interval to refresh the model.
	interval time.Duration
}

// collectorMonitorModelID serves as a unique id generator for CollectorMonitorModel.
var collectorMonitorModelID atomic.Int32

type CollectorMonitorModelMsg struct {
	id      int32
	results []metrics.Results
}

// NewCollectorMonitorModel creates a new CollectorMonitorModel.
func NewCollectorMonitorModel(collector metrics.Collector, numSamples int) CollectorMonitorModel {
	return CollectorMonitorModel{
		id:         collectorMonitorModelID.Add(1),
		collector:  collector,
		results:    collector.Results(),
		numSamples: numSamples,
		interval:   time.Second,
	}
}

func (m CollectorMonitorModel) Init() tea.Cmd {
	return m.scheduleRefreshCmd()
}

func (m CollectorMonitorModel) scheduleRefreshCmd() tea.Cmd {
	return tea.Tick(m.interval, func(time.Time) tea.Msg {
		return CollectorMonitorModelMsg{
			id:      m.id,
			results: m.collector.Results(),
		}
	})
}

func (m CollectorMonitorModel) Update(msg tea.Msg) (CollectorMonitorModel, tea.Cmd) {
	collectorMsg, ok := msg.(CollectorMonitorModelMsg)
	if !ok || collectorMsg.id != m.id {
		return m, nil
	}

	m.results = collectorMsg.results
	return m, m.scheduleRefreshCmd()
}

// Width returns the width of the longest metric (name, value and unit).
func (m CollectorMonitorModel) Width() int {
	width := 0
	for _, results := range m.results {
		w := len(displayMetricResults(m.collector.Name(), results))
		width = max(w, width)
	}
	return width
}

// View renders the spark line.
func (m CollectorMonitorModel) View(width int) string {
	s := ""
	for _, results := range m.results {
		text := displayMetricResults(m.collector.Name(), results)
		graph := sparkline(results.Samples, m.numSamples)
		s += fmt.Sprintf("%-*s %s\n", width, text, graph)
	}
	return strings.TrimSuffix(s, "\n")
}

var levels = []rune("▁▂▃▄▅▆▇█")

// sparkline draws a sparkline from the given data.
func sparkline(samples []metrics.Sample, lastN int) string {
	if len(samples) == 0 {
		return ""
	}

	data := samplesToFloatSlice(samples, lastN)
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

func samplesToFloatSlice(samples []metrics.Sample, lastN int) []float64 {
	l := min(lastN, len(samples))
	floats := make([]float64, l)
	for j, m := range samples[len(samples)-l:] {
		floats[j] = m.Value
	}
	return floats
}

func displayMetricResults(collectorName string, results metrics.Results) string {
	s := fmt.Sprintf("- %s: %s: ", collectorName, results.Name)
	if len(results.Samples) > 0 {
		s += fmt.Sprintf("%.2f", results.Samples[len(results.Samples)-1].Value)
		if results.Unit != "" {
			s += " " + results.Unit
		}
	} else {
		s += "N/A"
	}
	return s
}
