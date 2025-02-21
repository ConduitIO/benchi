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
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/timer"
	tea "github.com/charmbracelet/bubbletea"
)

// ProgressTimerModel is a model that combines a timer and a progress bar.
// It's useful for showing a progress bar that fills up over time.
type ProgressTimerModel struct {
	timeout  time.Duration
	timer    timer.Model
	progress progress.Model
}

func NewProgressTimerModel(timeout time.Duration, interval time.Duration, progressOpts ...progress.Option) ProgressTimerModel {
	return ProgressTimerModel{
		timeout:  timeout,
		timer:    timer.NewWithInterval(timeout, interval),
		progress: progress.New(progressOpts...),
	}
}

func (m ProgressTimerModel) Init() tea.Cmd {
	return tea.Batch(m.timer.Init(), m.progress.Init())
}

func (m ProgressTimerModel) Update(msg tea.Msg) (ProgressTimerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case timer.TickMsg:
		var cmd tea.Cmd
		m.timer, cmd = m.timer.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m ProgressTimerModel) View() string {
	return m.progress.ViewAs(float64(m.timeout-m.timer.Timeout)/float64(m.timeout)) + " (" + m.timer.View() + ")"
}
