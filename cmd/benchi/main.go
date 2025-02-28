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
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/timer"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/conduitio/benchi"
	"github.com/conduitio/benchi/cmd/benchi/internal"
	"github.com/conduitio/benchi/config"
	"github.com/conduitio/benchi/dockerutil"
	"github.com/docker/docker/client"
	"github.com/lmittmann/tint"
	slogmulti "github.com/samber/slog-multi"
	"gopkg.in/yaml.v3"
)

var (
	configPath = flag.String("config", "", "path to the benchmark config file")
	outPath    = flag.String("out", "./results", "path to the output folder")
	tools      = internal.StringsFlag("tool", nil, "filter tool to be tested (can be provided multiple times)")
	tests      = internal.StringsFlag("tests", nil, "filter test to run (can be provided multiple times)")
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

	now := time.Now()
	infoReader, errorReader, closeLog, err := prepareLogger(now)
	defer closeLog()

	_, err = tea.NewProgram(newMainModel(infoReader, errorReader, now)).Run()
	return err
}

func prepareLogger(now time.Time) (io.Reader, io.Reader, func() error, error) {
	// Create log file.
	logPath := filepath.Join(*outPath, fmt.Sprintf("%s_benchi.log", now.Format("20060102150405")))
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create log file: %w", err)
	}

	// Create pipes for the CLI to read logs.
	infoReader, infoWriter := io.Pipe()
	errorReader, errorWriter := io.Pipe()

	logHandler := slogmulti.Fanout(
		// Write all logs to the log file.
		slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug}),
		// Only write info and above to the pipe writer (CLI).
		tint.NewHandler(infoWriter, &tint.Options{Level: slog.LevelInfo, NoColor: os.Getenv("NO_COLOR") != ""}),
		// Only write errors to another pipe, to show in the CLI.
		tint.NewHandler(errorWriter, &tint.Options{Level: slog.LevelError, NoColor: os.Getenv("NO_COLOR") != ""}),
	)
	slog.SetDefault(slog.New(logHandler))

	return infoReader, errorReader, func() error {
		var errs []error
		errs = append(errs, logFile.Close())
		errs = append(errs, infoWriter.Close())
		errs = append(errs, errorWriter.Close())
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

	// Log models for the CLI.
	infoLogModel  internal.LogModel
	errorLogModel internal.LogModel
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

func newMainModel(infoReader, errorReader io.Reader, now time.Time) mainModel {
	ctx, ctxCancel := context.WithCancel(context.Background())
	cleanupCtx, cleanupCtxCancel := context.WithCancel(context.Background())
	return mainModel{
		ctx:              ctx,
		ctxCancel:        ctxCancel,
		cleanupCtx:       cleanupCtx,
		cleanupCtxCancel: cleanupCtxCancel,

		startedAt: now,

		infoLogModel:  internal.NewLogModel(infoReader, 10),
		errorLogModel: internal.NewLogModel(errorReader, 0),
	}
}

func (m mainModel) Init() tea.Cmd {
	return tea.Batch(m.initCmd(), m.infoLogModel.Init(), m.errorLogModel.Init())
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

func (m mainModel) initCmd() tea.Cmd {
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
			return fmt.Errorf("failed to create docker client: %w", err)
		}
		defer dockerClient.Close()

		slog.Info("Creating docker network", "network", benchi.NetworkName)
		net, err := dockerutil.CreateNetworkIfNotExist(m.ctx, dockerClient, benchi.NetworkName)
		if err != nil {
			return fmt.Errorf("failed to create docker network: %w", err)
		}
		slog.Info("Using network", "network", net.Name, "network-id", net.ID)

		testRunners, err := benchi.BuildTestRunners(cfg, benchi.TestRunnerOptions{
			ResultsDir:   resultsDir,
			StartedAt:    now,
			FilterTests:  *tests,
			FilterTools:  *tools,
			DockerClient: dockerClient,
		})
		if err != nil {
			return fmt.Errorf("failed to build test runners: %w", err)
		}

		return mainModelMsgInitDone{
			config:       cfg,
			resultsDir:   resultsDir,
			startedAt:    now,
			dockerClient: dockerClient,
			testRunners:  testRunners,
		}
	}
}

func (mainModel) runTestCmd(index int) tea.Cmd {
	return func() tea.Msg {
		return mainModelMsgNextTest{testIndex: index}
	}
}

