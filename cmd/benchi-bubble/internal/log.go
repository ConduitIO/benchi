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
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// LogModel displays a log stream. It reads lines from an io.Reader and displays
// them in a scrolling view.
type LogModel struct {
	in          *bufio.Reader
	displaySize int
	lines       []string
}

type LogModelMsgLine struct {
	line string
}

// NewLogModel is the constructor for LogModel.
func NewLogModel(in io.Reader, displaySize int) LogModel {
	return LogModel{
		in:          bufio.NewReader(in),
		displaySize: displaySize,
		lines:       make([]string, displaySize),
	}
}

func (m LogModel) Init() tea.Cmd {
	return m.readLine()
}

func (m LogModel) Update(msg tea.Msg) (LogModel, tea.Cmd) {
	switch msg := msg.(type) {
	case LogModelMsgLine:
		m.lines = append(m.lines[1:], msg.line)
		return m, m.readLine()
	}
	return m, nil
}

func (m LogModel) View() string {
	return strings.Join(m.lines, "\n")
}

func (m LogModel) readLine() tea.Cmd {
	return func() tea.Msg {
		line, err := m.in.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		return LogModelMsgLine{line: line}
	}
}
