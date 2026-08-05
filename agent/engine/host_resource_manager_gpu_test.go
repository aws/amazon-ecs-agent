//go:build unit
// +build unit

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
	"testing"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/assert"
)

// hrmWithPool builds a HostResourceManager whose GPU pool has one GPU (uuid-A)
// with the given total capacity, an empty consumed map, everything else empty.
func hrmWithPool(total int64) *HostResourceManager {
	return &HostResourceManager{
		gpuMemoryTotalMiB: map[string]int64{"uuid-A": total},
		gpuMemoryConsumed: map[string]apitask.GPUMemoryDemand{},
	}
}

// hrmWithPoolMulti builds a HostResourceManager whose GPU pool has the given
// UUID->total-MiB capacity entries, for tests that need more than one GPU.
func hrmWithPoolMulti(totals map[string]int64) *HostResourceManager {
	return &HostResourceManager{
		gpuMemoryTotalMiB: totals,
		gpuMemoryConsumed: map[string]apitask.GPUMemoryDemand{},
	}
}

// mpsDemand / wholeGPUDemand build the two demand shapes for pool tests.
func mpsDemand(mib int64) apitask.GPUMemoryDemand { return apitask.GPUMemoryDemand{MiB: mib} }
func wholeGPUDemand() apitask.GPUMemoryDemand     { return apitask.GPUMemoryDemand{WholeGPU: true} }

// gpuRemaining derives the memory remaining on a GPU the way logGPUMemoryPool
// displays it: total minus the consumed MPS share, or 0 when held whole-GPU.
func gpuRemaining(h *HostResourceManager, uuid string) int64 {
	consumed := h.gpuMemoryConsumed[uuid]
	if consumed.WholeGPU {
		return 0
	}
	return h.gpuMemoryTotalMiB[uuid] - consumed.MiB
}

// gpuModeOf derives a GPU's FREE/MPS/WHOLE_GPU state from its consumed record.
func gpuModeOf(h *HostResourceManager, uuid string) string {
	return gpuMode(h.gpuMemoryConsumed[uuid])
}

// gpuMemoryCapacity builds a GPU_MEMORY:<uuid> capacity resource (INTEGER MiB).
func gpuMemoryCapacity(uuid string, mib int32) types.Resource {
	key := GPUMemoryCapacityPrefix + uuid
	return types.Resource{
		Name:         utils.Strptr(key),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: mib,
	}
}

// TestNewGPUMemoryCapacityMap verifies NewHostResourceManager seeds the per-UUID
// capacity map from GPU_MEMORY:<uuid> entries and starts every GPU derived FREE
// with an empty consumed map.
func TestNewGPUMemoryCapacityMap(t *testing.T) {
	resourceMap := map[string]types.Resource{
		"CPU":    {Name: utils.Strptr("CPU"), Type: utils.Strptr("INTEGER"), IntegerValue: 2048},
		"MEMORY": {Name: utils.Strptr("MEMORY"), Type: utils.Strptr("INTEGER"), IntegerValue: 2048},
	}
	resourceMap[GPUMemoryCapacityPrefix+"a"] = gpuMemoryCapacity("a", 23040)
	resourceMap[GPUMemoryCapacityPrefix+"b"] = gpuMemoryCapacity("b", 16384)
	hrm := NewHostResourceManager(resourceMap)
	h := &hrm

	assert.Len(t, h.gpuMemoryTotalMiB, 2)
	assert.Equal(t, int64(23040), h.gpuMemoryTotalMiB["a"])
	assert.Equal(t, int64(16384), h.gpuMemoryTotalMiB["b"])
	// Consumed map starts empty; both GPUs derive FREE with full remaining.
	assert.Empty(t, h.gpuMemoryConsumed)
	assert.Equal(t, "FREE", gpuModeOf(h, "a"))
	assert.Equal(t, "FREE", gpuModeOf(h, "b"))
	assert.Equal(t, int64(23040), gpuRemaining(h, "a"))
	assert.Equal(t, int64(16384), gpuRemaining(h, "b"))
}

