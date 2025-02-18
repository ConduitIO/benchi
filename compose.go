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

package benchi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/conduitio/benchi/dockerutil"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

type ComposeProject struct {
	files      []string
	containers []string
	client     client.APIClient

	monitor     *containerMonitor
	stopMonitor context.CancelFunc
}

func NewComposeProject(
	files []string,
	client client.APIClient,
) (*ComposeProject, error) {
	containers, err := findContainerNames(files)
	if err != nil {
		return nil, fmt.Errorf("failed to find container names: %w", err)
	}

	monitor := newContainerMonitor(client, containers, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err := monitor.Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("containerMonitor failed", "error", err)
		}
	}()

	return &ComposeProject{
		files:      files,
		containers: containers,
		client:     client,

		monitor:     monitor,
		stopMonitor: cancel,
	}, nil
}

func findContainerNames(files []string) ([]string, error) {
	var buf bytes.Buffer
	err := dockerutil.ComposeConfig(
		context.Background(),
		dockerutil.ComposeOptions{
			File:   files,
			Stdout: &buf,
		},
		dockerutil.ComposeConfigOptions{
			Format: ptr("json"),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to parse compose files: %w", err)
	}

	var config map[string]any
	err = json.NewDecoder(&buf).Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse compose config: %w", err)
	}

	services, ok := config["services"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("services not found in compose config")
	}

	containers := make([]string, 0, len(services))
	for name, srv := range services {
		srvMap, ok := srv.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("service %s is not a map", name)
		}
		containerName, ok := srvMap["container_name"].(string)
		if !ok || containerName == "" {
			containerName = name
		}
		containers = append(containers, containerName)
	}
	slices.Sort(containers)

	return containers, nil
}

// Up starts the compose project in the background. It runs until the context is
// canceled or an error occurs or the project is stopped using Down.
func (cg *ComposeProject) Up(
	ctx context.Context,
	redirectLogsToFile string,
) error {
	f, err := os.Create(redirectLogsToFile)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	go func() {
		err := dockerutil.ComposeUp(
			ctx,
			dockerutil.ComposeOptions{
				File:   cg.files,
				Stdout: f,
				Stderr: f,
			},
			dockerutil.ComposeUpOptions{},
		)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("ComposeUp failed", "error", err)
		}
	}()
	return nil
}

// Down stops the compose project. It blocks until the project is stopped or the
// context is canceled.
func (cg *ComposeProject) Down(ctx context.Context) error {
	return dockerutil.ComposeDown(
		ctx,
		dockerutil.ComposeOptions{File: cg.files},
		dockerutil.ComposeDownOptions{},
	)
}

func (cg *ComposeProject) Close() {
	cg.stopMonitor()
}

func (cg *ComposeProject) Containers() []types.ContainerJSON {
	return cg.monitor.Containers()
}

func (cg *ComposeProject) Ready() bool {
	for _, c := range cg.Containers() {
		if c.State == nil ||
			!c.State.Running ||
			(c.State.Health != nil &&
				c.State.Health.Status != types.Healthy) {
			return false
		}
	}
	return true
}

// containerMonitor continuously monitors the status of selected docker
// containers.
type containerMonitor struct {
	client   client.APIClient
	interval time.Duration

	containers []types.ContainerJSON
	mu         sync.RWMutex
}

func newContainerMonitor(
	client client.APIClient,
	containerNames []string,
	interval time.Duration,
) *containerMonitor {
	containers := make([]types.ContainerJSON, len(containerNames))
	for i, name := range containerNames {
		containers[i] = types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{Name: name}}
	}
	return &containerMonitor{
		client:     client,
		interval:   interval,
		containers: containers,
	}
}

func (m *containerMonitor) Run(ctx context.Context) error {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	containersTmp := slices.Clone(m.containers)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			for i, c := range containersTmp {
				inspect, err := m.client.ContainerInspect(ctx, c.Name)
				if err != nil {
					slog.Error("failed to inspect container", "name", c.Name, "error", err)
					containersTmp[i] = types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{Name: c.Name}}
					continue
				}
				containersTmp[i] = inspect
			}
			m.mu.Lock()
			copy(m.containers, containersTmp)
			m.mu.Unlock()
		}
	}
}

func (m *containerMonitor) Containers() []types.ContainerJSON {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return slices.Clone(m.containers)
}
