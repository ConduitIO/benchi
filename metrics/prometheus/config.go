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

package prometheus

import (
	"fmt"
	"net/url"
	"time"
)

var defaultConfig = Config{
	ScrapeInterval: time.Second,
}

type Config struct {
	// URL points to the metrics endpoint of the service to be monitored.
	URL string `yaml:"url"`
	// ScrapeInterval is the time between scrapes (defaults to 1s).
	ScrapeInterval time.Duration `yaml:"scrape-interval"`
	// Queries are the query configurations used when querying the collected
	// metrics.
	Queries []QueryConfig `yaml:"queries"`
}

func (c Config) parseURL() (*url.URL, error) {
	u, err := url.Parse(c.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}
	return u, nil
}

type QueryConfig struct {
	Name string `yaml:"name"`
	// QueryString is the PromQL query string to be executed.
	QueryString string `yaml:"query"`
	// Interval is the query resolution.
	Interval time.Duration `yaml:"interval"`
}
