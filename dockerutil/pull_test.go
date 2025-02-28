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

func TestComposePullCmd(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name       string
		composeOpt ComposeOptions
		pullOpt    ComposePullOptions
		wantArgs   []string
	}{
		{
			name:       "empty options",
			composeOpt: ComposeOptions{},
			pullOpt:    ComposePullOptions{},
			wantArgs:   []string{"docker", "compose", "pull"},
		},
		{
			name: "compose options only",
			composeOpt: ComposeOptions{
				ProjectName: ptr("test-project"),
				File:        []string{"docker-compose.yml"},
			},
			pullOpt:  ComposePullOptions{},
			wantArgs: []string{"docker", "compose", "-f", "docker-compose.yml", "--project-name", "test-project", "pull"},
		},
		{
			name:       "pull options only",
			composeOpt: ComposeOptions{},
			pullOpt: ComposePullOptions{
				Quiet:              ptr(true),
				IgnorePullFailures: ptr(true),
				Policy:             ptr("always"),
			},
			wantArgs: []string{"docker", "compose", "pull", "--ignore-pull-failures", "--policy", "always", "--quiet"},
		},
		{
			name: "both compose and pull options",
			composeOpt: ComposeOptions{
				ProjectName: ptr("test-project"),
				File:        []string{"docker-compose.yml"},
			},
			pullOpt: ComposePullOptions{
				Quiet:       ptr(true),
				IncludeDeps: ptr(true),
				Policy:      ptr("missing"),
				Services:    []string{"service1", "service2"},
			},
			wantArgs: []string{
				"docker", "compose",
				"-f", "docker-compose.yml",
				"--project-name", "test-project",
				"pull",
				"--include-deps",
				"--policy", "missing",
				"--quiet",
				"service1", "service2",
			},
		},
		{
			name:       "with services only",
			composeOpt: ComposeOptions{},
			pullOpt: ComposePullOptions{
				Services: []string{"service1", "service2"},
			},
			wantArgs: []string{"docker", "compose", "pull", "service1", "service2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := pullCmd(ctx, tt.composeOpt, tt.pullOpt)

			if !reflect.DeepEqual(cmd.Args, tt.wantArgs) {
				t.Errorf("pullCmd() args = %v, want %v", cmd.Args, tt.wantArgs)
			}
		})
	}
}
