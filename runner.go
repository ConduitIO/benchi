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
	"github.com/docker/docker/client"
	"github.com/sourcegraph/conc/pool"
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

func BuildTestRunners(cfg config.Config, opt TestRunnerOptions) TestRunners {
	runs := make(TestRunners, 0, len(cfg.Tests)*len(cfg.Tools))

	for _, t := range cfg.Tests {
		// TODO filter
		infra := make([]config.ServiceConfig, 0, len(cfg.Infrastructure)+len(t.Infrastructure))
		for _, v := range cfg.Infrastructure {
			infra = append(infra, v)
		}
		for _, v := range t.Infrastructure {
			infra = append(infra, v)
		}

		metrics := make([]config.MetricsCollector, 0, len(cfg.Metrics)+len(t.Metrics))
		metrics = append(metrics, cfg.Metrics...)
		metrics = append(metrics, t.Metrics...)

		toolNames := slices.Collect(maps.Keys(cfg.Tools))
		for k := range t.Tools {
			if _, ok := cfg.Tools[k]; !ok {
				toolNames = append(toolNames, k)
			}
		}
		slices.Sort(toolNames)

		for _, tool := range toolNames {
			// TODO filter
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

			runs = append(runs, &TestRunner{
				infrastructure: infra,
				tools:          tools,
				metrics:        metrics,

				name:     t.Name,
				duration: t.Duration,
				steps:    t.Steps,

				tool:         tool,
				resultsDir:   filepath.Join(opt.ResultsDir, fmt.Sprintf("%s_%s_%s", opt.StartedAt.Format("20060102150405"), t.Name, tool)),
				dockerClient: opt.DockerClient,

				logger: slog.Default().With("test", t.Name, "tool", tool),
			})
		}
	}

	return runs
}

// TestRunner is a single test run for a single tool.
type TestRunner struct {
	step Step

	infrastructure []config.ServiceConfig
	tools          []config.ServiceConfig
	metrics        []config.MetricsCollector

	name     string
	duration time.Duration
	steps    config.TestSteps

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

func (r *TestRunner) runPreInfrastructure(context.Context) (err error) {
	_, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	if _, err := os.Stat(r.resultsDir); err == nil {
		return fmt.Errorf("output folder %q already exists", r.resultsDir)
	}
	if err := os.MkdirAll(r.resultsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output folder %q: %w", r.resultsDir, err)
	}

	return nil
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

func (r *TestRunner) runPostInfrastructure(context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	_ = logger

	return nil
}

func (r *TestRunner) runPreTool(context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	_ = logger

	return nil
}

func (r *TestRunner) runTool(ctx context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	_ = logger

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

func (r *TestRunner) runPostTool(context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	_ = logger

	return nil
}

func (r *TestRunner) runPreTest(context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	_ = logger

	return nil
}

func (r *TestRunner) runTest(ctx context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	const timeBetweenLogs = 5 * time.Second

	endTestAt := time.Now().Add(r.duration + 500*time.Millisecond) // Add 500ms to account for time drift and nicer log output
	testCompleted := time.After(r.duration)
	logTicker := time.NewTicker(timeBetweenLogs)
	defer logTicker.Stop()

	// TODO during

	for {
		logger.Info("Test in progress", "time-left", endTestAt.Sub(time.Now()).Truncate(time.Second))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-logTicker.C:
			continue
		case <-testCompleted:
		}
		break
	}

	return nil
}

func (r *TestRunner) runPostTest(context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	_ = logger

	return nil
}

func (r *TestRunner) runPreCleanup(context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	_ = logger

	return nil
}

func (r *TestRunner) runCleanup(ctx context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	_ = logger

	var errs []error
	for _, fn := range slices.Backward(r.cleanupFns) {
		errs = append(errs, fn(ctx))
	}
	return errors.Join(errs...)
}

func (r *TestRunner) runPostCleanup(context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	_ = logger

	return nil
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

	wg := pool.New().WithErrors()
	for _, c := range containers {
		wg.Go(func() error {
			for {
				resp, err := r.dockerClient.ContainerInspect(ctx, c)
				if err != nil {
					return err
				}
				switch {
				case resp.State.Dead:
					return fmt.Errorf("container %s is dead", resp.Name)
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

func ptr[T any](v T) *T {
	return &v
}
