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
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
)

// RunInContainer executes a command in the specified container. The container
// must be running and the command will be executed in the container's default
// shell.
func RunInContainer(ctx context.Context, logger *slog.Logger, container, command string) error {
	if command[len(command)-1] != '\n' {
		command += "\n"
	}

	// Build the docker run command
	dockerCmd := exec.Command("docker",
		"exec",
		"--interactive",     // Keep stdin open
		container,           // The container to run the command in
		"sh", "-c", command, // Run command through shell
	)

	return runDockerCmd(ctx, logger, dockerCmd, container)
}

// RunInDockerNetwork executes a command in a temporary container connected to
// the specified Docker network. The container will be removed after execution.
func RunInDockerNetwork(ctx context.Context, logger *slog.Logger, image, networkName, command string) error {
	container := randomContainerName()

	// Build the docker run command
	dockerCmd := exec.Command("docker",
		"run",
		"--rm",                   // Remove container after execution
		"--interactive",          // Keep stdin open
		"--network", networkName, // Connect to specified network
		"--name", container, // Give the container a random name
		"--init",            // Run an init process
		image,               // Use provided image
		"sh", "-c", command, // Run command through shell
	)

	return runDockerCmd(ctx, logger, dockerCmd, container)
}

var containerNameCounter atomic.Int32

func randomContainerName() string {
	return fmt.Sprintf("benchi-temporary-%d", containerNameCounter.Add(1))
}

//nolint:funlen // Maybe refactor later.
func runDockerCmd(ctx context.Context, logger *slog.Logger, cmd *exec.Cmd, container string) error {
	// Create pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Start the command
	logger.Debug("Executing command", "command", cmd.String())
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Read from stdout
	go func() {
		scn := bufio.NewScanner(stdoutPipe)
		for scn.Scan() {
			logger.Info(scn.Text(), "container", container, "stream", "stdout")
		}
		if err := scn.Err(); err != nil {
			logger.Error("stdout scan error", "container", container, "error", err)
		}
	}()

	// Read from stderr
	go func() {
		scn := bufio.NewScanner(stderrPipe)
		for scn.Scan() {
			logger.Error(scn.Text(), "container", container, "stream", "stderr")
		}
		if err := scn.Err(); err != nil {
			logger.Error("stderr scan error", "container", container, "error", err)
		}
	}()

	// Handle context cancellation gracefully
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait() // Wait for process to exit
	}()

	select {
	case <-ctx.Done():
		logger.Warn("Context canceled, sending SIGINT")

		// Send SIGINT to the process first
		if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
			logger.Error("Failed to send SIGINT", "error", err)
		}

		// Give the process some time to exit cleanly
		select {
		case err = <-done:
			break // Process exited normally
		case <-time.After(10 * time.Second):
			logger.Error("Process did not exit in time, killing it")
			if err := cmd.Process.Kill(); err != nil {
				logger.Error("Failed to kill process", "error", err)
				return fmt.Errorf("failed to kill process: %w", err)
			}
		}
	case err = <-done:
		break // Process exited normally
	}

	if err != nil {
		// If the exit code is 130 (SIGINT) and the context is cancelled,
		// consider it a normal termination.
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && exitError.ExitCode() == 130 && ctx.Err() != nil {
			logger.Info("Process was interrupted (expected)", "container", container)
			return nil //nolint:nilerr // Expected error
		}
		return fmt.Errorf("command failed: %w", err)
	}

	return nil
}
