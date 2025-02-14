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

import "time"

type Config struct {
	Infrastructure Infrastructure     `yaml:"infrastructure"`
	Tools          Tools              `yaml:"tools"`
	Metrics        []MetricsCollector `yaml:"metrics"`

	Tests []Test `yaml:"tests"`
}

type ServiceConfig struct {
	DockerCompose string `yaml:"docker-compose"`
}

type (
	Infrastructure map[string]ServiceConfig
	Tools          map[string]ServiceConfig
)

type MetricsCollector struct {
	Collector string            `yaml:"collector"`
	Interval  string            `yaml:"interval"`
	Settings  map[string]string `yaml:"settings"`
}

type Test struct {
	Infrastructure map[string]ServiceConfig `yaml:"infrastructure"`
	Tools          map[string]ServiceConfig `yaml:"tools"`
	Metrics        []MetricsCollector       `yaml:"metrics"`

	Name     string        `yaml:"name"`
	Duration time.Duration `yaml:"duration"`
	Steps    TestSteps     `yaml:"steps"`
}

type TestSteps struct {
	PreInfrastructure  []TestStep `yaml:"pre-infrastructure"`
	PostInfrastructure []TestStep `yaml:"post-infrastructure"`
	PreTool            []TestStep `yaml:"pre-tool"`
	PostTool           []TestStep `yaml:"post-tool"`
	PreTest            []TestStep `yaml:"pre-test"`
	During             []TestStep `yaml:"during"`
	PostTest           []TestStep `yaml:"post-test"`
	PreCleanup         []TestStep `yaml:"pre-cleanup"`
	PostCleanup        []TestStep `yaml:"post-cleanup"`
}

type TestStep struct {
	Name      string `yaml:"name"`
	Container string `yaml:"container"`
	Run       string `yaml:"run"`
}