func TestGPUMemoryMPSShareLifecycle(t *testing.T) {
	h := hrmWithPool(23040)

	// First MPS task: 4096 fits a FREE device.
	assert.True(t, h.checkConsumableGPUMemory("uuid-A", mpsDemand(4096)))
	h.consumeGPUMemory("uuid-A", mpsDemand(4096))
	assert.Equal(t, int64(18944), gpuRemaining(h, "uuid-A"))
	assert.Equal(t, "MPS", gpuModeOf(h, "uuid-A"))

	// Second MPS task shares the same device.
	assert.True(t, h.checkConsumableGPUMemory("uuid-A", mpsDemand(4096)))
	h.consumeGPUMemory("uuid-A", mpsDemand(4096))
	assert.Equal(t, int64(14848), gpuRemaining(h, "uuid-A"))

	// Release both; GPU returns to FREE only when remaining is back to total.
	h.releaseGPUMemory("uuid-A", mpsDemand(4096))
	assert.Equal(t, "MPS", gpuModeOf(h, "uuid-A"), "still one MPS task left")
	h.releaseGPUMemory("uuid-A", mpsDemand(4096))
	assert.Equal(t, int64(23040), gpuRemaining(h, "uuid-A"))
	assert.Equal(t, "FREE", gpuModeOf(h, "uuid-A"), "last MPS task left -> FREE")
}

// TestGPUMemoryMPSAsymmetricOutOfOrderRelease uses unequal shares released in
// the opposite order they were consumed, so a bug that assumed equal shares or
// LIFO release would surface. The GPU stays MPS until the last share is back.
// (Edge case 4.)
func TestGPUMemoryMPSAsymmetricOutOfOrderRelease(t *testing.T) {
	h := hrmWithPool(10000)

	h.consumeGPUMemory("uuid-A", mpsDemand(3000)) // remaining 7000
	h.consumeGPUMemory("uuid-A", mpsDemand(5000)) // remaining 2000
	assert.Equal(t, int64(2000), gpuRemaining(h, "uuid-A"))
	assert.Equal(t, "MPS", gpuModeOf(h, "uuid-A"))

	// Release the FIRST (smaller) share first: remaining 2000 -> 5000, still MPS.
	h.releaseGPUMemory("uuid-A", mpsDemand(3000))
	assert.Equal(t, int64(5000), gpuRemaining(h, "uuid-A"))
	assert.Equal(t, "MPS", gpuModeOf(h, "uuid-A"), "one MPS share still held")

	// Release the larger share: remaining back to total -> FREE.
	h.releaseGPUMemory("uuid-A", mpsDemand(5000))
	assert.Equal(t, int64(10000), gpuRemaining(h, "uuid-A"))
	assert.Equal(t, "FREE", gpuModeOf(h, "uuid-A"), "last MPS share released -> FREE")
}

func TestGPUMemoryMPSOverCapacityRejected(t *testing.T) {
	h := hrmWithPool(8192)
	h.consumeGPUMemory("uuid-A", mpsDemand(6144))
	// Only 2048 remains; a 4096 demand must not fit.
	assert.False(t, h.checkConsumableGPUMemory("uuid-A", mpsDemand(4096)))
	// But a 2048 demand fits exactly.
	assert.True(t, h.checkConsumableGPUMemory("uuid-A", mpsDemand(2048)))
}

func TestGPUMemoryWholeGPULifecycle(t *testing.T) {
	h := hrmWithPool(23040)

	// Whole-GPU fits a FREE device and drives it to WHOLE_GPU / remaining 0.
	assert.True(t, h.checkConsumableGPUMemory("uuid-A", wholeGPUDemand()))
	h.consumeGPUMemory("uuid-A", wholeGPUDemand())
	assert.Equal(t, int64(0), gpuRemaining(h, "uuid-A"))
	assert.Equal(t, "WHOLE_GPU", gpuModeOf(h, "uuid-A"))
	// The consumed record must stay {MiB:0, WholeGPU:true} - never a positive MiB.
	assert.Equal(t, int64(0), h.gpuMemoryConsumed["uuid-A"].MiB)

	// Release restores it to FREE / total.
	h.releaseGPUMemory("uuid-A", wholeGPUDemand())
	assert.Equal(t, int64(23040), gpuRemaining(h, "uuid-A"))
	assert.Equal(t, "FREE", gpuModeOf(h, "uuid-A"))
}

func TestGPUMemoryModeMixingRejected(t *testing.T) {
	h := hrmWithPool(23040)

	// MPS task on the device -> MPS mode.
	h.consumeGPUMemory("uuid-A", mpsDemand(4096))
	// A whole-GPU demand must NOT fit an MPS device (mode-mixing).
	assert.False(t, h.checkConsumableGPUMemory("uuid-A", wholeGPUDemand()))

	// Conversely, a whole-GPU device rejects an MPS demand.
	h2 := hrmWithPool(23040)
	h2.consumeGPUMemory("uuid-A", wholeGPUDemand())
	assert.False(t, h2.checkConsumableGPUMemory("uuid-A", mpsDemand(4096)))
}

