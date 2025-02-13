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
	"fmt"
	"os"
	"os/exec"
)

// ComposeOptions represents the options for the compose command.
type ComposeOptions struct {
	AllResources     *bool    //     --all-resources              Include all resources, even those not used by services
	Ansi             *string  //     --ansi string                Control when to print ANSI control characters ("never"|"always"|"auto") (default "auto")
	Compatibility    *bool    //     --compatibility              Run compose in backward compatibility mode
	DryRun           *bool    //     --dry-run                    Execute command in dry run mode
	EnvFile          []string //     --env-file stringArray       Specify an alternate environment file
	File             []string // -f, --file stringArray           Compose configuration files
	Parallel         *int     //     --parallel int               Control max parallelism, -1 for unlimited (default -1)
	Profile          []string //     --profile stringArray        Specify a profile to enable
	Progress         *string  //     --progress string            Set type of progress output (auto, tty, plain, json, quiet) (default "auto")
	ProjectDirectory *string  //     --project-directory string   Specify an alternate working directory
	ProjectName      *string  // -p, --project-name string        Project name
}

func (opt ComposeOptions) flags() []string {
	var flags []string
	if opt.AllResources != nil && *opt.AllResources {
		flags = append(flags, "--all-resources")
	}
	if opt.Ansi != nil {
		flags = append(flags, "--ansi", *opt.Ansi)
	}
	if opt.Compatibility != nil && *opt.Compatibility {
		flags = append(flags, "--compatibility")
	}
	if opt.DryRun != nil && *opt.DryRun {
		flags = append(flags, "--dry-run")
	}
	for _, envFile := range opt.EnvFile {
		flags = append(flags, "--env-file", envFile)
	}
	for _, file := range opt.File {
		flags = append(flags, "-f", file)
	}
	if opt.Parallel != nil {
		flags = append(flags, "--parallel", fmt.Sprintf("%d", *opt.Parallel))
	}
	for _, profile := range opt.Profile {
		flags = append(flags, "--profile", profile)
	}
	if opt.Progress != nil {
		flags = append(flags, "--progress", *opt.Progress)
	}
	if opt.ProjectDirectory != nil {
		flags = append(flags, "--project-directory", *opt.ProjectDirectory)
	}
	if opt.ProjectName != nil {
		flags = append(flags, "--project-name", *opt.ProjectName)
	}
	return flags
}

func composeCmd(ctx context.Context, composeOpt ComposeOptions) *exec.Cmd {
	args := []string{"docker", "compose"}
	args = append(args, composeOpt.flags()...)

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Dir = DirFromContext(ctx)

	return cmd
}
