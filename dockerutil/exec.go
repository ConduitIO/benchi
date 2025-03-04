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
	"fmt"
	"log/slog"
	"os/exec"
)

func logAndRun(cmd *exec.Cmd) error {
	slog.Debug("Executing command", "command", cmd.String())
	err := cmd.Run()
	if err != nil {
		slog.Error("Command failed", "command", cmd.String(), "error", err)
		return fmt.Errorf("failed to run command %s: %w", cmd.String(), err)
	}
	return nil
}
