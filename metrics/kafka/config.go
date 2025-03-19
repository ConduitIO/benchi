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

package kafka

import (
	"fmt"

	"github.com/conduitio/benchi/config"
	"github.com/go-viper/mapstructure/v2"
)

type Config struct {
	// Topics is a list of topics to monitor.
	Topics config.StringList `yaml:"topics"`
}

func parseConfig(settings map[string]any) (Config, error) {
	var cfg Config
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
		ErrorUnused:      false, // Unused fields will be reported by the prometheus collector.
		WeaklyTypedInput: true,
		Result:           &cfg,
		TagName:          "yaml",
	})
	if err != nil {
		return Config{}, fmt.Errorf("failed to create decoder: %w", err)
	}

	err = dec.Decode(settings)
	if err != nil {
		return Config{}, fmt.Errorf("failed to decode settings: %w", err)
	}

	return cfg, nil
}
