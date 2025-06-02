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

// Disclaimer: This code is a modified version of the original code from the
// Docker cli project (https://github.com/docker/cli/blob/master/cli/command/container/stats_helpers.go).
// The original code is licensed under the Apache License 2.0.

package docker

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// statsEntry represents the statistics data collected from a container.
type statsEntry struct {
	CollectedAt time.Time

	Name             string
	ID               string
	CPUPercentage    float64
	Memory           float64 // On Windows this is the private working set
	MemoryLimit      float64 // Not used on Windows
	MemoryPercentage float64 // Not used on Windows
	NetworkRx        float64
	NetworkTx        float64
	BlockRead        float64
	BlockWrite       float64
	PidsCurrent      uint64 // Not used on Windows
	IsInvalid        bool
}

// Stats represents an entity to store containers statistics synchronously.
type stats struct {
	statsEntry
	Container string
	Err       error
}

//nolint:funlen // Keeping this close to the original code in docker.
func collect(
	ctx context.Context,
	logger *slog.Logger,
	cli client.ContainerAPIClient,
	containerName string,
	out chan<- stats,
) {
	logger.Debug("Collecting stats")

	response, err := cli.ContainerStats(ctx, containerName, true)
	if err != nil {
		if errdefs.IsNotFound(err) {
			logger.Warn("Container not found, metrics won't be collected for container", "error", err)
			return
		}
		out <- stats{Container: containerName, Err: err}
		return
	}
	defer response.Body.Close()

	dec := json.NewDecoder(response.Body)

	for {
		var (
			v                      *container.StatsResponse
			memPercent, cpuPercent float64
			blkRead, blkWrite      uint64 // Only used on Linux
			mem, memLimit          float64
			pidsStatsCurrent       uint64
			previousCPU            uint64
			previousSystem         uint64
		)

		if err := dec.Decode(&v); err != nil {
			if ctx.Err() != nil {
				return // Context was canceled, just stop.
			}

			dec = json.NewDecoder(io.MultiReader(dec.Buffered(), response.Body))
			if err == io.EOF {
				break
			}
			out <- stats{Container: containerName, Err: err}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Docker returns the internal docker container name which is prefixed
		// with the parent name and /. We remove the leading slash to match the
		// connector name.
		v.Name = strings.TrimPrefix(v.Name, "/")
		if v.Name == "" {
			// older API versions do not provide the container name
			v.Name = containerName
		}

		if response.OSType != "windows" {
			previousCPU = v.PreCPUStats.CPUUsage.TotalUsage
			previousSystem = v.PreCPUStats.SystemUsage
			cpuPercent = calculateCPUPercentUnix(previousCPU, previousSystem, v)
			blkRead, blkWrite = calculateBlockIO(v.BlkioStats)
			mem = calculateMemUsageUnixNoCache(v.MemoryStats)
			memLimit = float64(v.MemoryStats.Limit)
			memPercent = calculateMemPercentUnixNoCache(memLimit, mem)
			pidsStatsCurrent = v.PidsStats.Current
		} else {
			cpuPercent = calculateCPUPercentWindows(v)
			blkRead = v.StorageStats.ReadSizeBytes
			blkWrite = v.StorageStats.WriteSizeBytes
			mem = float64(v.MemoryStats.PrivateWorkingSet)
		}
		netRx, netTx := calculateNetwork(v.Networks)
		out <- stats{Container: containerName, statsEntry: statsEntry{
			CollectedAt: time.Now(),

			Name:             v.Name,
			ID:               v.ID,
			CPUPercentage:    cpuPercent,
			Memory:           mem,
			MemoryPercentage: memPercent,
			MemoryLimit:      memLimit,
			NetworkRx:        netRx,
			NetworkTx:        netTx,
			BlockRead:        float64(blkRead),
			BlockWrite:       float64(blkWrite),
			PidsCurrent:      pidsStatsCurrent,
		}}
	}
}

func calculateCPUPercentUnix(previousCPU, previousSystem uint64, v *container.StatsResponse) float64 {
	var (
		cpuPercent = 0.0
		// calculate the change for the cpu usage of the container in between readings
		cpuDelta = float64(v.CPUStats.CPUUsage.TotalUsage) - float64(previousCPU)
		// calculate the change for the entire system between readings
		systemDelta = float64(v.CPUStats.SystemUsage) - float64(previousSystem)
		onlineCPUs  = float64(v.CPUStats.OnlineCPUs)
	)

	if onlineCPUs == 0.0 {
		onlineCPUs = float64(len(v.CPUStats.CPUUsage.PercpuUsage))
	}
	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * onlineCPUs * 100.0
	}
	return cpuPercent
}

func calculateCPUPercentWindows(v *container.StatsResponse) float64 {
	// Max number of 100ns intervals between the previous time read and now
	//nolint:gosec // This is always positive.
	possIntervals := uint64(v.Read.Sub(v.PreRead).Nanoseconds()) // Start with number of ns intervals
	possIntervals /= 100                                         // Convert to number of 100ns intervals
	possIntervals *= uint64(v.NumProcs)                          // Multiple by the number of processors

	// Intervals used
	intervalsUsed := v.CPUStats.CPUUsage.TotalUsage - v.PreCPUStats.CPUUsage.TotalUsage

	// Percentage avoiding divide-by-zero
	if possIntervals > 0 {
		return float64(intervalsUsed) / float64(possIntervals) * 100.0
	}
	return 0.00
}

func calculateBlockIO(blkio container.BlkioStats) (uint64, uint64) {
	var blkRead, blkWrite uint64
	for _, bioEntry := range blkio.IoServiceBytesRecursive {
		if len(bioEntry.Op) == 0 {
			continue
		}
		switch bioEntry.Op[0] {
		case 'r', 'R':
			blkRead += bioEntry.Value
		case 'w', 'W':
			blkWrite += bioEntry.Value
		}
	}
	return blkRead, blkWrite
}

func calculateNetwork(network map[string]container.NetworkStats) (float64, float64) {
	var rx, tx float64

	for _, v := range network {
		rx += float64(v.RxBytes)
		tx += float64(v.TxBytes)
	}
	return rx, tx
}

// calculateMemUsageUnixNoCache calculate memory usage of the container.
// Cache is intentionally excluded to avoid misinterpretation of the output.
//
// On cgroup v1 host, the result is `mem.Usage - mem.Stats["total_inactive_file"]` .
// On cgroup v2 host, the result is `mem.Usage - mem.Stats["inactive_file"] `.
//
// This definition is consistent with cadvisor and containerd/CRI.
// * https://github.com/google/cadvisor/commit/307d1b1cb320fef66fab02db749f07a459245451
// * https://github.com/containerd/cri/commit/6b8846cdf8b8c98c1d965313d66bc8489166059a
//
// On Docker 19.03 and older, the result was `mem.Usage - mem.Stats["cache"]`.
// See https://github.com/moby/moby/issues/40727 for the background.
func calculateMemUsageUnixNoCache(mem container.MemoryStats) float64 {
	// cgroup v1
	if v, isCgroup1 := mem.Stats["total_inactive_file"]; isCgroup1 && v < mem.Usage {
		return float64(mem.Usage - v)
	}
	// cgroup v2
	if v := mem.Stats["inactive_file"]; v < mem.Usage {
		return float64(mem.Usage - v)
	}
	return float64(mem.Usage)
}

func calculateMemPercentUnixNoCache(limit float64, usedNoCache float64) float64 {
	// MemoryStats.Limit will never be 0 unless the container is not running and we haven't
	// got any data from cgroup
	if limit != 0 {
		return usedNoCache / limit * 100.0
	}
	return 0
}
