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
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

	logPath := filepath.Join(*outPath, fmt.Sprintf("%s_benchi.log", time.Now().Format("20060102150405")))
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

	// Change working directory to config path, all relative paths are relative to the config file.
	err = os.Chdir(filepath.Dir(*configPath))
	if err != nil {
		return fmt.Errorf("could not change working directory: %w", err)
	}

	// TODO run all tests
	model, err := newTestModel(cfg, dockerClient)
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
	config config.Config
	tool   string

	infrastructureModel *composeProjectModel
	toolsModel          *composeProjectModel
}

func newTestModel(cfg config.Config, client client.APIClient) (tea.Model, error) {
	infraFiles := make([]string, 0, len(cfg.Infrastructure))
	for _, f := range cfg.Infrastructure {
		infraFiles = append(infraFiles, f.DockerCompose)
	}
	infraComposeProject, err := benchi.NewComposeProject(infraFiles, client)
	if err != nil {
		return nil, err
	}

	toolFiles := make([]string, 0, len(cfg.Tools))
	for _, f := range cfg.Tools {
		toolFiles = append(toolFiles, f.DockerCompose)
	}
	toolComposeProject, err := benchi.NewComposeProject(toolFiles, client)
	if err != nil {
		return nil, err
	}

	return &testModel{
		config: cfg,
		tool:   "conduit",

		infrastructureModel: &composeProjectModel{
			name:           "Infrastructure",
			composeProject: infraComposeProject,
		},
		toolsModel: &composeProjectModel{
			name:           "Tools",
			composeProject: toolComposeProject,
		},
	}, nil
}

func (m *testModel) Init() tea.Cmd {
	return tea.Batch(m.infrastructureModel.Init(), m.toolsModel.Init())
}

func (m *testModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case composeProjectModelTick:
		var cmds []tea.Cmd

		var cmdTmp tea.Cmd
		if m.infrastructureModel.name == msg.name {
			m.infrastructureModel, cmdTmp = m.infrastructureModel.Update(msg)
			cmds = append(cmds, cmdTmp)
		}
		if m.toolsModel.name == msg.name {
			m.toolsModel, cmdTmp = m.toolsModel.Update(msg)
			cmds = append(cmds, cmdTmp)
		}

		return m, tea.Batch(cmds...)
	}

	return m, nil
}

func (m *testModel) View() string {
	s := "Running test Foo (1/3)"
	s += "\n\n"
	s += m.infrastructureModel.View()
	s += "\n"
	s += m.toolsModel.View()
	return s
}

type composeProjectModel struct {
	name           string
	composeProject *benchi.ComposeProject

	containers []types.ContainerJSON
}

type composeProjectModelTick struct {
	name string
}

func (m *composeProjectModel) doTick() tea.Cmd {
	return func() tea.Msg {
		return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return composeProjectModelTick{name: m.name} })
	}
}

func (m *composeProjectModel) Init() tea.Cmd {
	m.containers = m.composeProject.Containers()
	return m.doTick()
}

func (m *composeProjectModel) Update(msg tea.Msg) (*composeProjectModel, tea.Cmd) {
	tickMsg, ok := msg.(composeProjectModelTick)
	if !ok || tickMsg.name != m.name {
		return m, nil
	}

	m.containers = m.composeProject.Containers()
	return m, m.doTick()
}

func (m *composeProjectModel) View() string {
	var s string
	s += fmt.Sprintf("%s:\n", m.name)
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

func (m *composeProjectModel) Close() {
	m.composeProject.Close()
}

/*

Loading tests ...

Tests to run:
- kafka-to-kafka
  - conduit
  - kafka-connect

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
