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
	"bufio"
	"fmt"
	"io"
	"strings"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
)

// LogModel displays a log stream. It reads lines from an io.Reader and displays
// them in a scrolling view.
type LogModel struct {
	id          int32
	in          *bufio.Reader
	displaySize int
	lines       []string
}

// logModelID serves as a unique id generator for LogModel.
var logModelID atomic.Int32

type LogModelMsg struct {
	id   int32
	line string
}

// NewLogModel is the constructor for LogModel.
func NewLogModel(in io.Reader, displaySize int) LogModel {
	return LogModel{
		id:          logModelID.Add(1),
		in:          bufio.NewReader(in),
		displaySize: displaySize,
		lines:       make([]string, 0, displaySize),
	}
}

func (m LogModel) Init() tea.Cmd {
	return m.readLineCmd()
}

func (m LogModel) Update(msg tea.Msg) (LogModel, tea.Cmd) {
	logMsg, ok := msg.(LogModelMsg)
	if !ok || logMsg.id != m.id {
		return m, nil
	}

	if m.displaySize > 0 && len(m.lines) == m.displaySize {
		m.lines = m.lines[1:]
	}

	m.lines = append(m.lines, logMsg.line)
	return m, m.readLineCmd()
}

func (m LogModel) View() string {
	return strings.Join(m.lines, "\n")
}

func (m LogModel) HasContent() bool {
	return len(m.lines) > 0
}

func (m LogModel) readLineCmd() tea.Cmd {
	return func() tea.Msg {
		line, err := m.in.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("failed to read log line: %w", err)
		}
		line = strings.TrimSpace(line)
		return LogModelMsg{id: m.id, line: line}
	}
}
