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
	"reflect"
	"testing"
)

func TestComposeDownCmd(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name       string
		composeOpt ComposeOptions
		downOpt    ComposeDownOptions
		wantArgs   []string
	}{
		{
			name:       "empty options",
			composeOpt: ComposeOptions{},
			downOpt:    ComposeDownOptions{},
			wantArgs:   []string{"docker", "compose", "down"},
		},
		{
			name: "compose options only",
			composeOpt: ComposeOptions{
				ProjectName: ptr("test-project"),
				File:        []string{"docker-compose.yml"},
			},
			downOpt:  ComposeDownOptions{},
			wantArgs: []string{"docker", "compose", "-f", "docker-compose.yml", "--project-name", "test-project", "down"},
		},
		{
			name:       "down options only",
			composeOpt: ComposeOptions{},
			downOpt: ComposeDownOptions{
				Volumes:       ptr(true),
				RemoveOrphans: ptr(true),
			},
			wantArgs: []string{"docker", "compose", "down", "--remove-orphans", "-v"},
		},
		{
			name: "both compose and down options",
			composeOpt: ComposeOptions{
				ProjectName: ptr("test-project"),
				File:        []string{"docker-compose.yml"},
			},
			downOpt: ComposeDownOptions{
				Volumes:       ptr(true),
				RemoveOrphans: ptr(true),
				Timeout:       ptr(30),
				Rmi:           ptr("local"),
			},
			wantArgs: []string{
				"docker", "compose",
				"-f", "docker-compose.yml",
				"--project-name", "test-project",
				"down",
				"--remove-orphans",
				"--rmi", "local",
				"-t", "30",
				"-v",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := downCmd(ctx, tt.composeOpt, tt.downOpt)

			if !reflect.DeepEqual(cmd.Args, tt.wantArgs) {
				t.Errorf("downCmd() args = %v, want %v", cmd.Args, tt.wantArgs)
			}
		})
	}
}