func TestGPUMemoryUnknownUUIDNotConsumable(t *testing.T) {
	h := hrmWithPool(23040)
	assert.False(t, h.checkConsumableGPUMemory("uuid-does-not-exist", mpsDemand(4096)))
	assert.False(t, h.checkConsumableGPUMemory("uuid-does-not-exist", wholeGPUDemand()))
}

func TestGPUMemoryReleaseClampsAgainstDrift(t *testing.T) {
	h := hrmWithPool(8192)
	h.consumeGPUMemory("uuid-A", mpsDemand(4096))
	// Over-release (more than consumed) must not push consumed negative /
	// remaining above total.
	h.releaseGPUMemory("uuid-A", mpsDemand(8192))
	assert.Equal(t, int64(8192), gpuRemaining(h, "uuid-A"))
	assert.Equal(t, int64(0), h.gpuMemoryConsumed["uuid-A"].MiB, "consumed clamped to 0, never negative")
	assert.Equal(t, "FREE", gpuModeOf(h, "uuid-A"))
}

// TestGPUMemoryUnknownMemory covers a GPU seeded with total 0, which is how a
// GPU whose memory was not reported enters the pool. Whole-GPU tasks must still
// run against it and stay exclusive; a second whole-GPU must be refused; MPS
// shares must be refused; and release returns it to derived FREE. (Edge case 2.)
func TestGPUMemoryUnknownMemory(t *testing.T) {
	h := hrmWithPool(0)

	// Whole-GPU fits an unknown-memory GPU and takes it exclusively.
	assert.True(t, h.checkConsumableGPUMemory("uuid-A", wholeGPUDemand()))
	h.consumeGPUMemory("uuid-A", wholeGPUDemand())
	assert.Equal(t, "WHOLE_GPU", gpuModeOf(h, "uuid-A"))
	assert.Equal(t, int64(0), gpuRemaining(h, "uuid-A"))

	// A second whole-GPU request is refused: exclusivity holds (no double-book).
	assert.False(t, h.checkConsumableGPUMemory("uuid-A", wholeGPUDemand()))

	// Any MPS share can never fit an unknown-memory GPU.
	assert.False(t, h.checkConsumableGPUMemory("uuid-A", mpsDemand(1)))
	assert.False(t, h.checkConsumableGPUMemory("uuid-A", mpsDemand(4096)))

	// Release returns it to derived FREE with remaining back at total (0).
	h.releaseGPUMemory("uuid-A", wholeGPUDemand())
	assert.Equal(t, "FREE", gpuModeOf(h, "uuid-A"))
	assert.Equal(t, int64(0), gpuRemaining(h, "uuid-A"))
}

// TestGPUMemoryUnknownMemoryReconsumeStaysZero mirrors restart reconcile: a
// whole-GPU task on an unknown-memory GPU is re-consumed, and remaining must
// never go negative (the whole-GPU path never subtracts).
func TestGPUMemoryUnknownMemoryReconsumeStaysZero(t *testing.T) {
	h := hrmWithPool(0)
	h.consumeGPUMemory("uuid-A", wholeGPUDemand())
	h.consumeGPUMemory("uuid-A", wholeGPUDemand())
	assert.Equal(t, int64(0), gpuRemaining(h, "uuid-A"))
	assert.Equal(t, int64(0), h.gpuMemoryConsumed["uuid-A"].MiB)
	assert.Equal(t, "WHOLE_GPU", gpuModeOf(h, "uuid-A"))
}

