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
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/conduitio/benchi/config"
	"github.com/conduitio/benchi/dockerutil"
	"github.com/conduitio/benchi/metrics"
	"github.com/conduitio/benchi/metrics/conduit"
	"github.com/conduitio/benchi/metrics/docker"
	"github.com/conduitio/benchi/metrics/kafka"
	"github.com/conduitio/benchi/metrics/prometheus"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/sourcegraph/conc/pool"
)

const (
	NetworkName      = "benchi"
	DefaultHookImage = "alpine:latest"
)

type TestRunnerOptions struct {
	// ResultsDir is the directory where the test results are stored.
	ResultsDir string
	// FilterTests is a list of test names to run. If empty, all tests are run.
	FilterTests []string
	// FilterTools is a list of tool names to run. If empty, all tools are run.
	FilterTools []string
	// DockerClient is the Docker client to use for running the tests.
	DockerClient client.APIClient
}

var registerCollectorsOnce sync.Once

//nolint:funlen,gocognit // It's a bit longer, but still readable.
func BuildTestRunners(cfg config.Config, opt TestRunnerOptions) (TestRunners, error) {
	registerCollectorsOnce.Do(func() {
		conduit.Register()
		prometheus.Register()
		kafka.Register()
		docker.Register(opt.DockerClient)
	})

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

		var infraContainers []string
		if len(infra) > 0 {
			var err error
			infraContainers, err = findContainerNames(context.Background(), collectDockerComposeFiles(infra))
			if err != nil {
				return nil, fmt.Errorf("failed to find infrastructure container names: %w", err)
			}
		}

		toolNames := slices.Collect(maps.Keys(cfg.Tools))
		for k := range t.Tools {
			if _, ok := cfg.Tools[k]; !ok {
				toolNames = append(toolNames, k)
			}
		}
		slices.Sort(toolNames)

		for _, tool := range toolNames {
			logger := logger.With("tool", tool)

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

			var toolContainers []string
			if len(tools) > 0 {
				var err error
				toolContainers, err = findContainerNames(context.Background(), collectDockerComposeFiles(tools))
				if err != nil {
					return nil, fmt.Errorf("failed to find tool container names: %w", err)
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

				infrastructureContainers: infraContainers,
				toolContainers:           toolContainers,

				name:     t.Name,
				duration: t.Duration,
				hooks:    t.Steps,

				tool:         tool,
				resultsDir:   filepath.Join(opt.ResultsDir, fmt.Sprintf("%s_%s", t.Name, tool)),
				dockerClient: opt.DockerClient,

				logger: logger,
			})
		}
	}

	return runs, nil
}

