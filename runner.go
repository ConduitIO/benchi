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
	"encoding/csv"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/conduitio/benchi/config"
	"github.com/conduitio/benchi/dockerutil"
	"github.com/conduitio/benchi/metrics"
	"github.com/conduitio/benchi/metrics/conduit"
	"github.com/conduitio/benchi/metrics/prometheus"
	"github.com/docker/docker/client"
	"github.com/sourcegraph/conc/pool"
)

const (
	NetworkName      = "benchi"
	DefaultHookImage = "alpine:latest"
)

type TestRunners []*TestRunner

type TestRunnerOptions struct {
	// ResultsDir is the directory where the test results are stored.
	ResultsDir string
	// StartedAt is the time when the test was started. All test results are
	// stored in a subdirectory that includes this time in the name.
	StartedAt time.Time
	// FilterTests is a list of test names to run. If empty, all tests are run.
	FilterTests []string
	// FilterTools is a list of tool names to run. If empty, all tools are run.
	FilterTools []string
	// DockerClient is the Docker client to use for running the tests.
	DockerClient client.APIClient
}

func BuildTestRunners(cfg config.Config, opt TestRunnerOptions) (TestRunners, error) {
	// Register metrics collectors
	conduit.Register()
	prometheus.Register()

	runs := make(TestRunners, 0, len(cfg.Tests)*len(cfg.Tools))

	for _, t := range cfg.Tests {
		logger := slog.Default().With("test", t.Name)

		if len(opt.FilterTests) > 0 && !slices.Contains(opt.FilterTests, t.Name) {
			logger.Info("Skipping test", "filter", opt.FilterTests)
			continue
		}

		infra := make([]config.ServiceConfig, 0, len(cfg.Infrastructure)+len(t.Infrastructure))
		for _, v := range cfg.Infrastructure {
			infra = append(infra, v)
		}
		for _, v := range t.Infrastructure {
			infra = append(infra, v)
		}

		toolNames := slices.Collect(maps.Keys(cfg.Tools))
		for k := range t.Tools {
			if _, ok := cfg.Tools[k]; !ok {
				toolNames = append(toolNames, k)
			}
		}
		slices.Sort(toolNames)

		for _, tool := range toolNames {
			logger = logger.With("tool", tool)

			if len(opt.FilterTools) > 0 && !slices.Contains(opt.FilterTools, tool) {
				logger.Info("Skipping tool")
				continue
			}

			tools := make([]config.ServiceConfig, 0)
			if cfg.Tools != nil {
				toolCfg, ok := cfg.Tools[tool]
				if ok {
					tools = append(tools, toolCfg)
				}
			}
			if t.Tools != nil {
				toolCfg, ok := t.Tools[tool]
				if ok {
					tools = append(tools, toolCfg)
				}
			}

			collectors := make([]metrics.Collector, 0, len(cfg.Metrics))
			for name, v := range cfg.Metrics {
				if len(v.Tools) > 0 && !slices.Contains(v.Tools, tool) {
					logger.Debug("Skipping collector", "collector", name)
					continue
				}

				collector, err := metrics.NewCollector(logger, name, v.Collector)
				if err != nil {
					return nil, fmt.Errorf("failed to create collector %s: %w", name, err)
				}
				err = collector.Configure(v.Settings)
				if err != nil {
					return nil, fmt.Errorf("failed to configure collector %s: %w", name, err)
				}
				collectors = append(collectors, collector)
			}

			runs = append(runs, &TestRunner{
				infrastructure: infra,
				tools:          tools,
				collectors:     collectors,

				name:     t.Name,
				duration: t.Duration,
				hooks:    t.Steps,

				tool:         tool,
				resultsDir:   filepath.Join(opt.ResultsDir, fmt.Sprintf("%s_%s_%s", opt.StartedAt.Format("20060102150405"), t.Name, tool)),
				dockerClient: opt.DockerClient,

				logger: logger,
			})
		}
	}

	return runs, nil
}

// TestRunner is a single test run for a single tool.
type TestRunner struct {
	step Step

	infrastructure []config.ServiceConfig
	tools          []config.ServiceConfig
	collectors     []metrics.Collector

	name     string
	duration time.Duration
	hooks    config.TestHooks

	tool         string // tool is the name of the tool to run the test with
	resultsDir   string // resultsDir is the directory where the test results are stored
	dockerClient client.APIClient

	logger     *slog.Logger
	cleanupFns []func(context.Context) error
}

//go:generate stringer -type=Step -linecomment

type Step int

