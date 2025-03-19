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
	"fmt"

	"gopkg.in/yaml.v3"
)

// StringList represents a YAML field that can be either a string or a slice of strings.
type StringList []string

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (s *StringList) UnmarshalYAML(value *yaml.Node) error {
	var single string
	var multi []string

	// Try to unmarshal as a single string
	if err := value.Decode(&single); err == nil {
		*s = []string{single}
		return nil
	}

	// Try to unmarshal as a slice of strings
	if err := value.Decode(&multi); err == nil {
		*s = multi
		return nil
	}

	return fmt.Errorf("failed to unmarshal StringList")
}