func (m mainModel) quitCmd() tea.Cmd {
	return func() tea.Msg {
		slog.Info("Removing docker network", "network", benchi.NetworkName)
		err := dockerutil.RemoveNetwork(m.cleanupCtx, m.dockerClient, benchi.NetworkName)
		if err != nil {
			slog.Error("Failed to remove network", "network", benchi.NetworkName, "error", err)
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

		return m, m.runTestCmd(0)
	case mainModelMsgNextTest:
		if msg.testIndex >= len(m.tests) {
			return m, m.quitCmd()
		}
		m.currentTestIndex = msg.testIndex
		return m, m.tests[m.currentTestIndex].Init()
	case testModelMsgDone:
		nextIndex := m.currentTestIndex + 1
		if m.ctx.Err() != nil {
			// Main context is cancelled, skip to the end.
			nextIndex = len(m.tests)
		}
		return m, m.runTestCmd(nextIndex)
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
	case internal.LogModelMsg:
		var cmds []tea.Cmd

		var cmdTmp tea.Cmd
		m.infoLogModel, cmdTmp = m.infoLogModel.Update(msg)
		cmds = append(cmds, cmdTmp)

		m.errorLogModel, cmdTmp = m.errorLogModel.Update(msg)
		cmds = append(cmds, cmdTmp)

		return m, tea.Batch(cmds...)
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
		return "Initializing ...\n\n" + m.infoLogModel.View()
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
	s += m.infoLogModel.View()

	if m.errorLogModel.HasContent() {
		s += "\n\nErrors:\n"
		s += m.errorLogModel.View()
	}

	s += "\n"

	return s
}

type testModel struct {
	ctx        context.Context
	cleanupCtx context.Context

	runner      *benchi.TestRunner
	errors      []error
	currentStep benchi.Step

	infrastructureModel internal.ContainerMonitorModel
	toolsModel          internal.ContainerMonitorModel
	collectorModels     []internal.CollectorMonitorModel

	progress internal.ProgressTimerModel
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

	collectorModels := make([]internal.CollectorMonitorModel, 0, len(runner.Collectors()))
	for _, c := range runner.Collectors() {
		collectorModels = append(collectorModels, internal.NewCollectorMonitorModel(c, 15))
	}

	return testModel{
		ctx:        ctx,
		cleanupCtx: cleanupCtx,

		runner: runner,

		// Run container monitor using the cleanup context, to keep monitor
		// running during cleanup.
		infrastructureModel: internal.NewContainerMonitorModel(cleanupCtx, client, infraContainers),
		toolsModel:          internal.NewContainerMonitorModel(cleanupCtx, client, toolContainers),
		collectorModels:     collectorModels,
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

func ptr[T any](v T) *T {
	return &v
}

func (m testModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.infrastructureModel.Init(),
		m.toolsModel.Init(),
		m.stepCmd(m.ctx),
	}

	for _, cm := range m.collectorModels {
		cmds = append(cmds, cm.Init())
	}

	return tea.Batch(cmds...)
}

func (m testModel) stepCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		err := m.runner.RunStep(ctx)
		return testModelMsgStep{err: err}
	}
}

func (m testModel) doneCmd() tea.Cmd {
	return func() tea.Msg {
		return testModelMsgDone{}
	}
}

func (m testModel) Update(msg tea.Msg) (testModel, tea.Cmd) {
	switch msg := msg.(type) {
	case testModelMsgDone:
		return m, nil

	case testModelMsgStep:
		if msg.err != nil {
			m.errors = append(m.errors, msg.err)
		}
		m.currentStep = m.runner.Step()

		switch {
		case m.currentStep == benchi.StepDone:
			return m, m.doneCmd()
		case m.currentStep >= benchi.StepPreCleanup:
			// Cleanup steps use the cleanup context.
			return m, m.stepCmd(m.cleanupCtx)
		case m.currentStep == benchi.StepTest:
			// Initialize progress bar.
			m.progress = internal.NewProgressTimerModel(
				m.runner.Duration(),
				time.Second,
				progress.WithDefaultGradient(),
				progress.WithoutPercentage(),
			)
			return m, tea.Batch(m.stepCmd(m.ctx), m.progress.Init())
		default:
			return m, m.stepCmd(m.ctx)
		}

	case timer.TickMsg:
		if m.currentStep == benchi.StepTest {
			var cmd tea.Cmd
			m.progress, cmd = m.progress.Update(msg)
			return m, cmd
		}

	case internal.ContainerMonitorModelMsg:
		var cmds []tea.Cmd

		var cmdTmp tea.Cmd
		m.infrastructureModel, cmdTmp = m.infrastructureModel.Update(msg)
		cmds = append(cmds, cmdTmp)

		m.toolsModel, cmdTmp = m.toolsModel.Update(msg)
		cmds = append(cmds, cmdTmp)

		return m, tea.Batch(cmds...)

	case internal.CollectorMonitorModelMsg:
		var cmds []tea.Cmd

		var cmdTmp tea.Cmd
		for i, cm := range m.collectorModels {
			m.collectorModels[i], cmdTmp = cm.Update(msg)
			cmds = append(cmds, cmdTmp)
		}

		return m, tea.Batch(cmds...)
	}

	return m, nil
}

func (m testModel) View() string {
	s := fmt.Sprintf("Step: %s", m.currentStep)
	if m.currentStep == benchi.StepTest {
		s += " " + m.progress.View()
	}
	s += "\n\n"
	s += "Infrastructure:\n"
	s += m.infrastructureModel.View()
	s += "\n"
	s += "Tools:\n"
	s += m.toolsModel.View()

	s += "\n"
	s += "Metrics:\n"

	indentStyle := lipgloss.NewStyle().PaddingLeft(2)
	for _, c := range m.collectorModels {
		s += indentStyle.Render(c.View())
	}

	return s
}