func findContainerNames(ctx context.Context, files []string) ([]string, error) {
	var buf bytes.Buffer
	err := dockerutil.ComposeConfig(
		ctx,
		dockerutil.ComposeOptions{
			File:   files,
			Stdout: &buf,
			Stderr: &buf,
		},
		dockerutil.ComposeConfigOptions{
			Format: ptr("json"),
		},
	)
	if err != nil {
		slog.Error("Failed to run docker compose config", "output", buf.String())
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

type TestRunners []*TestRunner

func (runners TestRunners) ExportAggregatedMetrics(resultsDir string) error {
	path := filepath.Join(resultsDir, "aggregated-results.csv")
	slog.Info("Exporting aggregated metrics", "path", path)

	headers, records := runners.aggregatedMetrics()

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("error creating file: %w", err)
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	err = writer.Write(headers)
	if err != nil {
		return fmt.Errorf("error writing CSV header: %w", err)
	}

	err = writer.WriteAll(records)
	if err != nil {
		return fmt.Errorf("error writing CSV records: %w", err)
	}

	err = writer.Error()
	if err != nil {
		return fmt.Errorf("error writing CSV records: %w", err)
	}

	return nil
}

func (runners TestRunners) aggregatedMetrics() (headers []string, records [][]string) {
	headers = []string{"test", "tool"}

	// colIndexes maps the collector name and metric name to the column index in
	// the records and headers slices.
	colIndexes := make(map[string]map[string]int)

	for _, tr := range runners {
		recs := make([]string, len(headers))
		recs[0] = tr.Name()
		recs[1] = tr.Tool()

		for _, c := range tr.Collectors() {
			indexes := colIndexes[c.Name()]
			if indexes == nil {
				indexes = make(map[string]int)
				colIndexes[c.Name()] = indexes
			}

			for _, r := range c.Results() {
				idx := indexes[r.Name]
				if idx == 0 {
					idx = len(headers)
					indexes[r.Name] = idx
					headers = append(headers, r.Name)
					// Backfill records with empty values.
					for i := 0; i < len(records); i++ {
						records[i] = append(records[i], "")
					}
					recs = append(recs, "") //nolint:makezero // False positive.
				}

				mean, ok := runners.trimmedMean(r.Samples)
				if ok {
					recs[idx] = fmt.Sprintf("%.2f", mean)
				}
			}
		}
		records = append(records, recs)
	}

	return headers, records
}

// trimmedMean calculates the trimmed mean of the samples. It trims any zeros
// from the start and end of the samples, then trims any samples that are more
// than 2 standard deviations from the mean. It returns the trimmed mean and a
// boolean indicating if the trimmed mean was calculated successfully.
func (runners TestRunners) trimmedMean(samples []metrics.Sample) (float64, bool) {
	if len(samples) == 0 {
		return 0, false
	}

	// Trim any zeros from the start and end of the samples.
	for len(samples) > 0 && samples[0].Value == 0 {
		samples = samples[1:]
	}
	for len(samples) > 0 && samples[len(samples)-1].Value == 0 {
		samples = samples[:len(samples)-1]
	}

	if len(samples) == 0 {
		return 0, true
	}

	n := float64(len(samples)) // Number of samples as a float.

	// Calculate mean and standard deviation
	var sum float64
	for _, s := range samples {
		sum += s.Value
	}
	mean := sum / n

	var variance float64
	for _, s := range samples {
		variance += (s.Value - mean) * (s.Value - mean)
	}
	stddev := math.Sqrt(variance / n)

	// Trim samples that are more than 2 standard deviations from the mean.
	var trimmed []float64
	lowerBound := mean - 2*stddev
	upperBound := mean + 2*stddev
	for _, s := range samples {
		if s.Value >= lowerBound && s.Value <= upperBound {
			trimmed = append(trimmed, s.Value)
		}
	}

	// Calculate the trimmed mean.
	var trimmedSum float64
	for _, s := range trimmed {
		trimmedSum += s
	}

	return trimmedSum / float64(len(trimmed)), true
}

// TestRunner is a single test run for a single tool.
type TestRunner struct {
	step Step

	infrastructure []config.ServiceConfig
	tools          []config.ServiceConfig
	collectors     []metrics.Collector

	infrastructureContainers []string
	toolContainers           []string

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

func (r *TestRunner) InfrastructureContainers() []string {
	return r.infrastructureContainers
}

func (r *TestRunner) Tools() []config.ServiceConfig {
	return r.tools
}

func (r *TestRunner) ToolContainers() []string {
	return r.toolContainers
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

	// Pull infrastructure images
	logger.Info("Pulling docker images for infrastructure containers", "containers", r.infrastructureContainers)
	err = dockerutil.ComposePull(
		ctx,
		dockerutil.ComposeOptions{
			File: collectDockerComposeFiles(r.infrastructure),
		},
		dockerutil.ComposePullOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to pull infrastructure images: %w", err)
	}

	// Pull tool images
	logger.Info("Pulling docker images for tool containers", "containers", r.toolContainers)
	err = dockerutil.ComposePull(
		ctx,
		dockerutil.ComposeOptions{
			File: collectDockerComposeFiles(r.tools),
		},
		dockerutil.ComposePullOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to pull tool images: %w", err)
	}

	err = r.pullHookImages(ctx, logger, r.hooks.All())
	if err != nil {
		return fmt.Errorf("failed to pull docker images for hooks: %w", err)
	}

	return r.runHooks(ctx, logger, r.hooks.PreInfrastructure)
}

func (r *TestRunner) runInfrastructure(ctx context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	paths := collectDockerComposeFiles(r.infrastructure)
	if len(paths) == 0 {
		logger.Info("No infrastructure to start")
		return nil
	}

	logPath := filepath.Join(r.resultsDir, "infrastructure.log")

	err = r.dockerComposeUpWait(ctx, logger, paths, r.infrastructureContainers, logPath)
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

	paths := collectDockerComposeFiles(r.tools)
	if len(paths) == 0 {
		logger.Info("No tools to start")
		return nil
	}

	logPath := filepath.Join(r.resultsDir, "tools.log")

	err = r.dockerComposeUpWait(ctx, logger, paths, r.toolContainers, logPath)
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

	if len(r.hooks.During) > 0 {
		// Special case: we run all hooks during the test in parallel.
		logger.Debug("Running hooks", "count", len(r.hooks.During))
		for _, hook := range r.hooks.During {
			if len(hook.Tools) > 0 && !slices.Contains(hook.Tools, r.tool) {
				logger.Debug("Skipping hook", "hook", hook.Name, "tools", hook.Tools)
				continue
			}
			wg.Go(func(ctx context.Context) error {
				err := r.runHook(ctx, logger, hook)
				if err != nil {
					return fmt.Errorf("failed to run hook %q in step %s: %w", hook.Name, r.step, err)
				}
				return nil
			})
		}
	}

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

	errs := make([]error, len(r.cleanupFns))
	for i, fn := range slices.Backward(r.cleanupFns) {
		errs[i] = fn(ctx)
	}
	return errors.Join(errs...)
}

func (r *TestRunner) runPostCleanup(ctx context.Context) (err error) {
	logger, lastLog := r.loggerForStep(r.step)
	defer func() { lastLog(err) }()

	return r.runHooks(ctx, logger, r.hooks.PostCleanup)
}

// -- UTILS --------------------------------------------------------------------

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

//nolint:funlen // It's a bit long, but still readable.
func (r *TestRunner) dockerComposeUpWait(
	ctx context.Context,
	logger *slog.Logger,
	dockerComposeFiles []string,
	containers []string,
	logPath string,
) error {
	f, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	// Close file in cleanup
	r.cleanupFns = append(r.cleanupFns, func(context.Context) error {
		return f.Close()
	})

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

	// We create a context deadline of 5 minutes here, since we expect any
	// service to start in that time and we want to prevent the test from
	// running indefinitely.
	// We also use a context with cancel cause to be able to cancel the context
	// with a specific error in the compose up goroutine, which will continue
	// to run if all goes well.
	monitorCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	//nolint:govet // The cancel function is called in the defer above.
	monitorCtx, _ = context.WithTimeout(monitorCtx, time.Minute*5)

	go func() {
		err := dockerutil.ComposeUp(
			// Note: we use the original context here, not the monitor context,
			// since we want this goroutine to keep running if all goes well.
			ctx,
			dockerutil.ComposeOptions{
				File:   dockerComposeFiles,
				Stdout: f,
				Stderr: f,
			},
			dockerutil.ComposeUpOptions{},
		)
		if err != nil {
			slog.Error("Docker compose up failed", "error", err)
			cancel(fmt.Errorf("docker compose up failed: %w", err))
		}
	}()

	wg := pool.New().WithErrors().WithContext(monitorCtx).WithCancelOnError()

	for _, c := range containers {
		wg.Go(func(ctx context.Context) error { return r.ensureContainerHealthy(ctx, logger, c) })
	}

	err = wg.Wait()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// Check if there is a cause, that would be the error from the
			// compose up goroutine.
			err = context.Cause(monitorCtx)
		}
		return fmt.Errorf("failed to start containers: %w", err)
	}

	return nil
}

func (r *TestRunner) ensureContainerHealthy(ctx context.Context, logger *slog.Logger, container string) error {
	lastStatus := "N/A"
	for {
		logger.Info("Waiting for container status to be healthy", "container", container, "status", lastStatus)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}

		resp, err := r.dockerClient.ContainerInspect(ctx, container)
		if err != nil {
			if errdefs.IsNotFound(err) {
				lastStatus = "N/A"
				slog.Debug("Container not found (probably still being created)", "name", container)
				continue
			}
			return fmt.Errorf("failed to inspect container %s: %w", container, err)
		}
		logger.Debug("Inspected container", "container", container, "response", resp, "state", *resp.State)

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

		lastStatus = resp.State.Health.Status
	}
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

	rr := collector.Results()
	metricNames := make([]string, len(rr))
	for i, result := range rr {
		metricNames[i] = result.Name
	}
	header = append(header, metricNames...)

	records := make([][]string, 0)
	for col, results := range rr {
		series := results.Samples
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
		return fmt.Errorf("error writing CSV header: %w", err)
	}

	err = writer.WriteAll(records)
	if err != nil {
		return fmt.Errorf("error writing CSV records: %w", err)
	}

	err = writer.Error()
	if err != nil {
		return fmt.Errorf("error writing CSV records: %w", err)
	}

	return nil
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
			return fmt.Errorf("failed to run hook %q in step %s: %w", hook.Name, r.step, err)
		}
	}
	return nil
}

