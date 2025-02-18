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

// ComposeUp starts a docker compose project.
//
// ComposeOptions contains general compose config like project name and files.
// ComposeUpOptions contains options specific to the up command like detach and timeout.
//
// Returns error if the compose command fails.
func ComposeUp(ctx context.Context, composeOpt ComposeOptions, upOpt ComposeUpOptions) error {
	cmd := upCmd(ctx, composeOpt, upOpt)
	return execCmd(cmd)
}

// ComposeUpOptions represents the options for the up command.
type ComposeUpOptions struct {
	AbortOnContainerExit    *bool    //     --abort-on-container-exit      Stops all containers if any container was stopped. Incompatible with -d
	AbortOnContainerFailure *bool    //     --abort-on-container-failure   Stops all containers if any container exited with failure. Incompatible with -d
	AlwaysRecreateDeps      *bool    //     --always-recreate-deps         Recreate dependent containers. Incompatible with --no-recreate.
	Attach                  []string //     --attach stringArray           Restrict attaching to the specified services. Incompatible with --attach-dependencies.
	AttachDependencies      *bool    //     --attach-dependencies          Automatically attach to log output of dependent services
	Build                   *bool    //     --build                        Build images before starting containers
	Detach                  *bool    // -d, --detach                       Detached mode: Run containers in the background
	DryRun                  *bool    //     --dry-run                      Execute command in dry run mode
	ExitCodeFrom            *string  //     --exit-code-from string        Return the exit code of the selected service container. Implies --abort-on-container-exit
	ForceRecreate           *bool    //     --force-recreate               Recreate containers even if their configuration and image haven't changed
	Menu                    *bool    //     --menu                         Enable interactive shortcuts when running attached. Incompatible with --detach.
	NoAttach                []string //     --no-attach stringArray        Do not attach (stream logs) to the specified services
	NoBuild                 *bool    //     --no-build                     Don't build an image, even if it's policy
	NoColor                 *bool    //     --no-color                     Produce monochrome output
	NoDeps                  *bool    //     --no-deps                      Don't start linked services
	NoLogPrefix             *bool    //     --no-log-prefix                Don't print prefix in logs
	NoRecreate              *bool    //     --no-recreate                  If containers already exist, don't recreate them. Incompatible with --force-recreate.
	NoStart                 *bool    //     --no-start                     Don't start the services after creating them
	Pull                    *string  //     --pull string                  Pull image before running ("always"|"missing"|"never") (default "policy")
	QuietPull               *bool    //     --quiet-pull                   Pull without printing progress information
	RemoveOrphans           *bool    //     --remove-orphans               Remove containers for services not defined in the Compose file
	RenewAnonVolumes        *bool    // -V, --renew-anon-volumes           Recreate anonymous volumes instead of retrieving data from the previous containers
	Scale                   *string  //     --scale scale                  Scale SERVICE to NUM instances. Overrides the scale setting in the Compose file if present.
	Timeout                 *int     // -t, --timeout int                  Use this timeout in seconds for container shutdown when attached or when containers are already running
	Timestamps              *bool    //     --timestamps                   Show timestamps
	Wait                    *bool    //     --wait                         Wait for services to be running|healthy. Implies detached mode.
	WaitTimeout             *int     //     --wait-timeout int             Maximum duration in seconds to wait for the project to be running|healthy
	Watch                   *bool    // -w, --watch                        Watch source code and rebuild/refresh containers when files are updated.
	Yes                     *bool    // -y, --y                            Assume "yes" as answer to all prompts and run non-interactively
}

func (opt ComposeUpOptions) flags() []string {
	var flags []string
	if opt.AbortOnContainerExit != nil && *opt.AbortOnContainerExit {
		flags = append(flags, "--abort-on-container-exit")
	}
	if opt.AbortOnContainerFailure != nil && *opt.AbortOnContainerFailure {
		flags = append(flags, "--abort-on-container-failure")
	}
	if opt.AlwaysRecreateDeps != nil && *opt.AlwaysRecreateDeps {
		flags = append(flags, "--always-recreate-deps")
	}
	for _, attach := range opt.Attach {
		flags = append(flags, "--attach", attach)
	}
	if opt.AttachDependencies != nil && *opt.AttachDependencies {
		flags = append(flags, "--attach-dependencies")
	}
	if opt.Build != nil && *opt.Build {
		flags = append(flags, "--build")
	}
	if opt.Detach != nil && *opt.Detach {
		flags = append(flags, "--detach")
	}
	if opt.DryRun != nil && *opt.DryRun {
		flags = append(flags, "--dry-run")
	}
	if opt.ExitCodeFrom != nil {
		flags = append(flags, "--exit-code-from", *opt.ExitCodeFrom)
	}
	if opt.ForceRecreate != nil && *opt.ForceRecreate {
		flags = append(flags, "--force-recreate")
	}
	if opt.Menu != nil && *opt.Menu {
		flags = append(flags, "--menu")
	}
	for _, noAttach := range opt.NoAttach {
		flags = append(flags, "--no-attach", noAttach)
	}
	if opt.NoBuild != nil && *opt.NoBuild {
		flags = append(flags, "--no-build")
	}
	if opt.NoColor != nil && *opt.NoColor {
		flags = append(flags, "--no-color")
	}
	if opt.NoDeps != nil && *opt.NoDeps {
		flags = append(flags, "--no-deps")
	}
	if opt.NoLogPrefix != nil && *opt.NoLogPrefix {
		flags = append(flags, "--no-log-prefix")
	}
	if opt.NoRecreate != nil && *opt.NoRecreate {
		flags = append(flags, "--no-recreate")
	}
	if opt.NoStart != nil && *opt.NoStart {
		flags = append(flags, "--no-start")
	}
	if opt.Pull != nil {
		flags = append(flags, "--pull", *opt.Pull)
	}
	if opt.QuietPull != nil && *opt.QuietPull {
		flags = append(flags, "--quiet-pull")
	}
	if opt.RemoveOrphans != nil && *opt.RemoveOrphans {
		flags = append(flags, "--remove-orphans")
	}
	if opt.RenewAnonVolumes != nil && *opt.RenewAnonVolumes {
		flags = append(flags, "--renew-anon-volumes")
	}
	if opt.Scale != nil {
		flags = append(flags, "--scale", *opt.Scale)
	}
	if opt.Timeout != nil {
		flags = append(flags, "--timeout", fmt.Sprintf("%d", *opt.Timeout))
	}
	if opt.Timestamps != nil && *opt.Timestamps {
		flags = append(flags, "--timestamps")
	}
	if opt.Wait != nil && *opt.Wait {
		flags = append(flags, "--wait")
	}
	if opt.WaitTimeout != nil {
		flags = append(flags, "--wait-timeout", fmt.Sprintf("%d", *opt.WaitTimeout))
	}
	if opt.Watch != nil && *opt.Watch {
		flags = append(flags, "--watch")
	}
	if opt.Yes != nil && *opt.Yes {
		flags = append(flags, "--y")
	}
	return flags
}

func upCmd(ctx context.Context, composeOpt ComposeOptions, upOpt ComposeUpOptions) *exec.Cmd {
	cmd := composeCmd(ctx, composeOpt)

	cmd.Args = append(cmd.Args, "up")
	cmd.Args = append(cmd.Args, upOpt.flags()...)

	return cmd
}
