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
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

var (
	noColor     = os.Getenv("NO_COLOR") != ""
	statusStyle = lipgloss.NewStyle().Bold(true)

	statusRedStyle    = statusStyle.Foreground(lipgloss.Color("1"))
	statusGreenStyle  = statusStyle.Foreground(lipgloss.Color("2"))
	statusYellowStyle = statusStyle.Foreground(lipgloss.Color("3"))
)

type ContainerMonitorModel struct {
	id         int32
	logger     *slog.Logger
	ctx        context.Context
	client     client.APIClient
	interval   time.Duration
	containers []container.InspectResponse
}

// containerMonitorModelID serves as a unique id generator for ContainerMonitorModel.
var containerMonitorModelID atomic.Int32

type ContainerMonitorModelMsg struct {
	containers []container.InspectResponse
	id         int32
}

func NewContainerMonitorModel(ctx context.Context, client client.APIClient, containerNames []string) ContainerMonitorModel {
	containers := make([]container.InspectResponse, len(containerNames))
	for i, name := range containerNames {
		containers[i] = container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{Name: name}}
	}
	return ContainerMonitorModel{
		id:         containerMonitorModelID.Add(1),
		logger:     slog.Default().With("model", "container-monitor"),
		ctx:        ctx,
		client:     client,
		containers: containers,
		interval:   500 * time.Millisecond,
	}
}

func (m ContainerMonitorModel) Init() tea.Cmd {
	return m.scheduleRefreshCmd()
}

func (m ContainerMonitorModel) Update(msg tea.Msg) (ContainerMonitorModel, tea.Cmd) {
	refreshMsg, ok := msg.(ContainerMonitorModelMsg)
	if !ok || refreshMsg.id != m.id {
		return m, nil
	}

	m.containers = refreshMsg.containers
	return m, m.scheduleRefreshCmd()
}

func (m ContainerMonitorModel) View() string {
	var out string
	for _, c := range m.containers {
		style := statusStyle
		status := "N/A"

		if c.State != nil {
			status = c.State.Status

			// Statuses: created, running, paused, restarting, removing, exited, dead
			switch status {
			case "running":
				style = statusGreenStyle
				if c.State.Health != nil {
					switch c.State.Health.Status {
					case types.Healthy:
						style = statusGreenStyle
					case types.Unhealthy:
						style = statusRedStyle
					case types.Starting:
						style = statusYellowStyle
					}
					status += fmt.Sprintf(" (%s)", c.State.Health.Status)
				}
			case "exited", "dead":
				style = statusRedStyle
				if c.State.Error != "" {
					status += fmt.Sprintf(" (error: %s)", c.State.Error)
				}
			case "created", "paused", "restarting", "removing":
				style = statusYellowStyle
			}
		}
		if noColor {
			style.UnsetBackground()
			style.UnsetForeground()
		}

		out += fmt.Sprintf("  - %s: %s\n", c.Name, style.Render(status))
	}
	return out
}

func (m ContainerMonitorModel) scheduleRefreshCmd() tea.Cmd {
	return tea.Tick(m.interval, func(time.Time) tea.Msg {
		containersTmp := slices.Clone(m.containers)
		for i, c := range containersTmp {
			m.logger.Debug("Inspecting container", "name", c.Name)
			inspect, err := m.client.ContainerInspect(m.ctx, c.Name)
			if err != nil {
				if errdefs.IsNotFound(err) {
					m.logger.Debug("Container not found", "name", c.Name)
				} else {
					m.logger.Error("Failed to inspect container", "name", c.Name, "error", err)
				}
				containersTmp[i] = container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{Name: c.Name}}
				continue
			}
			// Docker inspect returns the internal docker container name which
			// is prefixed with the parent name and /. We remove the leading
			// slash to make the output nicer.
			inspect.Name = strings.TrimPrefix(inspect.Name, "/")
			containersTmp[i] = inspect
		}

		return ContainerMonitorModelMsg{
			id:         m.id,
			containers: containersTmp,
		}
	})
}
