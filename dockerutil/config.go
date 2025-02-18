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

// ComposeConfig parses, resolves, and renders the compose file in canonical format.
//
// ComposeOptions contains general compose config like project name and files.
// ComposeConfigOptions contains options specific to the config command like format and output.
//
// Returns error if the compose command fails.
func ComposeConfig(ctx context.Context, composeOpt ComposeOptions, configOpt ComposeConfigOptions) error {
	cmd := configCmd(ctx, composeOpt, configOpt)
	return execCmd(cmd)
}

// ComposeConfigOptions represents the options for the config command.
type ComposeConfigOptions struct {
	DryRun              *bool   //     --dry-run                 Execute command in dry run mode
	Environment         *bool   //     --environment             Print environment used for interpolation.
	Format              *string //     --format string           Format the output. Values: [yaml | json] (default "yaml")
	Hash                *string //     --hash string             Print the service config hash, one per line.
	Images              *bool   //     --images                  Print the image names, one per line.
	NoConsistency       *bool   //     --no-consistency          Don't check model consistency - warning: may produce invalid Compose output
	NoInterpolate       *bool   //     --no-interpolate          Don't interpolate environment variables
	NoNormalize         *bool   //     --no-normalize            Don't normalize compose model
	NoPathResolution    *bool   //     --no-path-resolution      Don't resolve file paths
	Output              *string // -o, --output string           Save to file (default to stdout)
	Profiles            *bool   //     --profiles                Print the profile names, one per line.
	Quiet               *bool   // -q, --quiet                   Only validate the configuration, don't print anything
	ResolveImageDigests *bool   //     --resolve-image-digests   Pin image tags to digests
	Services            *bool   //     --services                Print the service names, one per line.
	Variables           *bool   //     --variables               Print model variables and default values.
	Volumes             *bool   //     --volumes                 Print the volume names, one per line.
}

func (opt ComposeConfigOptions) flags() []string {
	var flags []string
	if opt.DryRun != nil && *opt.DryRun {
		flags = append(flags, "--dry-run")
	}
	if opt.Environment != nil && *opt.Environment {
		flags = append(flags, "--environment")
	}
	if opt.Format != nil {
		flags = append(flags, "--format", *opt.Format)
	}
	if opt.Hash != nil {
		flags = append(flags, "--hash", *opt.Hash)
	}
	if opt.Images != nil && *opt.Images {
		flags = append(flags, "--images")
	}
	if opt.NoConsistency != nil && *opt.NoConsistency {
		flags = append(flags, "--no-consistency")
	}
	if opt.NoInterpolate != nil && *opt.NoInterpolate {
		flags = append(flags, "--no-interpolate")
	}
	if opt.NoNormalize != nil && *opt.NoNormalize {
		flags = append(flags, "--no-normalize")
	}
	if opt.NoPathResolution != nil && *opt.NoPathResolution {
		flags = append(flags, "--no-path-resolution")
	}
	if opt.Output != nil {
		flags = append(flags, "--output", *opt.Output)
	}
	if opt.Profiles != nil && *opt.Profiles {
		flags = append(flags, "--profiles")
	}
	if opt.Quiet != nil && *opt.Quiet {
		flags = append(flags, "--quiet")
	}
	if opt.ResolveImageDigests != nil && *opt.ResolveImageDigests {
		flags = append(flags, "--resolve-image-digests")
	}
	if opt.Services != nil && *opt.Services {
		flags = append(flags, "--services")
	}
	if opt.Variables != nil && *opt.Variables {
		flags = append(flags, "--variables")
	}
	if opt.Volumes != nil && *opt.Volumes {
		flags = append(flags, "--volumes")
	}
	return flags
}

func configCmd(ctx context.Context, composeOpt ComposeOptions, configOpt ComposeConfigOptions) *exec.Cmd {
	cmd := composeCmd(ctx, composeOpt)

	cmd.Args = append(cmd.Args, "config")
	cmd.Args = append(cmd.Args, configOpt.flags()...)

	return cmd
}
