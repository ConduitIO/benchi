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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/conduitio/benchi"
	"github.com/conduitio/benchi/config"
	"github.com/conduitio/benchi/dockerutil"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"gopkg.in/yaml.v3"
)

var (
	configPath = flag.String("config", "", "path to the benchmark config file")
	outPath    = flag.String("out", "./results", "path to the output folder")
)

func main() {
	if err := mainE(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func mainE() error {
	ctx := context.Background()
	flag.Parse()

	fmt.Print("Preparing ...")

	if configPath == nil || strings.TrimSpace(*configPath) == "" {
		return fmt.Errorf("config path is required")
	}

	// Create output directory if it does not exist.
	err := os.MkdirAll(*outPath, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	now := time.Now()
	logPath := filepath.Join(*outPath, fmt.Sprintf("%s_benchi.log", now.Format("20060102150405")))
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer logFile.Close()

	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug})))

	cfg, err := parseConfig()
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	net, err := dockerutil.CreateNetworkIfNotExist(ctx, dockerClient, "benchi") // TODO move magic value to constant
	if err != nil {
		return err
	}
	slog.Info("Using network", "network", net.Name, "network-id", net.ID)
	defer dockerutil.RemoveNetwork(ctx, dockerClient, net.Name)

	// Resolve absolute path before changing working directory
	*outPath, err = filepath.Abs(*outPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for output directory: %w", err)
	}

	// Change working directory to config path, all relative paths are relative to the config file.
	err = os.Chdir(filepath.Dir(*configPath))
	if err != nil {
		return fmt.Errorf("could not change working directory: %w", err)
	}

	tr := benchi.BuildTestRunners(cfg, benchi.TestRunnerOptions{
		ResultsDir:   *outPath,
		StartedAt:    now,
		FilterTests:  nil,
		FilterTools:  nil,
		DockerClient: dockerClient,
	})

	// TODO run all tests
	model, err := newTestModel(dockerClient, tr[0])
	if err != nil {
		return fmt.Errorf("failed to create test model: %w", err)
	}
	_, err = tea.NewProgram(model, tea.WithContext(ctx)).Run()
	return err
}

func parseConfig() (config.Config, error) {
	f, err := os.Open(*configPath)
	if err != nil {
		return config.Config{}, err
	}
	defer f.Close()
	var cfg config.Config
	err = yaml.NewDecoder(f).Decode(&cfg)
	if err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

type testModel struct {
	runner      *benchi.TestRunner
	errors      []error
	currentStep benchi.Step

	infrastructureModel *containerMonitorModel
	toolsModel          *containerMonitorModel
}

type stepResult struct {
	err error
}

func newTestModel(client client.APIClient, runner *benchi.TestRunner) (tea.Model, error) {
	infraFiles := make([]string, 0, len(runner.Infrastructure()))
	for _, f := range runner.Infrastructure() {
		infraFiles = append(infraFiles, f.DockerCompose)
	}
	infraContainers, err := findContainerNames(infraFiles)
	if err != nil {
		return nil, err
	}

	toolFiles := make([]string, 0, len(runner.Tools()))
	for _, f := range runner.Tools() {
		toolFiles = append(toolFiles, f.DockerCompose)
	}
	toolContainers, err := findContainerNames(toolFiles)
	if err != nil {
		return nil, err
	}

	return &testModel{
		runner: runner,

		infrastructureModel: newContainerMonitorModel(client, infraContainers),
		toolsModel:          newContainerMonitorModel(client, toolContainers),
	}, nil
}

func (m *testModel) Init() tea.Cmd {
	return tea.Batch(m.infrastructureModel.Init(), m.toolsModel.Init(), m.step())
}

func (m *testModel) step() tea.Cmd {
	return func() tea.Msg {
		err := m.runner.RunStep(context.Background())
		return stepResult{err: err}
	}
}

func (m *testModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case containerMonitorModelRefresh:
		var cmds []tea.Cmd

		var cmdTmp tea.Cmd
		m.infrastructureModel, cmdTmp = m.infrastructureModel.Update(msg)
		cmds = append(cmds, cmdTmp)

		m.toolsModel, cmdTmp = m.toolsModel.Update(msg)
		cmds = append(cmds, cmdTmp)

		return m, tea.Batch(cmds...)
	case stepResult:
		if msg.err != nil {
			m.errors = append(m.errors, msg.err)
		}
		m.currentStep = m.runner.Step()
		if m.currentStep != benchi.StepDone {
			return m, m.step()
		}
	}

	return m, nil
}

func (m *testModel) View() string {
	s := fmt.Sprintf("Running test %s (1/3)", m.runner.Name())
	s = fmt.Sprintf("Step: %s", m.currentStep)
	s += "\n\n"
	s += "Infrastructure:\n"
	s += m.infrastructureModel.View()
	s += "\n"
	s += "Tools:\n"
	s += m.toolsModel.View()
	return s
}

type containerMonitorModel struct {
	id         int32
	client     client.APIClient
	interval   time.Duration
	containers []types.ContainerJSON
}

// containerMonitorModelID serves as a unique id generator for containerMonitorModel.
var containerMonitorModelID atomic.Int32

type containerMonitorModelRefresh struct {
	containers []types.ContainerJSON
	id         int32
}

func newContainerMonitorModel(client client.APIClient, containerNames []string) *containerMonitorModel {
	containers := make([]types.ContainerJSON, len(containerNames))
	for i, name := range containerNames {
		containers[i] = types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{Name: name}}
	}
	return &containerMonitorModel{
		id:         containerMonitorModelID.Add(1),
		client:     client,
		containers: containers,
		interval:   500 * time.Millisecond,
	}
}

