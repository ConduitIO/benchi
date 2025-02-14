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

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/conduitio/benchi"
	"github.com/conduitio/benchi/config"
	"github.com/conduitio/benchi/dockerutil"
	"github.com/docker/docker/client"
	"gopkg.in/yaml.v3"
)

var (
	networkName = flag.String("network", "benchi", "name of the docker network to create")
	configPath  = flag.String("config", "", "path to the benchmark config file")
	outPath     = flag.String("out", "./results", "path to the output folder")
	verbose     = flag.Bool("verbose", false, "enable verbose logging")
)

func main() {
	err := mainE()
	if err != nil {
		panic(err)
	}
}

func mainE() error {
	ctx := context.Background()
	flag.Parse()

	lvl := slog.LevelInfo
	if *verbose {
		lvl = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})))

	if configPath == nil || strings.TrimSpace(*configPath) == "" {
		return fmt.Errorf("config path is required")
	}

	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	net, err := dockerutil.CreateNetworkIfNotExist(ctx, dockerClient, *networkName)
	if err != nil {
		return err
	}
	slog.Info("Using network", "network", net.Name, "network-id", net.ID)
	defer dockerutil.RemoveNetwork(ctx, dockerClient, net.Name)

	cfg, err := parseConfig()
	if err != nil {
		return err
	}

	err = benchi.Run(ctx, cfg, benchi.RunOptions{
		OutPath:      *outPath,
		FilterTests:  nil, // TODO: implement filter
		Dir:          filepath.Dir(*configPath),
		DockerClient: dockerClient,
	})

	return nil
}

func parseConfig() (config.Config, error) {
	f, err := os.Open(*configPath)
	if err != nil {
		return config.Config{}, err
	}
	defer f.Close()
	var cfg config.Config
	err = yaml.NewDecoder(f).Decode(&cfg)
	f.Close()
	if err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}
