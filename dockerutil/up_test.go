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
	"reflect"
	"testing"
)

func TestComposeUpCmd(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		composeOpt ComposeOptions
		upOpt      UpOptions
		wantArgs   []string
	}{
		{
			name:       "empty options",
			composeOpt: ComposeOptions{},
			upOpt:      UpOptions{},
			wantArgs:   []string{"docker", "compose", "up"},
		},
		{
			name: "compose options only",
			composeOpt: ComposeOptions{
				ProjectName: ptr("test-project"),
				File:        []string{"docker-compose.yml"},
			},
			upOpt:    UpOptions{},
			wantArgs: []string{"docker", "compose", "-f", "docker-compose.yml", "--project-name", "test-project", "up"},
		},
		{
			name:       "up options only",
			composeOpt: ComposeOptions{},
			upOpt: UpOptions{
				Detach:        ptr(true),
				RemoveOrphans: ptr(true),
			},
			wantArgs: []string{"docker", "compose", "up", "--detach", "--remove-orphans"},
		},
		{
			name: "both compose and up options",
			composeOpt: ComposeOptions{
				ProjectName: ptr("test-project"),
				File:        []string{"docker-compose.yml"},
			},
			upOpt: UpOptions{
				Detach:        ptr(true),
				RemoveOrphans: ptr(true),
				Timeout:       ptr(30),
			},
			wantArgs: []string{
				"docker", "compose",
				"-f", "docker-compose.yml",
				"--project-name", "test-project",
				"up",
				"--detach",
				"--remove-orphans",
				"--timeout", "30",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := upCmd(ctx, tt.composeOpt, tt.upOpt)

			if !reflect.DeepEqual(cmd.Args, tt.wantArgs) {
				t.Errorf("upCmd() args = %v, want %v", cmd.Args, tt.wantArgs)
			}
		})
	}
}

func ptr[T any](s T) *T {
	return &s
}
