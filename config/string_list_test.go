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

package config

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestStringOrSlice_UnmarshalYAML(t *testing.T) {
	testCases := []struct {
		input string
		want  StringList
	}{{
		input: `"single"`,
		want:  StringList{"single"},
	}, {
		input: `["one", "two", "three"]`,
		want:  StringList{"one", "two", "three"},
	}, {
		input: `- foo
- bar
- baz`,
		want: StringList{"foo", "bar", "baz"},
	}}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			var result StringList
			err := yaml.Unmarshal([]byte(tc.input), &result)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(result, tc.want) {
				t.Errorf("got: %v, want: %v", result, tc.want)
			}
		})
	}
}
