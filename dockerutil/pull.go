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

// ComposePull pulls docker images for a compose project.
//
// ComposeOptions contains general compose config like project name and files.
// ComposePullOptions contains options specific to the pull command.
//
// Returns error if the compose command fails.
func ComposePull(ctx context.Context, composeOpt ComposeOptions, pullOpt ComposePullOptions) error {
	cmd := pullCmd(ctx, composeOpt, pullOpt)
	return execCmd(cmd)
}

// ComposePullOptions represents the options for the pull command.
type ComposePullOptions struct {
	DryRun             *bool   //     --dry-run                Execute command in dry run mode
	IgnoreBuildable    *bool   //     --ignore-buildable       Ignore images that can be built
	IgnorePullFailures *bool   //     --ignore-pull-failures   Pull what it can and ignores images with pull failures
	IncludeDeps        *bool   //     --include-deps           Also pull services declared as dependencies
	Policy             *string //     --policy string          Apply pull policy ("missing"|"always")
	Quiet              *bool   // -q, --quiet                  Pull without printing progress information

	Services []string // Specific services to pull
}

func (opt ComposePullOptions) flags() []string {
	var flags []string

	if opt.DryRun != nil && *opt.DryRun {
		flags = append(flags, "--dry-run")
	}
	if opt.IgnoreBuildable != nil && *opt.IgnoreBuildable {
		flags = append(flags, "--ignore-buildable")
	}
	if opt.IgnorePullFailures != nil && *opt.IgnorePullFailures {
		flags = append(flags, "--ignore-pull-failures")
	}
	if opt.IncludeDeps != nil && *opt.IncludeDeps {
		flags = append(flags, "--include-deps")
	}
	if opt.Policy != nil {
		flags = append(flags, "--policy", *opt.Policy)
	}
	if opt.Quiet != nil && *opt.Quiet {
		flags = append(flags, "--quiet")
	}

	// Append services if specified
	flags = append(flags, opt.Services...)

	return flags
}

func pullCmd(ctx context.Context, composeOpt ComposeOptions, pullOpt ComposePullOptions) *exec.Cmd {
	cmd := composeCmd(ctx, composeOpt)

	cmd.Args = append(cmd.Args, "pull")
	cmd.Args = append(cmd.Args, pullOpt.flags()...)

	return cmd
}
