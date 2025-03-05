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

func TestComposePsCmd(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name       string
		composeOpt ComposeOptions
		psOpt      ComposePsOptions
		wantArgs   []string
	}{
		{
			name:       "empty options",
			composeOpt: ComposeOptions{},
			psOpt:      ComposePsOptions{},
			wantArgs:   []string{"docker", "compose", "ps"},
		},
		{
			name: "compose options only",
			composeOpt: ComposeOptions{
				ProjectName: ptr("test-project"),
				File:        []string{"docker-compose.yml"},
			},
			psOpt:    ComposePsOptions{},
			wantArgs: []string{"docker", "compose", "-f", "docker-compose.yml", "--project-name", "test-project", "ps"},
		},
		{
			name:       "ps options only",
			composeOpt: ComposeOptions{},
			psOpt: ComposePsOptions{
				All:      ptr(true),
				Quiet:    ptr(true),
				NoTrunc:  ptr(true),
				Status:   []string{"running", "exited"},
				Services: ptr(true),
			},
			wantArgs: []string{"docker", "compose", "ps", "--all", "--no-trunc", "--quiet", "--services", "--status", "running", "--status", "exited"},
		},
		{
			name: "both compose and ps options",
			composeOpt: ComposeOptions{
				ProjectName: ptr("test-project"),
				File:        []string{"docker-compose.yml"},
			},
			psOpt: ComposePsOptions{
				All:          ptr(true),
				Format:       ptr("json"),
				ServiceNames: []string{"web", "db"},
			},
			wantArgs: []string{
				"docker", "compose",
				"-f", "docker-compose.yml",
				"--project-name", "test-project",
				"ps",
				"--all",
				"--format", "json",
				"web", "db",
			},
		},
		{
			name:       "with service names only",
			composeOpt: ComposeOptions{},
			psOpt: ComposePsOptions{
				ServiceNames: []string{"web", "db"},
			},
			wantArgs: []string{"docker", "compose", "ps", "web", "db"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := psCmd(ctx, tt.composeOpt, tt.psOpt)

			if !reflect.DeepEqual(cmd.Args, tt.wantArgs) {
				t.Errorf("psCmd() args = %v, want %v", cmd.Args, tt.wantArgs)
			}
		})
	}
}