// TestGPUMemoryPerUUIDIndependence verifies that consuming one GPU leaves the
// others untouched, including a mix of a known-memory GPU and an unknown-memory
// (total 0) GPU on the same instance.
func TestGPUMemoryPerUUIDIndependence(t *testing.T) {
	h := hrmWithPoolMulti(map[string]int64{"uuid-A": 22563, "uuid-B": 0})

	assert.True(t, h.checkConsumableGPUMemory("uuid-A", mpsDemand(4096)))
	h.consumeGPUMemory("uuid-A", mpsDemand(4096))
	assert.Equal(t, int64(18467), gpuRemaining(h, "uuid-A"))
	assert.Equal(t, "MPS", gpuModeOf(h, "uuid-A"))
	assert.Equal(t, int64(0), gpuRemaining(h, "uuid-B"))
	assert.Equal(t, "FREE", gpuModeOf(h, "uuid-B"), "unknown-memory GPU untouched")

	// Whole-GPU on the unknown-memory GPU is admitted and leaves A as it was.
	assert.True(t, h.checkConsumableGPUMemory("uuid-B", wholeGPUDemand()))
	h.consumeGPUMemory("uuid-B", wholeGPUDemand())
	assert.Equal(t, "WHOLE_GPU", gpuModeOf(h, "uuid-B"))
	assert.Equal(t, "MPS", gpuModeOf(h, "uuid-A"), "GPU-A unaffected by GPU-B consume")
	assert.Equal(t, int64(18467), gpuRemaining(h, "uuid-A"))

	// MPS is still refused on the unknown-memory GPU.
	assert.False(t, h.checkConsumableGPUMemory("uuid-B", mpsDemand(4096)))
}

// TestGPUModeDerivation covers the gpuMode helper that derives the display state
// from a consumed record (replaces the old gpuMode enum String()).
func TestGPUModeDerivation(t *testing.T) {
	assert.Equal(t, "FREE", gpuMode(apitask.GPUMemoryDemand{}))
	assert.Equal(t, "MPS", gpuMode(apitask.GPUMemoryDemand{MiB: 4096}))
	assert.Equal(t, "WHOLE_GPU", gpuMode(apitask.GPUMemoryDemand{WholeGPU: true}))
}

// ---- Edge cases the consumed-model can express that the enum could not ----

// TestGPUMemoryNeverProducesIllegalCombo (edge case 1) verifies the consumed
// record is never driven to the illegal {MiB>0, WholeGPU:true} state through the
// normal consume/consumable paths, in both orders.
func TestGPUMemoryNeverProducesIllegalCombo(t *testing.T) {
	// MPS first, then a whole-GPU attempt is refused, so WholeGPU never flips.
	h := hrmWithPool(23040)
	h.consumeGPUMemory("uuid-A", mpsDemand(4096))
	assert.False(t, h.checkConsumableGPUMemory("uuid-A", wholeGPUDemand()),
		"whole-GPU must be refused on an MPS-held GPU")
	assert.Positive(t, h.gpuMemoryConsumed["uuid-A"].MiB)
	assert.False(t, h.gpuMemoryConsumed["uuid-A"].WholeGPU, "MPS state must not carry WholeGPU")

	// Whole-GPU first, then an MPS attempt is refused, so MiB stays 0.
	h2 := hrmWithPool(23040)
	h2.consumeGPUMemory("uuid-A", wholeGPUDemand())
	assert.False(t, h2.checkConsumableGPUMemory("uuid-A", mpsDemand(4096)),
		"MPS must be refused on a whole-GPU-held GPU")
	assert.True(t, h2.gpuMemoryConsumed["uuid-A"].WholeGPU)
	assert.Zero(t, h2.gpuMemoryConsumed["uuid-A"].MiB, "whole-GPU state must not carry a positive MiB")
}

// TestGPUMemoryMPSFillsToExactlyZero (edge case 3) fills a known GPU to exactly
// remaining 0 with MPS shares. Because the mode is MPS (not WHOLE_GPU), a
// whole-GPU request is refused and further MPS is refused - the GPU is full, not
// exclusive.
func TestGPUMemoryMPSFillsToExactlyZero(t *testing.T) {
	h := hrmWithPool(8192)
	assert.True(t, h.checkConsumableGPUMemory("uuid-A", mpsDemand(8192)))
	h.consumeGPUMemory("uuid-A", mpsDemand(8192))
	assert.Equal(t, int64(0), gpuRemaining(h, "uuid-A"))
	assert.Equal(t, "MPS", gpuModeOf(h, "uuid-A"), "full MPS GPU is MPS, not WHOLE_GPU")

	// A whole-GPU request is refused: the GPU is not free even though remaining is 0.
	assert.False(t, h.checkConsumableGPUMemory("uuid-A", wholeGPUDemand()))
	// Any further MPS is refused: no room left.
	assert.False(t, h.checkConsumableGPUMemory("uuid-A", mpsDemand(1)))

	// Releasing the share returns the GPU to FREE and admits a whole-GPU task.
	h.releaseGPUMemory("uuid-A", mpsDemand(8192))
	assert.Equal(t, "FREE", gpuModeOf(h, "uuid-A"))
	assert.True(t, h.checkConsumableGPUMemory("uuid-A", wholeGPUDemand()))
}

