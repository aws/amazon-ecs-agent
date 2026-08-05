// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package engine

import (
	"fmt"
	"strings"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
)

// Per-UUID GPU memory accounting, modeled like the CPU/MEMORY resources: a
// capacity map (gpuMemoryTotalMiB) fixed at init, and a consumed map
// (gpuMemoryConsumed) that consume/release move up and down. Each physical GPU
// has an entry so multiple MPS tasks can share it up to its memory, while a
// whole-GPU task takes it exclusively. Every GPU stays in the capacity map for
// the life of the agent; consume and reclaim only change its consumed record.

// GPUMemoryCapacityPrefix keys a per-GPU total-memory capacity entry in the host
// resource map: "GPU_MEMORY:<uuid>", an INTEGER of usable MiB.
const GPUMemoryCapacityPrefix = "GPU_MEMORY:"

// gpuMemoryResourceKey is a display label for a per-UUID GPU memory demand, used
// in the "resources not consumable" log so a blocked GPU task is identifiable.
func gpuMemoryResourceKey(uuid string) string {
	return GPUMemoryCapacityPrefix + uuid
}

// gpuUUIDFromCapacityKey returns the UUID from a GPU_MEMORY:<uuid> capacity key.
func gpuUUIDFromCapacityKey(key string) (string, bool) {
	if !strings.HasPrefix(key, GPUMemoryCapacityPrefix) {
		return "", false
	}
	return strings.TrimPrefix(key, GPUMemoryCapacityPrefix), true
}

// gpuMode derives a GPU's FREE/MPS/WHOLE_GPU state from its consumed record,
// for logging only. WHOLE_GPU is exclusive (WholeGPU true), MPS has a positive
// share reserved, and everything else is FREE.
func gpuMode(consumed apitask.GPUMemoryDemand) string {
	if consumed.WholeGPU {
		return "WHOLE_GPU"
	}
	if consumed.MiB > 0 {
		return "MPS"
	}
	return "FREE"
}

// checkConsumableGPUMemory reports whether a demand fits. A whole-GPU request
// needs a GPU with nothing consumed on it (no MPS share, not already exclusive).
// An MPS share needs a GPU that is not exclusively held and still has room.
// Keying the whole-GPU path off the consumed record is what prevents an MPS and
// a whole-GPU task sharing a GPU.
func (h *HostResourceManager) checkConsumableGPUMemory(uuid string, demand apitask.GPUMemoryDemand) bool {
	total, ok := h.gpuMemoryTotalMiB[uuid]
	if !ok {
		return false
	}
	consumed := h.gpuMemoryConsumed[uuid]
	if demand.WholeGPU {
		// Exclusive: needs a fully free GPU (no MPS share, not already exclusive).
		return consumed.MiB == 0 && !consumed.WholeGPU
	}
	if consumed.WholeGPU {
		// Can't share an exclusively-held GPU.
		return false
	}
	// A GPU whose memory was not reported has total 0. The whole-GPU branch
	// above still admits it (it checks the consumed record, not total); an MPS
	// share is refused here because 0 can never satisfy a positive demand.
	return total-consumed.MiB >= demand.MiB
}

// consumeGPUMemory applies a task's GPU memory demand to the pool: a whole-GPU
// demand marks the device exclusive, an MPS demand adds to the reserved share.
func (h *HostResourceManager) consumeGPUMemory(uuid string, demand apitask.GPUMemoryDemand) {
	if _, ok := h.gpuMemoryTotalMiB[uuid]; !ok {
		// checkResourcesHealth rejects an unknown UUID before consume runs.
		logger.Warn("GPU memory consume for unknown GPU UUID", logger.Fields{"gpuUUID": uuid})
		return
	}
	consumed := h.gpuMemoryConsumed[uuid]
	if demand.WholeGPU {
		consumed.WholeGPU = true
	} else {
		consumed.MiB += demand.MiB
	}
	h.gpuMemoryConsumed[uuid] = consumed
}

// releaseGPUMemory reverses a demand with the identical value it was consumed
// with, so consume and reclaim stay balanced. A GPU is derived FREE again once
// its consumed record is back at the zero value.
func (h *HostResourceManager) releaseGPUMemory(uuid string, demand apitask.GPUMemoryDemand) {
	if _, ok := h.gpuMemoryTotalMiB[uuid]; !ok {
		logger.Warn("GPU memory release for unknown GPU UUID", logger.Fields{"gpuUUID": uuid})
		return
	}
	consumed := h.gpuMemoryConsumed[uuid]
	if demand.WholeGPU {
		consumed.WholeGPU = false
	} else {
		consumed.MiB -= demand.MiB
		if consumed.MiB < 0 {
			// consumed should never go negative since consume and release are symmetric
			logger.Error("GPU memory over-released; clamping to 0", logger.Fields{
				"gpuUUID":  uuid,
				"consumed": consumed.MiB,
			})
			consumed.MiB = 0
		}
	}
	h.gpuMemoryConsumed[uuid] = consumed
}

func (h *HostResourceManager) logGPUMemoryPool(msg string, taskArn string) {
	if len(h.gpuMemoryTotalMiB) == 0 {
		return
	}
	var b strings.Builder
	for uuid, total := range h.gpuMemoryTotalMiB {
		b.WriteString(formatGPUMemoryEntry(uuid, total, h.gpuMemoryConsumed[uuid]))
	}
	logger.Debug("GPU memory pool: "+msg, logger.Fields{
		"taskArn": taskArn,
		"pool":    b.String(),
	})
}

// formatGPUMemoryEntry renders one GPU's pool snapshot token. A whole-GPU
// consume leaves MiB at 0, so remaining is shown as 0 rather than total. A GPU
// with total 0 had no reported memory and shows total=unknown so the snapshot
// is not mistaken for a discovery failure.
func formatGPUMemoryEntry(uuid string, total int64, consumed apitask.GPUMemoryDemand) string {
	remaining := total - consumed.MiB
	if consumed.WholeGPU {
		remaining = 0
	}
	if total == 0 {
		return fmt.Sprintf("%s{total=unknown remaining=%d mode=%s} ", uuid, remaining, gpuMode(consumed))
	}
	return fmt.Sprintf("%s{total=%d remaining=%d mode=%s} ", uuid, total, remaining, gpuMode(consumed))
}
