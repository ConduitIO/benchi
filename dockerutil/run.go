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
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
)

// RunInContainer executes a command in the specified container. The container
// must be running and the command will be executed in the container's default
// shell.
func RunInContainer(ctx context.Context, container, command string) error {
	if command[len(command)-1] != '\n' {
		command += "\n"
	}

	// Build the docker run command
	dockerCmd := exec.CommandContext(ctx, "docker",
		"exec",
		"--interactive", // Keep stdin open
		container,       // The container to run the command in
		"sh",            // Run command through shell
	)

	return runDockerCmd(dockerCmd, container, command)
}

// RunInDockerNetwork executes a command in a temporary container connected to
// the specified Docker network. The container will use the alpine image and
// will be removed after execution.
func RunInDockerNetwork(ctx context.Context, networkName, command string) error {
	// Build the docker run command
	dockerCmd := exec.CommandContext(ctx, "docker",
		"run",
		"--rm",                   // Remove container after execution
		"--interactive",          // Keep stdin open
		"--network", networkName, // Connect to specified network
		"alpine", // Use alpine as base image
		"sh",     // Run command through shell
	)

	return runDockerCmd(dockerCmd, "[temporary]", command)
}

func runDockerCmd(dockerCmd *exec.Cmd, container, stdin string) error {
	// Create pipes for stdin, stdout and stderr
	stdinPipe, err := dockerCmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	stdoutPipe, err := dockerCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderrPipe, err := dockerCmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Start the command
	if err := dockerCmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Write the input string to stdin and close it
	go func() {
		defer stdinPipe.Close()
		_, _ = stdinPipe.Write([]byte(stdin))
	}()

	// Read from stdout
	go func() {
		scn := bufio.NewScanner(stdoutPipe)
		for scn.Scan() {
			slog.Info(scn.Text(), "container", container, "stream", "stdout")
		}
		if err := scn.Err(); err != nil {
			slog.Error("stdout scan error", "container", container, "error", err)
		}
	}()

	// Read from stderr
	go func() {
		scn := bufio.NewScanner(stderrPipe)
		for scn.Scan() {
			slog.Error(scn.Text(), "container", container, "stream", "stderr")
		}
		if err := scn.Err(); err != nil {
			slog.Error("stderr scan error", "container", container, "error", err)
		}
	}()

	// Wait for the command to finish
	return dockerCmd.Wait()
}

func execCmd(cmd *exec.Cmd) error {
	slog.Debug("Executing command", "command", cmd.String())

	return cmd.Run()
}