func (r *TestRunner) runHook(ctx context.Context, logger *slog.Logger, hook config.TestHook) error {
	logger = logger.With("hook", hook.Name)

	if hook.Container != "" {
		slog.Info("Running command in existing container", "container", hook.Container, "command", hook.Run)
		//nolint:wrapcheck // The utility function is responsible for wrapping the error.
		return dockerutil.RunInContainer(ctx, logger, hook.Container, hook.Run)
	}
	image := hook.Image
	if hook.Image == "" {
		image = DefaultHookImage
	}

	logger.Info("Running command in temporary container", "image", image, "command", hook.Run)
	//nolint:wrapcheck // The utility function is responsible for wrapping the error.
	return dockerutil.RunInDockerNetwork(ctx, logger, image, NetworkName, hook.Run)
}

func (r *TestRunner) pullHookImages(ctx context.Context, logger *slog.Logger, hooks []config.TestHook) error {
	var imgs []string
	for _, hook := range hooks {
		img := hook.Image
		if img == "" {
			img = DefaultHookImage
		}
		if !slices.Contains(imgs, img) {
			imgs = append(imgs, img)
		}
	}

	logger.Info("Pulling docker images for hooks", "images", imgs)
	for _, img := range imgs {
		resp, err := r.dockerClient.ImagePull(ctx, img, image.PullOptions{})
		if err != nil {
			return fmt.Errorf("failed to pull docker image %q: %w", img, err)
		}
		scanner := bufio.NewScanner(resp)
		tmp := make(map[string]any)
		logAttrs := make([]any, 0)
		for scanner.Scan() {
			clear(tmp)
			logAttrs = logAttrs[:0]

			body := scanner.Bytes()
			if err := json.Unmarshal(body, &tmp); err != nil {
				logger.Warn("Failed to unmarshal docker image pull response", "image", img, "error", err)
				tmp["response"] = string(body)
			}

			for k, v := range tmp {
				logAttrs = append(logAttrs, slog.Any(k, v))
			}

			logger.Info("image pull response", logAttrs...)
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("failed to read image pull response: %w", err)
		}
		_ = resp.Close()
	}

	return nil
}

func collectDockerComposeFiles(cfgs []config.ServiceConfig) []string {
	paths := make([]string, len(cfgs))
	for i, cfg := range cfgs {
		paths[i] = cfg.DockerCompose
	}
	return paths
}

func ptr[T any](v T) *T {
	return &v
}
