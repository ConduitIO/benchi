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
	"os/exec"
)

// ComposeDown stops and removes a docker compose project.
//
// ComposeOptions contains general compose config like project name and files.
// DownOptions contains options specific to the down command like volumes and timeout.
//
// Returns error if the compose command fails.
func ComposeDown(ctx context.Context, composeOpt ComposeOptions, downOpt DownOptions) error {
	cmd := downCmd(ctx, composeOpt, downOpt)
	return Exec(cmd)
}

// DownOptions represents the options for the down command.
type DownOptions struct {
	DryRun        *bool   //     --dry-run          Execute command in dry run mode
	RemoveOrphans *bool   //     --remove-orphans   Remove containers for services not defined in the Compose file
	Rmi           *string //     --rmi string       Remove images used by services. "local" remove only images that don't have a custom tag ("local"|"all")
	Timeout       *int    // -t, --timeout int      Specify a shutdown timeout in seconds
	Volumes       *bool   // -v, --volumes          Remove named volumes declared in the "volumes" section
}

func (opt DownOptions) flags() []string {
	var flags []string
	if opt.DryRun != nil && *opt.DryRun {
		flags = append(flags, "--dry-run")
	}
	if opt.RemoveOrphans != nil && *opt.RemoveOrphans {
		flags = append(flags, "--remove-orphans")
	}
	if opt.Rmi != nil {
		flags = append(flags, "--rmi", *opt.Rmi)
	}
	if opt.Timeout != nil {
		flags = append(flags, "-t", fmt.Sprintf("%d", *opt.Timeout))
	}
	if opt.Volumes != nil && *opt.Volumes {
		flags = append(flags, "-v")
	}
	return flags
}

func downCmd(ctx context.Context, composeOpt ComposeOptions, downOpt DownOptions) *exec.Cmd {
	cmd := composeCmd(ctx, composeOpt)

	cmd.Args = append(cmd.Args, "down")
	cmd.Args = append(cmd.Args, downOpt.flags()...)

	return cmd
}
