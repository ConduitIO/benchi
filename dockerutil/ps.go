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

package dockerutil

import (
	"context"
	"os/exec"
)

// ComposePs lists containers in a docker compose project.
//
// ComposeOptions contains general compose config like project name and files.
// ComposePsOptions contains options specific to the ps command.
//
// Returns error if the compose command fails.
func ComposePs(ctx context.Context, composeOpt ComposeOptions, psOpt ComposePsOptions) error {
	cmd := psCmd(ctx, composeOpt, psOpt)
	return execCmd(cmd)
}

// ComposePsOptions represents the options for the ps command.
type ComposePsOptions struct {
	All      *bool    // -a, --all                 Show all stopped containers (including those created by the run command)
	DryRun   *bool    //     --dry-run             Execute command in dry run mode
	Filter   *string  //     --filter string       Filter services by a property (supported filters: status)
	Format   *string  //     --format string       Format output using a custom template
	NoTrunc  *bool    //     --no-trunc            Don't truncate output
	Orphans  *bool    //     --orphans             Include orphaned services (not declared by project)
	Quiet    *bool    // -q, --quiet               Only display IDs
	Services *bool    //     --services            Display services
	Status   []string //     --status stringArray  Filter services by status

	ServiceNames []string // Optional list of services to show
}

func (opt ComposePsOptions) flags() []string {
	var flags []string
	if opt.All != nil && *opt.All {
		flags = append(flags, "--all")
	}
	if opt.DryRun != nil && *opt.DryRun {
		flags = append(flags, "--dry-run")
	}
	if opt.Filter != nil {
		flags = append(flags, "--filter", *opt.Filter)
	}
	if opt.Format != nil {
		flags = append(flags, "--format", *opt.Format)
	}
	if opt.NoTrunc != nil && *opt.NoTrunc {
		flags = append(flags, "--no-trunc")
	}
	if opt.Orphans != nil && *opt.Orphans {
		flags = append(flags, "--orphans")
	}
	if opt.Quiet != nil && *opt.Quiet {
		flags = append(flags, "--quiet")
	}
	if opt.Services != nil && *opt.Services {
		flags = append(flags, "--services")
	}
	for _, status := range opt.Status {
		flags = append(flags, "--status", status)
	}

	// Add service names at the end if specified
	flags = append(flags, opt.ServiceNames...)

	return flags
}

func psCmd(ctx context.Context, composeOpt ComposeOptions, psOpt ComposePsOptions) *exec.Cmd {
	cmd := composeCmd(ctx, composeOpt)

	cmd.Args = append(cmd.Args, "ps")
	cmd.Args = append(cmd.Args, psOpt.flags()...)

	return cmd
}
