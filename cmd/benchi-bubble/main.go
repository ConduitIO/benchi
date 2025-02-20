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
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/timer"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/conduitio/benchi"
	"github.com/conduitio/benchi/cmd/benchi-bubble/internal"
	"github.com/conduitio/benchi/config"
	"github.com/conduitio/benchi/dockerutil"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	slogmulti "github.com/samber/slog-multi"
	"gopkg.in/yaml.v3"
)

var (
	configPath = flag.String("config", "", "path to the benchmark config file")
	outPath    = flag.String("out", "./results", "path to the output folder")
)

const (
	networkName = "benchi"
)

func main() {
	if err := mainE(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func mainE() error {
	flag.Parse()

	if configPath == nil || strings.TrimSpace(*configPath) == "" {
		return fmt.Errorf("config path is required")
	}

	// Create output directory if it does not exist.
	err := os.MkdirAll(*outPath, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Prepare logger.
	now := time.Now()
	pr, closeLog, err := prepareLogger(now)
	defer closeLog()

	_, err = tea.NewProgram(newMainModel(pr, now)).Run()
	return err
}

func prepareLogger(now time.Time) (io.Reader, func() error, error) {
	// Create log file.
	logPath := filepath.Join(*outPath, fmt.Sprintf("%s_benchi.log", now.Format("20060102150405")))
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create log file: %w", err)
	}

	// Create a pipe for the CLI to read logs.
	pr, pw := io.Pipe()

	logHandler := slogmulti.Fanout(
		// Write all logs to the log file.
		slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug}),
		// Only write info and above to the pipe writer (CLI).
		slog.NewTextHandler(pw, &slog.HandlerOptions{Level: slog.LevelInfo}),
	)
	slog.SetDefault(slog.New(logHandler))

	return pr, func() error {
		var errs []error
		errs = append(errs, logFile.Close())
		errs = append(errs, pw.Close())
		return errors.Join(errs...)
	}, nil
}

type mainModel struct {
	ctx              context.Context
	ctxCancel        context.CancelFunc
	cleanupCtx       context.Context
	cleanupCtxCancel context.CancelFunc

	initialized bool

	// These fields are initialized in Init
	config       config.Config
	resultsDir   string
	startedAt    time.Time
	dockerClient client.APIClient

	tests            []testModel
	currentTestIndex int

	// Log model for the CLI.
	logModel internal.LogModel
}

type mainModelMsgInitDone struct {
	config       config.Config
	resultsDir   string
	startedAt    time.Time
	dockerClient client.APIClient

	testRunners benchi.TestRunners
}

type mainModelMsgNextTest struct {
	testIndex int
}

func newMainModel(logReader io.Reader, now time.Time) mainModel {
	ctx, ctxCancel := context.WithCancel(context.Background())
	cleanupCtx, cleanupCtxCancel := context.WithCancel(context.Background())
	return mainModel{
		ctx:              ctx,
		ctxCancel:        ctxCancel,
		cleanupCtx:       cleanupCtx,
		cleanupCtxCancel: cleanupCtxCancel,

		startedAt: now,

		logModel: internal.NewLogModel(logReader, 10),
	}
}

func (m mainModel) Init() tea.Cmd {
	return tea.Batch(m.init(), m.logModel.Init())
}

func (m mainModel) init() tea.Cmd {
	return func() tea.Msg {
		now := time.Now()

		// Resolve absolute paths.
		resultsDir, err := filepath.Abs(*outPath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for output directory: %w", err)
		}
		slog.Info("Results directory", "path", resultsDir)

		configPath, err := filepath.Abs(*configPath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for config file: %w", err)
		}
		slog.Info("Config file", "path", configPath)

		// Parse config.
		cfg, err := m.parseConfig(configPath)
		if err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}
		slog.Info("Parsed config", "path", configPath)

		// Change working directory to config path, all relative paths are relative
		// to the config file.
		err = os.Chdir(filepath.Dir(configPath))
		if err != nil {
			return fmt.Errorf("could not change working directory: %w", err)
		}

		// Create docker client and initialize network.
		dockerClient, err := client.NewClientWithOpts(client.FromEnv)
		if err != nil {
			return err
		}
		defer dockerClient.Close()

		slog.Info("Creating docker network", "network", networkName)
		net, err := dockerutil.CreateNetworkIfNotExist(m.ctx, dockerClient, networkName)
		if err != nil {
			return err
		}
		slog.Info("Using network", "network", net.Name, "network-id", net.ID)

		testRunners := benchi.BuildTestRunners(cfg, benchi.TestRunnerOptions{
			ResultsDir:   resultsDir,
			StartedAt:    now,
			FilterTests:  nil,
			FilterTools:  nil,
			DockerClient: dockerClient,
		})

		return mainModelMsgInitDone{
			config:       cfg,
			resultsDir:   resultsDir,
			startedAt:    now,
			dockerClient: dockerClient,
			testRunners:  testRunners,
		}
	}
}

