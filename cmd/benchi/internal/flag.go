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

package internal

import (
	"flag"
	"strings"
)

// stringsFlagValue is a custom flag.Value implementation that allows users to provide
// multiple values for a flag by providing the flag multiple times.
type stringsFlagValue []string

func (s *stringsFlagValue) String() string {
	return strings.Join(*s, ",")
}

func (s *stringsFlagValue) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func StringsFlag(name string, value []string, usage string) *[]string {
	f := stringsFlagValue(value)
	flag.Var(&f, name, usage)
	return (*[]string)(&f)
}
