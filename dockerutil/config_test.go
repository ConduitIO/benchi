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

func TestComposeConfigCmd(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name       string
		composeOpt ComposeOptions
		configOpt  ComposeConfigOptions
		wantArgs   []string
	}{
		{
			name:       "empty options",
			composeOpt: ComposeOptions{},
			configOpt:  ComposeConfigOptions{},
			wantArgs:   []string{"docker", "compose", "config"},
		},
		{
			name: "compose options only",
			composeOpt: ComposeOptions{
				ProjectName: ptr("test-project"),
				File:        []string{"docker-compose.yml"},
			},
			configOpt: ComposeConfigOptions{},
			wantArgs:  []string{"docker", "compose", "-f", "docker-compose.yml", "--project-name", "test-project", "config"},
		},
		{
			name:       "config options only",
			composeOpt: ComposeOptions{},
			configOpt: ComposeConfigOptions{
				Format: ptr("json"),
			},
			wantArgs: []string{"docker", "compose", "config", "--format", "json"},
		},
		{
			name: "both compose and config options",
			composeOpt: ComposeOptions{
				ProjectName: ptr("test-project"),
				File:        []string{"docker-compose.yml"},
			},
			configOpt: ComposeConfigOptions{
				Format: ptr("json"),
				Quiet:  ptr(true),
			},
			wantArgs: []string{
				"docker", "compose",
				"-f", "docker-compose.yml",
				"--project-name", "test-project",
				"config",
				"--format", "json",
				"--quiet",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := configCmd(ctx, tt.composeOpt, tt.configOpt)

			if !reflect.DeepEqual(cmd.Args, tt.wantArgs) {
				t.Errorf("configCmd() args = %v, want %v", cmd.Args, tt.wantArgs)
			}
		})
	}
}