// TestGPUMemoryRestartReconsumeIdempotent (edge case 5) mirrors restart
// reconcile: re-consuming the identical set of demands rebuilds an identical
// consumed record, with no drift or negatives.
func TestGPUMemoryRestartReconsumeIdempotent(t *testing.T) {
	build := func() *HostResourceManager {
		h := hrmWithPoolMulti(map[string]int64{"uuid-A": 14911, "uuid-B": 14911})
		// GPU-A shared by two MPS shares; GPU-B taken whole.
		h.consumeGPUMemory("uuid-A", mpsDemand(4096))
		h.consumeGPUMemory("uuid-A", mpsDemand(4096))
		h.consumeGPUMemory("uuid-B", wholeGPUDemand())
		return h
	}

	first := build()
	second := build()

	// The two independently-built consumed maps must be identical.
	assert.Equal(t, first.gpuMemoryConsumed, second.gpuMemoryConsumed)
	assert.Equal(t, int64(8192), second.gpuMemoryConsumed["uuid-A"].MiB)
	assert.Equal(t, int64(6719), gpuRemaining(second, "uuid-A"))
	assert.Equal(t, "MPS", gpuModeOf(second, "uuid-A"))
	assert.Equal(t, "WHOLE_GPU", gpuModeOf(second, "uuid-B"))
	// No consumed value went negative.
	for uuid, consumed := range second.gpuMemoryConsumed {
		assert.GreaterOrEqual(t, consumed.MiB, int64(0), "consumed MiB negative for %s", uuid)
	}
}

// TestConsumeReleaseGPUMemoryThroughPublicAPI drives a GPU memory demand through
// the real consume() and release() entry points (not the internal pool helpers).
func TestConsumeReleaseGPUMemoryThroughPublicAPI(t *testing.T) {
	h := getTestHostResourceManager(int32(2048), int32(2048), []string{}, []string{}, []string{"gpu1"})

	arn := "arn:aws:ecs:us-east-1:0:task/cluster/1"
	gpuMemory := map[string]apitask.GPUMemoryDemand{"gpu1": mpsDemand(4096)}

	consumed, err := h.consume(arn, nil, gpuMemory)
	assert.NoError(t, err)
	assert.True(t, consumed)
	// getTestHostResourceManager seeds each GPU with 16384 MiB.
	assert.Equal(t, int64(16384-4096), gpuRemaining(h, "gpu1"))
	assert.Equal(t, "MPS", gpuModeOf(h, "gpu1"))

	err = h.release(arn, nil, gpuMemory)
	assert.NoError(t, err)
	assert.Equal(t, int64(16384), gpuRemaining(h, "gpu1"))
	assert.Equal(t, "FREE", gpuModeOf(h, "gpu1"))
}

// TestConsumeGPUMemoryOverCapacityThroughPublicAPI verifies a demand larger than
// remaining is rejected by consume() (task stays pending).
func TestConsumeGPUMemoryOverCapacityThroughPublicAPI(t *testing.T) {
	h := getTestHostResourceManager(int32(2048), int32(2048), []string{}, []string{}, []string{"gpu1"})

	arn := "arn:aws:ecs:us-east-1:0:task/cluster/1"
	// getTestHostResourceManager seeds 16384 MiB; ask for more.
	gpuMemory := map[string]apitask.GPUMemoryDemand{"gpu1": mpsDemand(20000)}

	consumed, err := h.consume(arn, nil, gpuMemory)
	assert.NoError(t, err)
	assert.False(t, consumed, "over-capacity GPU memory demand must not be consumed")
	assert.Equal(t, int64(16384), gpuRemaining(h, "gpu1"), "pool must be unchanged on a rejected consume")
}

// TestReconsumeGPUMemoryAfterCapacityRegression documents the restart edge where
// a GPU's memory is rediscovered lower than what its running tasks hold (e.g.
// unreported -> total 0). consume() refuses the running task's demand, so the
// pool under-counts and shows the GPU with room it does not physically have.
// reconcileHostResources logs this as a Critical GPU accounting desync; the pool
// self-heals on the next restart with good discovery.
func TestReconsumeGPUMemoryAfterCapacityRegression(t *testing.T) {
	// GPU rediscovered with total 0, but a running task holds 8192 MiB of MPS.
	h := getTestHostResourceManager(int32(2048), int32(2048), []string{}, []string{}, []string{})
	h.gpuMemoryTotalMiB["gpu1"] = 0

	arn := "arn:aws:ecs:us-east-1:0:task/cluster/1"
	gpuMemory := map[string]apitask.GPUMemoryDemand{"gpu1": mpsDemand(8192)}

	consumed, err := h.consume(arn, nil, gpuMemory)
	assert.NoError(t, err)
	assert.False(t, consumed, "MPS demand cannot re-consume against a regressed total 0")
	// The desync: nothing is recorded, so the GPU reads FREE with remaining 0.
	assert.Equal(t, int64(0), gpuRemaining(h, "gpu1"))
	assert.Equal(t, "FREE", gpuModeOf(h, "gpu1"))
}