const (
	StepPreInfrastructure  Step = iota // pre-infrastructure
	StepInfrastructure                 // infrastructure
	StepPostInfrastructure             // post-infrastructure
	StepPreTool                        // pre-tool
	StepTool                           // tool
	StepPostTool                       // post-tool
	StepPreTest                        // pre-test
	StepTest                           // test
	StepPostTest                       // post-test
	StepPreCleanup                     // pre-cleanup
	StepCleanup                        // cleanup
	StepPostCleanup                    // post-cleanup
	StepDone                           // done
)

func (r *TestRunner) Name() string {
	return r.name
}

func (r *TestRunner) Tool() string {
	return r.tool
}

func (r *TestRunner) Duration() time.Duration {
	return r.duration
}

func (r *TestRunner) Infrastructure() []config.ServiceConfig {
	return r.infrastructure
}

func (r *TestRunner) Tools() []config.ServiceConfig {
	return r.tools
}

func (r *TestRunner) Collectors() []metrics.Collector {
	return r.collectors
}

// Run runs the test to completion.
func (r *TestRunner) Run(ctx context.Context) error {
	var errs []error
	for {
		currentStep := r.Step()
		if currentStep == StepDone {
			break
		}

		err := r.RunStep(ctx)
		if err != nil {
			err = fmt.Errorf("error in step %s: %w", currentStep, err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *TestRunner) Step() Step {
	return r.step
}

// RunStep runs a single step in the test. This allows the caller to step
// through the test one step at a time, updating the user interface as it goes.
func (r *TestRunner) RunStep(ctx context.Context) error {
	var err error
	switch r.step {
	case StepPreInfrastructure:
		err = r.runPreInfrastructure(ctx)
	case StepInfrastructure:
		err = r.runInfrastructure(ctx)
	case StepPostInfrastructure:
		err = r.runPostInfrastructure(ctx)
	case StepPreTool:
		err = r.runPreTool(ctx)
	case StepTool:
		err = r.runTool(ctx)
	case StepPostTool:
		err = r.runPostTool(ctx)
	case StepPreTest:
		err = r.runPreTest(ctx)
	case StepTest:
		err = r.runTest(ctx)
	case StepPostTest:
		err = r.runPostTest(ctx)
	case StepPreCleanup:
		err = r.runPreCleanup(ctx)
	case StepCleanup:
		err = r.runCleanup(ctx)
	case StepPostCleanup:
		err = r.runPostCleanup(ctx)
	case StepDone:
		// noop
	}

	r.step = r.nextStep(r.step, err)
	return err
}

// -- STEPS --------------------------------------------------------------------

func (r *TestRunner) runPreInfrastructure(ctx context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	if _, err := os.Stat(r.resultsDir); err == nil {
		return fmt.Errorf("output folder %q already exists", r.resultsDir)
	}
	if err := os.MkdirAll(r.resultsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output folder %q: %w", r.resultsDir, err)
	}

	// TODO pull all images

	return r.runHooks(ctx, logger, r.hooks.PreInfrastructure)
}

func (r *TestRunner) runInfrastructure(ctx context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	paths := r.collectDockerComposeFiles(r.infrastructure)
	if len(paths) == 0 {
		logger.Info("No infrastructure to start")
		return nil
	}

	logPath := filepath.Join(r.resultsDir, "infrastructure.log")

	err = r.dockerComposeUpWait(ctx, logger, paths, logPath)
	if err != nil {
		return fmt.Errorf("failed to start infrastructure: %w", err)
	}

	logger.Info("Infrastructure started")
	return nil
}

func (r *TestRunner) runPostInfrastructure(ctx context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	return r.runHooks(ctx, logger, r.hooks.PostInfrastructure)
}

func (r *TestRunner) runPreTool(ctx context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	return r.runHooks(ctx, logger, r.hooks.PreTool)
}

func (r *TestRunner) runTool(ctx context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	paths := r.collectDockerComposeFiles(r.tools)
	if len(paths) == 0 {
		logger.Info("No tools to start")
		return nil
	}

	logPath := filepath.Join(r.resultsDir, "tools.log")

	err = r.dockerComposeUpWait(ctx, logger, paths, logPath)
	if err != nil {
		return fmt.Errorf("failed to start tools: %w", err)
	}

	logger.Info("Tools started")
	return nil
}

func (r *TestRunner) runPostTool(ctx context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	return r.runHooks(ctx, logger, r.hooks.PostTool)
}

func (r *TestRunner) runPreTest(ctx context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	return r.runHooks(ctx, logger, r.hooks.PreTest)
}

func (r *TestRunner) runTest(ctx context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	endTestAt := time.Now().Add(r.duration + 500*time.Millisecond) // Add 500ms to account for time drift and nicer log output
	testCtx, cancel := context.WithDeadline(ctx, endTestAt)
	defer cancel()

	wg := pool.New().WithErrors().WithContext(testCtx).WithCancelOnError()
	for _, collector := range r.collectors {
		logger.Info("Running collector", "name", collector.Name(), "type", collector.Type())
		wg.Go(func(ctx context.Context) error {
			err := collector.Run(ctx)
			if err != nil {
				return fmt.Errorf("collector %s failed: %w", collector.Name(), err)
			}
			return nil
		})
	}
	wg.Go(func(ctx context.Context) error {
		const timeBetweenLogs = 5 * time.Second
		logTicker := time.NewTicker(timeBetweenLogs)
		defer logTicker.Stop()

		for {
			logger.Info("Test in progress", "time-left", time.Until(endTestAt).Truncate(time.Second))
			select {
			case <-ctx.Done():
				if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return ctx.Err()
				}
				return nil
			case <-logTicker.C:
				continue
			}
		}
	})

	// TODO during

	err = wg.Wait()
	if errors.Is(err, context.DeadlineExceeded) {
		err = nil // not an error
	}

	return err
}

func (r *TestRunner) runPostTest(ctx context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	var errs []error
	for _, collector := range r.collectors {
		err := r.exportMetricsCSV(logger, collector)
		if err != nil {
			logger.Error("Failed to export collector metrics", "name", collector.Name(), "error", err)
			errs = append(errs, fmt.Errorf("failed to export collector metrics %s: %w", collector.Name(), err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return r.runHooks(ctx, logger, r.hooks.PostTest)
}

func (r *TestRunner) runPreCleanup(ctx context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	return r.runHooks(ctx, logger, r.hooks.PreCleanup)
}

func (r *TestRunner) runCleanup(ctx context.Context) (err error) {
	_, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	var errs []error
	for _, fn := range slices.Backward(r.cleanupFns) {
		errs = append(errs, fn(ctx))
	}
	return errors.Join(errs...)
}

func (r *TestRunner) runPostCleanup(ctx context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	return r.runHooks(ctx, logger, r.hooks.PostCleanup)
}

// -- UTILS --------------------------------------------------------------------

func (r *TestRunner) collectDockerComposeFiles(cfgs []config.ServiceConfig) []string {
	paths := make([]string, len(cfgs))
	for i, cfg := range cfgs {
		paths[i] = cfg.DockerCompose
	}
	return paths
}

// prepareStep returns a logger for the given step name and a function that
// logs the result of the step. The function should be deferred.
func (r *TestRunner) loggerForStep(s Step) (*slog.Logger, func(error)) {
	logger := r.logger.With("step", s.String())

	logger.Info("Running step")
	return logger, func(err error) {
		if err != nil {
			logger.Error("Step failed", "error", err)
		} else {
			logger.Info("Step successful")
		}
	}
}

func (r *TestRunner) nextStep(s Step, err error) Step {
	if s == StepDone {
		return StepDone // no next step
	}
	// If an error occurred, we skip to the cleanup step.
	if err != nil && s < StepPreCleanup {
		return StepPreCleanup
	}
	return s + 1
}

func (r *TestRunner) dockerComposeUpWait(
	ctx context.Context,
	logger *slog.Logger,
	dockerComposeFiles []string,
	logPath string,
) error {
	f, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	// Close file in cleanup
	r.cleanupFns = append(r.cleanupFns, func(ctx context.Context) error {
		return f.Close()
	})

	var composeUpErr atomic.Pointer[error]
	go func() {
		err := dockerutil.ComposeUp(
			ctx,
			dockerutil.ComposeOptions{
				File:   dockerComposeFiles,
				Stdout: f,
				Stderr: f,
			},
			dockerutil.ComposeUpOptions{},
		)
		if err != nil && !errors.Is(err, context.Canceled) {
			composeUpErr.Store(&err)
			slog.Error("docker compose up failed", "error", err)
		}
	}()

	r.cleanupFns = append(r.cleanupFns, func(ctx context.Context) error {
		logger.Info("Stopping containers")
		return dockerutil.ComposeDown(
			ctx,
			dockerutil.ComposeOptions{
				File: dockerComposeFiles,
			},
			dockerutil.ComposeDownOptions{},
		)
	})

	logger.Info("Waiting for containers to start")
	var containers []string

	for range 30 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}

		var buf bytes.Buffer
		err = dockerutil.ComposePs(
			ctx,
			dockerutil.ComposeOptions{
				File:   dockerComposeFiles,
				Stdout: &buf,
			},
			dockerutil.ComposePsOptions{
				Quiet: ptr(true),
			},
		)
		if err != nil {
			return fmt.Errorf("failed to list containers: %w", err)
		}
		containers = strings.Fields(buf.String())
		if len(containers) > 0 {
			break
		}
	}

	logger.Info(fmt.Sprintf("Identified %d containers", len(containers)))
	if err := composeUpErr.Load(); err != nil && *err != nil {
		return fmt.Errorf("failed to start containers: %w", *err)
	}

	// We create a context deadline of 5 minutes here, since we expect any
	// service to start
	//  the docker compose file. But maybe a long timeout would be good, to be safe.
	ctx, cancel := context.WithTimeout(ctx, time.Minute*5)
	defer cancel()

	wg := pool.New().WithErrors()
	for _, c := range containers {
		wg.Go(func() error {
			for {
				resp, err := r.dockerClient.ContainerInspect(ctx, c)
				if err != nil {
					return err
				}
				logger.Debug("Inspected container", "container", c, "response", resp, "state", *resp.State)

				switch {
				case resp.State.Dead || (!resp.State.Running && resp.State.ExitCode != 0):
					return fmt.Errorf("container %s is dead (exit code: %d, error: %q)", resp.Name, resp.State.ExitCode, resp.State.Error)
				case !resp.State.Running && resp.State.ExitCode == 0:
					logger.Warn("Container exited with code 0 (assuming it's an init container)", "container", resp.Name)
					return nil
				case resp.State.Health != nil && strings.EqualFold(resp.State.Health.Status, "healthy"):
					logger.Info("Container is healthy", "container", resp.Name)
					return nil
				case resp.State.Health == nil && resp.State.Running:
					logger.Info("Container is running (consider adding a health check!)", "container", resp.Name)
					return nil
				}

				logger.Info("Waiting for container status to be healthy", "container", resp.Name, "status", resp.State.Health.Status)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Second):
				}
				continue
			}
		})
	}

	err = wg.Wait()
	if err != nil {
		return fmt.Errorf("failed to start containers: %w", err)
	}

	return nil
}

func (r *TestRunner) exportMetricsCSV(logger *slog.Logger, collector metrics.Collector) error {
	path := filepath.Join(r.resultsDir, fmt.Sprintf("%s.csv", collector.Name()))
	logger.Info("Exporting metrics", "collector", collector.Name(), "path", path)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("error creating file: %w", err)
	}
	defer f.Close()

	header := []string{"time"}

	metricNames := slices.Collect(maps.Keys(collector.Metrics()))
	slices.Sort(metricNames)
	header = append(header, metricNames...)

	records := make([][]string, 0)
	for col, name := range metricNames {
		series := collector.Metrics()[name]
		for i, sample := range series {
			if len(records) <= i {
				records = append(records, make([]string, len(metricNames)+1))
				records[i][0] = sample.At.Format(time.RFC3339)
			}
			records[i][col+1] = fmt.Sprintf("%f", sample.Value)
		}
	}

	writer := csv.NewWriter(f)
	err = writer.Write(header)
	if err != nil {
		return fmt.Errorf("error writing header: %w", err)
	}

	return writer.WriteAll(records)
}

func (r *TestRunner) runHooks(ctx context.Context, logger *slog.Logger, hooks []config.TestHook) error {
	logger.Debug("Running hooks", "count", len(hooks))
	for _, hook := range hooks {
		if len(hook.Tools) > 0 && !slices.Contains(hook.Tools, r.tool) {
			logger.Debug("Skipping hook", "hook", hook.Name, "tools", hook.Tools)
			continue
		}
		err := r.runHook(ctx, logger, hook)
		if err != nil {
			return fmt.Errorf("failed to run hook %s:%s: %w", r.step, hook.Name, err)
		}
	}
	return nil
}

func (r *TestRunner) runHook(ctx context.Context, logger *slog.Logger, hook config.TestHook) error {
	logger = logger.With("hook", hook.Name)

	if hook.Container != "" {
		slog.Info("Running command in existing container", "container", hook.Container, "command", hook.Run)
		return dockerutil.RunInContainer(ctx, logger, hook.Container, hook.Run)
	}
	image := hook.Image
	if hook.Image == "" {
		image = DefaultHookImage
	}

	logger.Info("Running command in temporary container", "image", image, "command", hook.Run)
	return dockerutil.RunInDockerNetwork(ctx, logger, image, NetworkName, hook.Run)
}

func ptr[T any](v T) *T {
	return &v
}