func (mainModel) parseConfig(path string) (config.Config, error) {
	f, err := os.Open(path)
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

func (mainModel) runTest(index int) tea.Cmd {
	return func() tea.Msg {
		return mainModelMsgNextTest{testIndex: index}
	}
}

func (m mainModel) quit() tea.Cmd {
	return func() tea.Msg {
		slog.Info("Removing docker network", "network", networkName)
		err := dockerutil.RemoveNetwork(m.cleanupCtx, m.dockerClient, networkName)
		if err != nil {
			slog.Error("Failed to remove network", "network", networkName, "error", err)
		}
		return tea.Quit()
	}
}

func (m mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	slog.Debug("Received message", "msg", msg, "type", fmt.Sprintf("%T", msg))
	switch msg := msg.(type) {
	case mainModelMsgInitDone:
		m.config = msg.config
		m.resultsDir = msg.resultsDir
		m.startedAt = msg.startedAt
		m.dockerClient = msg.dockerClient

		tests := make([]testModel, len(msg.testRunners))
		for i, tr := range msg.testRunners {
			test, err := newTestModel(m.ctx, m.cleanupCtx, m.dockerClient, tr)
			if err != nil {
				return m, func() tea.Msg { return fmt.Errorf("failed to create test model: %w", err) }
			}
			tests[i] = test
		}
		m.tests = tests
		m.initialized = true

		return m, m.runTest(0)
	case mainModelMsgNextTest:
		if msg.testIndex >= len(m.tests) {
			return m, m.quit()
		}
		m.currentTestIndex = msg.testIndex
		return m, m.tests[m.currentTestIndex].Init()
	case testModelMsgDone:
		if m.ctx.Err() != nil {
			// Main context is cancelled, skip to the end.
			return m, m.runTest(len(m.tests))
		}
		return m, m.runTest(m.currentTestIndex + 1)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			if m.ctx.Err() == nil {
				// First time, cancel the main context.
				m.ctxCancel()
				return m, nil
			} else if m.cleanupCtx.Err() == nil {
				// Second time, cancel the cleanup context.
				m.cleanupCtxCancel()
				return m, nil
			} else {
				// Third time, just quit.
				return m, tea.Quit
			}
		}
	case error:
		slog.Error("Error message", "error", msg)
		return m, nil
	case internal.LogModelMsgLine:
		var cmd tea.Cmd
		m.logModel, cmd = m.logModel.Update(msg)
		return m, cmd
	}

	if m.initialized {
		var cmd tea.Cmd
		m.tests[m.currentTestIndex], cmd = m.tests[m.currentTestIndex].Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m mainModel) View() string {
	if !m.initialized {
		return "Initializing ...\n\n" + m.logModel.View()
	}

	s := fmt.Sprintf("Running test %s (%d/%d)", m.tests[m.currentTestIndex].runner.Name(), m.currentTestIndex+1, len(m.tests))
	if m.cleanupCtx.Err() != nil {
		s += " (cleanup cancelled, press Ctrl+C again to quit immediately)"
	} else if m.ctx.Err() != nil {
		s += " (gracefully stopping test, press Ctrl+C again to cancel cleanup)"
	}
	s += "\n\n"
	s += m.tests[m.currentTestIndex].View()

	s += "\n\n"
	s += m.logModel.View()
	return s
}

type testModel struct {
	ctx        context.Context
	cleanupCtx context.Context

	runner      *benchi.TestRunner
	errors      []error
	currentStep benchi.Step

	infrastructureModel containerMonitorModel
	toolsModel          containerMonitorModel

	progress internal.ProgressTimer
}

type testModelMsgStep struct {
	err error
}

type testModelMsgDone struct{}

func newTestModel(ctx context.Context, cleanupCtx context.Context, client client.APIClient, runner *benchi.TestRunner) (testModel, error) {
	infraFiles := make([]string, 0, len(runner.Infrastructure()))
	for _, f := range runner.Infrastructure() {
		infraFiles = append(infraFiles, f.DockerCompose)
	}
	infraContainers, err := findContainerNames(infraFiles)
	if err != nil {
		return testModel{}, err
	}

	toolFiles := make([]string, 0, len(runner.Tools()))
	for _, f := range runner.Tools() {
		toolFiles = append(toolFiles, f.DockerCompose)
	}
	toolContainers, err := findContainerNames(toolFiles)
	if err != nil {
		return testModel{}, err
	}

	return testModel{
		ctx:        ctx,
		cleanupCtx: cleanupCtx,

		runner: runner,

		infrastructureModel: newContainerMonitorModel(client, infraContainers),
		toolsModel:          newContainerMonitorModel(client, toolContainers),
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

func (m testModel) Init() tea.Cmd {
	return tea.Batch(m.infrastructureModel.Init(), m.toolsModel.Init(), m.step(m.ctx))
}

func (m testModel) step(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		err := m.runner.RunStep(ctx)
		return testModelMsgStep{err: err}
	}
}

func (m testModel) done() tea.Cmd {
	return func() tea.Msg {
		return testModelMsgDone{}
	}
}

func (m testModel) Update(msg tea.Msg) (testModel, tea.Cmd) {
	switch msg := msg.(type) {
	case testModelMsgStep:
		if msg.err != nil {
			m.errors = append(m.errors, msg.err)
		}
		m.currentStep = m.runner.Step()

		switch {
		case m.currentStep == benchi.StepDone:
			return m, m.done()
		case m.currentStep >= benchi.StepPreCleanup:
			return m, m.step(m.cleanupCtx)
		case m.currentStep == benchi.StepTest:
			// Initialize progress bar.
			m.progress = internal.NewProgressTimer(m.runner.Duration(), time.Second, progress.WithDefaultGradient())
			return m, tea.Batch(m.step(m.ctx), m.progress.Init())
		default:
			return m, m.step(m.ctx)
		}
	case testModelMsgDone:
		return m, nil
	case timer.TickMsg:
		if m.currentStep == benchi.StepTest {
			var cmd tea.Cmd
			m.progress, cmd = m.progress.Update(msg)
			return m, cmd
		}
	}

	var cmds []tea.Cmd

	var cmdTmp tea.Cmd
	m.infrastructureModel, cmdTmp = m.infrastructureModel.Update(msg)
	cmds = append(cmds, cmdTmp)

	m.toolsModel, cmdTmp = m.toolsModel.Update(msg)
	cmds = append(cmds, cmdTmp)

	return m, tea.Batch(cmds...)
}

func (m testModel) View() string {
	s := fmt.Sprintf("Running test %s (1/3)", m.runner.Name())
	s = fmt.Sprintf("Step: %s", m.currentStep)
	if m.currentStep == benchi.StepTest {
		s += " " + m.progress.View()
	}
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

type containerMonitorModelRefreshMsg struct {
	containers []types.ContainerJSON
	id         int32
}

func newContainerMonitorModel(client client.APIClient, containerNames []string) containerMonitorModel {
	containers := make([]types.ContainerJSON, len(containerNames))
	for i, name := range containerNames {
		containers[i] = types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{Name: name}}
	}
	return containerMonitorModel{
		id:         containerMonitorModelID.Add(1),
		client:     client,
		containers: containers,
		interval:   500 * time.Millisecond,
	}
}

func (m containerMonitorModel) scheduleRefresh() tea.Cmd {
	return tea.Tick(m.interval, func(time.Time) tea.Msg {
		containersTmp := slices.Clone(m.containers)
		for i, c := range containersTmp {
			slog.Debug("Inspecting container", "name", c.Name)
			inspect, err := m.client.ContainerInspect(context.Background(), c.Name)
			if err != nil {
				if errdefs.IsNotFound(err) {
					slog.Debug("Container not found", "name", c.Name)
				} else {
					slog.Error("Failed to inspect container", "name", c.Name, "error", err)
				}
				containersTmp[i] = types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{Name: c.Name}}
				continue
			}
			containersTmp[i] = inspect
		}

		return containerMonitorModelRefreshMsg{
			id:         m.id,
			containers: containersTmp,
		}
	})
}

func (m containerMonitorModel) Init() tea.Cmd {
	return m.scheduleRefresh()
}

func (m containerMonitorModel) Update(msg tea.Msg) (containerMonitorModel, tea.Cmd) {
	refreshMsg, ok := msg.(containerMonitorModelRefreshMsg)
	if !ok || refreshMsg.id != m.id {
		return m, nil
	}

	m.containers = refreshMsg.containers
	return m, m.scheduleRefresh()
}

func (m containerMonitorModel) View() string {
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
