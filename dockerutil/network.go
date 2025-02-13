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
	"context"
	"log/slog"

	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
)

// CreateNetworkIfNotExist inspects for an existing network and creates it if it
// does not exist. If the network already exists, it returns the existing network
// details. This function uses the 'bridge' driver for network creation.
func CreateNetworkIfNotExist(ctx context.Context, dockerClient client.APIClient, networkName string) (network.Inspect, error) {
	net, err := dockerClient.NetworkInspect(ctx, networkName, network.InspectOptions{})
	if errdefs.IsNotFound(err) {
		slog.Info("Network not found, creating it", "network", networkName)

		var netResp network.CreateResponse
		netResp, err = dockerClient.NetworkCreate(ctx, networkName, network.CreateOptions{
			Driver: "bridge",
		})
		if err != nil {
			return network.Inspect{}, err
		}

		if netResp.Warning != "" {
			slog.Warn("Network created with warnings", "network-id", netResp.ID, "warning", netResp.Warning)
		} else {
			slog.Info("Network created", "network-id", netResp.ID)
		}

		net, err = dockerClient.NetworkInspect(ctx, netResp.ID, network.InspectOptions{})
	}
	if err != nil {
		return net, err
	}
	return net, nil
}

func RemoveNetwork(ctx context.Context, dockerClient client.APIClient, networkName string) error {
	slog.Info("Removing network", "network", networkName)
	err := dockerClient.NetworkRemove(ctx, networkName)
	if err != nil {
		slog.Error("Network removing failed", "error", err)
		return err
	}
	slog.Info("Network removed", "network", networkName)
	return nil
}