func (m *containerMonitorModel) doRefresh() tea.Cmd {
	return tea.Tick(m.interval, func(time.Time) tea.Msg {
		containersTmp := slices.Clone(m.containers)
		for i, c := range containersTmp {
			slog.Debug("inspecting container", "name", c.Name)
			inspect, err := m.client.ContainerInspect(context.Background(), c.Name)
			if err != nil {
				slog.Error("failed to inspect container", "name", c.Name, "error", err)
				containersTmp[i] = types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{Name: c.Name}}
				continue
			}
			containersTmp[i] = inspect
		}

		return containerMonitorModelRefresh{
			id:         m.id,
			containers: containersTmp,
		}
	})
}

func (m *containerMonitorModel) Init() tea.Cmd {
	return m.doRefresh()
}

func (m *containerMonitorModel) Update(msg tea.Msg) (*containerMonitorModel, tea.Cmd) {
	refreshMsg, ok := msg.(containerMonitorModelRefresh)
	if !ok || refreshMsg.id != m.id {
		return m, nil
	}

	m.containers = refreshMsg.containers
	return m, m.doRefresh()
}

func (m *containerMonitorModel) View() string {
	var s string
	for _, c := range m.containers {
		if c.State == nil {
			s += fmt.Sprintf("  - %s: %s\n", c.Name, "N/A")
			continue
		}

		s += fmt.Sprintf("  - %s: %s", c.Name, c.State.Status)
		if c.State.Health != nil {
			s += fmt.Sprintf(" (%s)", c.State.Health.Status)
		}
		s += "\n"
	}
	return s
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

	var cfg map[string]any
	err = json.NewDecoder(&buf).Decode(&cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse compose config: %w", err)
	}

	services, ok := cfg["services"].(map[string]any)
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

func ptr[T any](v T) *T {
	return &v
}

/*
Tests:
[⣾] kafka-to-kafka
  [⣾] conduit

      Step: infrastructure

      Infrastructure:
       ⣾ benchi-zookeeper (zookeeper:3.9.0): starting
       ⣾ benchi-kafka (kafka:3.9.0): starting

      Tools:
         benchi-conduit: N/A

      Collectors:
         kafka-docker-usage: waiting
         conduit-docker-usage: waiting
         conduit-metrics: waiting
         kafka-metrics: waiting

  [ ] kafka-connect

----------------------

Running test (1/3)

Test: kafka-to-kafka
Tool: conduit
Status: starting infrastructure

Infrastructure:
 ⣾ benchi-zookeeper (zookeeper:3.9.0): starting
 ⣾ benchi-kafka (kafka:3.9.0): starting

Tools:
   benchi-conduit: waiting

Collectors:
   kafka-docker-usage: waiting
   conduit-docker-usage: waiting
   conduit-metrics: waiting
   kafka-metrics: waiting

----------------------

Running test (1/3)

Test: kafka-to-kafka
Tool: conduit
Status: starting tools

Infrastructure:
 ✔ benchi-zookeeper (zookeeper:3.9.0): running (healthy)
 ✔ benchi-kafka (kafka:3.9.0): running (healthy)

Tools:
 ⣾ benchi-conduit: starting

Collectors:
   kafka-docker-usage: waiting
   conduit-docker-usage: waiting
   conduit-metrics: waiting
   kafka-metrics: waiting

----------------------

Running test (1/3)

Test: kafka-to-kafka
Tool: conduit
Status: running (39s remaining)

Infrastructure:
 ✔ benchi-zookeeper (zookeeper:3.9.0): running (healthy)
 ✔ benchi-kafka (kafka:3.9.0): running (healthy)

Tools:
 ✔ benchi-conduit: running (consider adding a health-check)

Collectors:
 ⣾ kafka-docker-usage: running
 ⣾ conduit-docker-usage: running
 ⣾ conduit-metrics: running
 ⣾ kafka-metrics: running

----------------------

Running test (1/3)

Test: kafka-to-kafka
Tool: conduit
Status: stopping collectors

Infrastructure:
 ✔ benchi-zookeeper (zookeeper:3.9.0): running (healthy)
 ✔ benchi-kafka (kafka:3.9.0): running (healthy)

Tools:
 ✔ benchi-conduit: running (consider adding a health-check)

Collectors:
 ✔ kafka-docker-usage: stopped
 ⣾ conduit-docker-usage: stopping
 ✔ conduit-metrics: stopped
 ⣾ kafka-metrics: stopping

----------------------

Running test (1/3)

Test: kafka-to-kafka
Tool: conduit
Status: stopping tools

Infrastructure:
 ✔ benchi-zookeeper (zookeeper:3.9.0): running (healthy)
 ✔ benchi-kafka (kafka:3.9.0): running (healthy)

 Tools:
 ✔ benchi-conduit: stopped

 Collectors:
 ✔ kafka-docker-usage: stopped
 ✔ conduit-docker-usage: stopped
 ✔ conduit-metrics: stopped
 ✔ kafka-metrics: stopped

----------------------

Running test (1/3)

Test: kafka-to-kafka
Tool: conduit
Status: stopping infrastructure

Infrastructure:
 ✔ benchi-zookeeper (zookeeper:3.9.0): stopped
 ✔ benchi-kafka (kafka:3.9.0): stopped

 Tools:
 ✔ benchi-conduit: stopped

 Collectors:
 ✔ kafka-docker-usage: stopped
 ✔ conduit-docker-usage: stopped
 ✔ conduit-metrics: stopped
 ✔ kafka-metrics: stopped

*/
