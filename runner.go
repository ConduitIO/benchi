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
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/conduitio/benchi/config"
	"github.com/conduitio/benchi/dockerutil"
)

type RunOptions struct {
	// Dir is the working directory where the test is run. All relative paths
	// are resolved relative to this directory.
	Dir         string
	OutPath     string
	FilterTests []string
}

func Run(ctx context.Context, cfg config.Config, opt RunOptions) error {
	if opt.Dir != "" {
		ctx = dockerutil.ContextWithDir(ctx, opt.Dir)
	}

	// Create output directory if it does not exist.
	err := os.MkdirAll(opt.OutPath, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	for i, t := range cfg.Tests {
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

		for _, tool := range toolNames {
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

			err := testRun{
				Infrastructure: infra,
				Tools:          tools,
				Metrics:        metrics,

				Name:     t.Name,
				Duration: t.Duration,
				Steps:    t.Steps,

				Tool:    tool,
				OutPath: filepath.Join(opt.OutPath, fmt.Sprintf("%s_%s", time.Now().Format("20060102"), tool)),
			}.Run(ctx)
			if err != nil {
				return fmt.Errorf("failed to run test %d (%s): %w", i, tool, err)
			}
		}
	}
	return nil
}

// testRun is a single test run for a single tool.
type testRun struct {
	Infrastructure []config.ServiceConfig
	Tools          []config.ServiceConfig
	Metrics        []config.MetricsCollector

	Name     string
	Duration time.Duration
	Steps    config.TestSteps

	Tool    string // Tool is the name of the tool to run the test with
	OutPath string // OutPath is the directory where the test results are stored
}

func (r testRun) Run(ctx context.Context) error {
	steps := map[string]func(context.Context) error{
		"pre-infrastructure":  r.preInfrastructure,
		"infrastructure":      r.infrastructure,
		"post-infrastructure": r.postInfrastructure,
		"pre-tool":            r.preTool,
		"tool":                r.tool,
		"post-tool":           r.postTool,
		"pre-test":            r.preTest,
		"test":                r.test,
		"post-test":           r.postTest,
		"pre-cleanup":         r.preCleanup,
		"cleanup":             r.cleanup,
		"post-cleanup":        r.postCleanup,
	}

	// Run each step
	for name, step := range steps {
		slog.Info("running step", "step", name)
		if err := step(ctx); err != nil {
			slog.Error("step failed", "step", name, "error", err)
			return fmt.Errorf("step %s failed: %w", name, err)
		}
		slog.Info("step successful", "step", name)
	}

	return nil
}

func (r testRun) preInfrastructure(ctx context.Context) error {
	return nil
}

func (r testRun) infrastructure(ctx context.Context) error {
	paths := r.collectDockerComposeFiles(r.Infrastructure)

	return dockerutil.ComposeUp(
		ctx,
		dockerutil.ComposeOptions{
			File: paths,
		},
		dockerutil.UpOptions{},
	)
}

func (r testRun) postInfrastructure(ctx context.Context) error {
	return nil
}

func (r testRun) preTool(ctx context.Context) error {
	return nil
}

func (r testRun) tool(ctx context.Context) error {
	return nil
}

func (r testRun) postTool(ctx context.Context) error {
	return nil
}

func (r testRun) preTest(ctx context.Context) error {
	return nil
}

func (r testRun) test(ctx context.Context) error {
	// TODO during
	return nil
}

func (r testRun) postTest(ctx context.Context) error {
	return nil
}

func (r testRun) preCleanup(ctx context.Context) error {
	return nil
}

func (r testRun) cleanup(ctx context.Context) error {
	paths := r.collectDockerComposeFiles(r.Infrastructure)

	return dockerutil.ComposeDown(
		ctx,
		dockerutil.ComposeOptions{
			File: paths,
		},
		dockerutil.DownOptions{},
	)
}

func (r testRun) postCleanup(ctx context.Context) error {
	return nil
}

func (r testRun) collectDockerComposeFiles(cfgs []config.ServiceConfig) []string {
	paths := make([]string, len(cfgs))
	for i, cfg := range cfgs {
		paths[i] = cfg.DockerCompose
	}
	return paths
}
