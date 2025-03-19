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

// Config represents the overall configuration for the application.
type Config struct {
	// Infrastructure defines the infrastructure services configuration.
	Infrastructure Infrastructure `yaml:"infrastructure"`
	// Tools defines the tools configuration.
	Tools Tools `yaml:"tools"`
	// Metrics defines the metrics collectors configuration.
	Metrics map[string]MetricsCollector `yaml:"metrics"`
	// Tests defines the test configurations.
	Tests []Test `yaml:"tests"`
}

// ServiceConfig represents the configuration for a service.
type ServiceConfig struct {
	// DockerCompose is the path to the Docker Compose file for the service. If
	// it's a relative path, it will be resolved relative to the configuration
	// file.
	DockerCompose StringList `yaml:"compose"`
}

// Infrastructure represents a map of service configurations for the infrastructure.
type Infrastructure map[string]ServiceConfig

// Tools represents a map of service configurations for the tools.
type Tools map[string]ServiceConfig

// MetricsCollector represents the configuration for a metrics collector.
type MetricsCollector struct {
	// Collector is the name of the metrics collector.
	Collector string `yaml:"collector"`
	// Settings defines additional settings for the metrics collector. The
	// specific settings depend on the collector.
	Settings map[string]any `yaml:"settings"`
	// Tools is a list of tools for which the collector is applicable. If empty,\
	// the collector will be applied to all tools.
	Tools []string `yaml:"tools"`
}

// Test represents the configuration for a test.
type Test struct {
	// Infrastructure defines the infrastructure services configuration for the
	// test. This configuration will be merged with the global infrastructure
	// configuration.
	Infrastructure map[string]ServiceConfig `yaml:"infrastructure"`
	// Tools defines the tools configuration for the test. This configuration will
	// be merged with the global tools configuration.
	Tools map[string]ServiceConfig `yaml:"tools"`

	// Name is the name of the test.
	Name string `yaml:"name"`
	// Duration is the duration of the test. The test will be stopped after this
	// duration.
	Duration time.Duration `yaml:"duration"`
	// Steps defines the hooks to be executed during the test.
	Steps TestHooks `yaml:"steps"`
}

// TestHooks represents the hooks to be executed during different steps of the
// test.
type TestHooks struct {
	// PreInfrastructure defines the hooks to be executed before setting up the
	// infrastructure.
	PreInfrastructure []TestHook `yaml:"pre-infrastructure"`
	// PostInfrastructure defines the hooks to be executed after setting up the
	// infrastructure.
	PostInfrastructure []TestHook `yaml:"post-infrastructure"`
	// PreTool defines the hooks to be executed before setting up the tools.
	PreTool []TestHook `yaml:"pre-tool"`
	// PostTool defines the hooks to be executed after setting up the tools.
	PostTool []TestHook `yaml:"post-tool"`
	// PreTest defines the hooks to be executed before starting the test.
	PreTest []TestHook `yaml:"pre-test"`
	// During defines the hooks to be executed during the test.
	During []TestHook `yaml:"during"`
	// PostTest defines the hooks to be executed after the test.
	PostTest []TestHook `yaml:"post-test"`
	// PreCleanup defines the hooks to be executed before cleaning up the test
	// environment.
	PreCleanup []TestHook `yaml:"pre-cleanup"`
	// PostCleanup defines the hooks to be executed after cleaning up the test
	// environment.
	PostCleanup []TestHook `yaml:"post-cleanup"`
}

func (th TestHooks) All() []TestHook {
	var hooks []TestHook
	hooks = append(hooks, th.PreInfrastructure...)
	hooks = append(hooks, th.PostInfrastructure...)
	hooks = append(hooks, th.PreTool...)
	hooks = append(hooks, th.PostTool...)
	hooks = append(hooks, th.PreTest...)
	hooks = append(hooks, th.During...)
	hooks = append(hooks, th.PostTest...)
	hooks = append(hooks, th.PreCleanup...)
	hooks = append(hooks, th.PostCleanup...)
	return hooks
}

type TestHook struct {
	// Name is the name of the hook.
	Name string `yaml:"name"`
	// Container is the name of the container to run the command in. If empty, the
	// command will be run on a temporary container using the image specified in
	// the `image` field.
	Container string `yaml:"container"`
	// Image is the image to use for the temporary container. If `container` is
	// specified, this field is ignored. If both `container` and `image` are empty,
	// the command will be run in a temporary container using the alpine image.
	Image string `yaml:"image"`
	// Run is the command to be executed.
	Run string `yaml:"run"`
	// Tools is a list of tools for which the hook is applicable. If empty, the
	// hook will be applied to all tools.
	Tools []string `yaml:"tools"`
}