// TestConsumeMixedCPUAndGPUMemory verifies GPU memory demand coexists with the
// INTEGER (CPU/MEMORY) resources in a single consume call.
func TestConsumeMixedCPUAndGPUMemory(t *testing.T) {
	h := getTestHostResourceManager(int32(2048), int32(2048), []string{}, []string{}, []string{"gpu1"})

	arn := "arn:aws:ecs:us-east-1:0:task/cluster/1"
	resources := map[string]types.Resource{
		"CPU":    {Name: utils.Strptr("CPU"), Type: utils.Strptr("INTEGER"), IntegerValue: 512},
		"MEMORY": {Name: utils.Strptr("MEMORY"), Type: utils.Strptr("INTEGER"), IntegerValue: 768},
	}
	gpuMemory := map[string]apitask.GPUMemoryDemand{"gpu1": mpsDemand(4096)}

	consumed, err := h.consume(arn, resources, gpuMemory)
	assert.NoError(t, err)
	assert.True(t, consumed)
	assert.Equal(t, int32(512), h.consumedResource["CPU"].IntegerValue)
	assert.Equal(t, int32(768), h.consumedResource["MEMORY"].IntegerValue)
	assert.Equal(t, int64(16384-4096), gpuRemaining(h, "gpu1"))
}

// TestCheckResourcesHealthGPUMemory: a known UUID passes, an unknown UUID errors.
func TestCheckResourcesHealthGPUMemory(t *testing.T) {
	h := getTestHostResourceManager(int32(2048), int32(2048), []string{}, []string{}, []string{"gpu1"})

	err := h.checkResourcesHealth(nil, map[string]apitask.GPUMemoryDemand{"gpu1": mpsDemand(4096)})
	assert.NoError(t, err)

	err = h.checkResourcesHealth(nil, map[string]apitask.GPUMemoryDemand{"gpu-unknown": mpsDemand(4096)})
	assert.Error(t, err)
}

// TestGPUMemoryCapacityKeyHelpers covers the capacity key round trip used to
// seed the pool from the host resource map.
func TestGPUMemoryCapacityKeyHelpers(t *testing.T) {
	uuid, ok := gpuUUIDFromCapacityKey("GPU_MEMORY:uuid-A")
	assert.True(t, ok)
	assert.Equal(t, "uuid-A", uuid)

	_, ok = gpuUUIDFromCapacityKey("CPU")
	assert.False(t, ok)
}

// TestFormatGPUMemoryEntry asserts the pool snapshot string for each derived
// state. A whole-GPU device must read remaining=0 mode=WHOLE_GPU even though its
// consumed MiB is 0, and an unknown-memory device must read total=unknown.
func TestFormatGPUMemoryEntry(t *testing.T) {
	assert.Equal(t, "uuid-A{total=23040 remaining=23040 mode=FREE} ",
		formatGPUMemoryEntry("uuid-A", 23040, apitask.GPUMemoryDemand{}))
	assert.Equal(t, "uuid-A{total=23040 remaining=18944 mode=MPS} ",
		formatGPUMemoryEntry("uuid-A", 23040, apitask.GPUMemoryDemand{MiB: 4096}))
	assert.Equal(t, "uuid-A{total=23040 remaining=0 mode=WHOLE_GPU} ",
		formatGPUMemoryEntry("uuid-A", 23040, apitask.GPUMemoryDemand{WholeGPU: true}))
	assert.Equal(t, "uuid-A{total=unknown remaining=0 mode=FREE} ",
		formatGPUMemoryEntry("uuid-A", 0, apitask.GPUMemoryDemand{}))
	assert.Equal(t, "uuid-A{total=unknown remaining=0 mode=WHOLE_GPU} ",
		formatGPUMemoryEntry("uuid-A", 0, apitask.GPUMemoryDemand{WholeGPU: true}))
}
