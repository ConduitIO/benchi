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
	"strings"
)

// RunInDockerNetwork executes a command in a temporary container connected to
// the specified Docker network. The container will use the alpine image and
// will be removed after execution.
func RunInDockerNetwork(ctx context.Context, cmd *exec.Cmd, networkName string) error {
	// Prepare the command string, preserving quotes and spaces
	cmdStr := make([]string, len(cmd.Args))
	for i, arg := range cmd.Args {
		if strings.Contains(arg, " ") {
			cmdStr[i] = fmt.Sprintf("'%s'", arg)
		} else {
			cmdStr[i] = arg
		}
	}

	// Build the docker run command
	dockerCmd := exec.CommandContext(ctx, "docker",
		"run",
		"--rm",                   // Remove container after execution
		"--network", networkName, // Connect to specified network
		"alpine",   // Use alpine as base image
		"sh", "-c", // Run command through shell
		strings.Join(cmdStr, " "), // The actual command to run
	)

	// Forward stdin, stdout, and stderr
	dockerCmd.Stdin = cmd.Stdin
	dockerCmd.Stdout = cmd.Stdout
	dockerCmd.Stderr = cmd.Stderr

	return execCmd(cmd)
}
