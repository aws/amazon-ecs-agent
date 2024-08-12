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

package stats

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	predictableHighMemoryUtilizationInBytes = 7377772544

	// predictableHighMemoryUtilizationInMiB is the expected Memory usage in MiB for
	// the "predictableHighMemoryUtilizationInBytes" value (7377772544 / (1024 * 1024))
	predictableHighMemoryUtilizationInMiB = 7035
	// the "predictableInt64Overflow" requires 3 metrics to guarantee overflow
	// note math.MaxInt64 is odd, so integer division will trim off 1
	predictableInt64Overflow = math.MaxInt64 / int64(2)
)

var now time.Time = time.Now()

func getTimestamps() []time.Time {
	return []time.Time{
		now.Add(-time.Millisecond * 2100),
		now.Add(-time.Millisecond * 2000),
		now.Add(-time.Millisecond * 1900),
		now.Add(-time.Millisecond * 1800),
		now.Add(-time.Millisecond * 1700),
		now.Add(-time.Millisecond * 1600),
		now.Add(-time.Millisecond * 1500),
		now.Add(-time.Millisecond * 1400),
		now.Add(-time.Millisecond * 1300),
		now.Add(-time.Millisecond * 1200),
		now.Add(-time.Millisecond * 1100),
		now.Add(-time.Millisecond * 1000),
		now.Add(-time.Millisecond * 900),
		now.Add(-time.Millisecond * 800),
		now.Add(-time.Millisecond * 700),
		now.Add(-time.Millisecond * 600),
		now.Add(-time.Millisecond * 500),
		now.Add(-time.Millisecond * 400),
		now.Add(-time.Millisecond * 300),
		now.Add(-time.Millisecond * 200),
		now.Add(-time.Millisecond * 100),
		now,
	}
}

func getUintStats() []uint64 {
	return []uint64{
		22400432,
		116499979,
		248503503,
		372167097,
		502862518,
		638485801,
		780707806,
		911624529,
		1047689820,
		1177013119,
		1313474186,
		1449445062,
		1586294238,
		1719604012,
		1837238842,
		1974606362,
		2112444996,
		2248922292,
		2382142527,
		2516445820,
		2653783456,
		2666483380,
	}
}

func getRandomMemoryUtilizationInBytes() []uint64 {
	return []uint64{
		1839104,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		716800,
	}
}

func getTestRestartCount(i int64, includeRestartCount bool) *int64 {
	if !includeRestartCount {
		return nil
	}
	return &i
}

func getPredictableHighMemoryUtilizationInBytes(size int) []uint64 {
	var memBytes []uint64
	for i := 0; i < size; i++ {
		memBytes = append(memBytes, predictableHighMemoryUtilizationInBytes)
	}
	return memBytes
}

func getLargeInt64Stats(size int) []uint64 {
	var uintStats []uint64
	for i := 0; i < size; i++ {
		uintStats = append(uintStats, uint64(predictableInt64Overflow))
	}
	return uintStats
}

func getContainerStats(predictableHighUtilization bool, includeRestartCount bool) []*ContainerStats {
	timestamps := getTimestamps()
	cpuTimes := getUintStats()
	var memoryUtilizationInBytes []uint64
	var uintStats []uint64
	if predictableHighUtilization {
		memoryUtilizationInBytes = getPredictableHighMemoryUtilizationInBytes(len(cpuTimes))
		uintStats = getLargeInt64Stats(len(cpuTimes))
	} else {
		memoryUtilizationInBytes = getRandomMemoryUtilizationInBytes()
		uintStats = getUintStats()
	}
	stats := []*ContainerStats{}
	for i, time := range timestamps {
		stats = append(stats, &ContainerStats{
			cpuUsage:          cpuTimes[i],
			memoryUsage:       memoryUtilizationInBytes[i],
			storageReadBytes:  uintStats[i],
			storageWriteBytes: uintStats[i],
			networkStats: &NetworkStats{
				RxBytes:   uintStats[i],
				RxDropped: 0,
				RxErrors:  uintStats[i],
				RxPackets: uintStats[i],
				TxBytes:   uintStats[i],
				TxDropped: uintStats[i],
				TxErrors:  0,
				TxPackets: uintStats[i],
			},
			restartCount: getTestRestartCount(int64(i), includeRestartCount),
			timestamp:    time})
	}
	return stats
}

func createQueue(size int, predictableHighUtilization bool) *Queue {
	stats := getContainerStats(predictableHighUtilization, false)
	queue := NewQueue(size)
	for _, stat := range stats {
		queue.add(stat)
	}
	return queue
}

func TestQueueReset(t *testing.T) {
	queue := NewQueue(10)
	// empty queue should throw errors getting stats sets:
	_, err := queue.GetCPUStatsSet()
	require.Error(t, err)
	_, err = queue.GetMemoryStatsSet()
	require.Error(t, err)
	_, err = queue.GetStorageStatsSet()
	require.Error(t, err)
	_, err = queue.GetNetworkStatsSet()
	require.Error(t, err)
	_, err = queue.GetRestartStatsSet()
	require.Error(t, err)

	// add some metrics, and getting stats sets should now succeed:
	stats := getContainerStats(false, false)
	for i := 0; i < 4; i++ {
		queue.add(stats[i])
	}
	_, err = queue.GetCPUStatsSet()
	require.NoError(t, err)
	_, err = queue.GetMemoryStatsSet()
	require.NoError(t, err)
	_, err = queue.GetStorageStatsSet()
	require.NoError(t, err)
	_, err = queue.GetNetworkStatsSet()
	require.NoError(t, err)
	// restart stats still fail if not added
	_, err = queue.GetRestartStatsSet()
	require.Error(t, err)

	// after resetting the queue, there are no metrics to send and getting stats sets should error again:
	queue.Reset()
	_, err = queue.GetCPUStatsSet()
	require.Error(t, err)
	_, err = queue.GetMemoryStatsSet()
	require.Error(t, err)
	_, err = queue.GetStorageStatsSet()
	require.Error(t, err)
	_, err = queue.GetNetworkStatsSet()
	require.Error(t, err)
	_, err = queue.GetRestartStatsSet()
	require.Error(t, err)

	// add single stat to queue and getting stats sets should succeed again:
	queue.add(stats[4])
	_, err = queue.GetCPUStatsSet()
	require.NoError(t, err)
	_, err = queue.GetMemoryStatsSet()
	require.NoError(t, err)
	_, err = queue.GetStorageStatsSet()
	require.NoError(t, err)
	_, err = queue.GetNetworkStatsSet()
	require.NoError(t, err)
	_, err = queue.GetRestartStatsSet()
	require.Error(t, err)
}

func TestQueueWithRestartStats(t *testing.T) {
	queue := NewQueue(10)
	// empty queue should throw errors getting stats sets:
	restartStats, err := queue.GetRestartStatsSet()
	require.Error(t, err)
	require.Nil(t, restartStats)

	// add metrics with restart count, and getting should now succeed.
	stats := getContainerStats(false, true)
	for i := 0; i < 4; i++ {
		queue.add(stats[i])
	}
	restartStats, err = queue.GetRestartStatsSet()
	require.NoError(t, err)
	require.NotNil(t, restartStats)
	require.NotNil(t, restartStats.RestartCount)
	require.Equal(t, int64(3), *restartStats.RestartCount)

	// after resetting the queue, there are no metrics to send and getting stats sets should error again:
	queue.Reset()
	restartStats, err = queue.GetRestartStatsSet()
	require.Error(t, err)
	require.Nil(t, restartStats)

	// add single stat to queue and getting stats sets should succeed again:
	queue.add(stats[4])
	restartStats, err = queue.GetRestartStatsSet()
	require.NoError(t, err)
	require.NotNil(t, restartStats)
	require.NotNil(t, restartStats.RestartCount)
	require.Equal(t, int64(1), *restartStats.RestartCount)

	// add a stat with a lower restart count, and verify that this returns an error
	queue.add(stats[2])
	restartStats, err = queue.GetRestartStatsSet()
	require.Error(t, err)
	require.Nil(t, restartStats)
	queue.Reset()

	// add rest of metrics, overflowing queue, and verify values
	for i := 5; i < len(stats); i++ {
		queue.add(stats[i])
	}
	restartStats, err = queue.GetRestartStatsSet()
	require.NoError(t, err)
	require.NotNil(t, restartStats)
	require.NotNil(t, restartStats.RestartCount)
	// expect count to be length of queue (10) minus 1
	require.Equal(t, int64(9), *restartStats.RestartCount)
}

func TestQueueAddRemove(t *testing.T) {
	timestamps := getTimestamps()
	queueLength := 5
	// Set predictableHighUtilization to false, expect random values when aggregated.
	queue := createQueue(queueLength, false)
	buf := queue.buffer
	require.Len(t, buf, queueLength, "Buffer size is incorrect.")

	timestampsIndex := len(timestamps) - len(buf)
	for i, stat := range buf {
		if stat.Timestamp != timestamps[timestampsIndex+i] {
			t.Errorf("Unexpected value for Stats element in buffer. expected %s got %s", timestamps[timestampsIndex+i], stat.Timestamp)
		}
	}

	cpuStatsSet, err := queue.GetCPUStatsSet()
	require.NoError(t, err)
	require.NotEqual(t, math.MaxFloat64, *cpuStatsSet.Min)
	require.NotEqual(t, math.NaN(), *cpuStatsSet.Min)
	require.NotEqual(t, -math.MaxFloat64, *cpuStatsSet.Max)
	require.NotEqual(t, math.NaN(), *cpuStatsSet.Max)
	require.Equal(t, int64(queueLength), *cpuStatsSet.SampleCount)
	require.Equal(t, int(554), int(*cpuStatsSet.Sum))

	memStatsSet, err := queue.GetMemoryStatsSet()
	require.NoError(t, err)
	require.NotEqual(t, math.MaxFloat64, *memStatsSet.Min)
	require.NotEqual(t, math.NaN(), *memStatsSet.Min)
	require.NotEqual(t, -math.MaxFloat64, *memStatsSet.Max)
	require.NotEqual(t, math.NaN(), *memStatsSet.Max)
	require.Equal(t, int64(queueLength), *memStatsSet.SampleCount)
	require.Equal(t, int(12), int(*memStatsSet.Sum))

	storageStatsSet, err := queue.GetStorageStatsSet()
	require.NoError(t, err)
	// assuming min is initialized to math.MaxUint64 then truncated
	storageReadStatsSet := storageStatsSet.ReadSizeBytes
	require.NotEqual(t, math.MaxInt64, *storageReadStatsSet.Min)
	require.NotEqual(t, math.MaxInt64, *storageReadStatsSet.OverflowMin)
	require.NotEqual(t, 0, *storageReadStatsSet.Max)
	require.Equal(t, int64(queueLength), *storageReadStatsSet.SampleCount)
	require.Equal(t, int64(12467777475), *storageReadStatsSet.Sum)

	storageWriteStatsSet := storageStatsSet.WriteSizeBytes
	require.NotEqual(t, math.MaxInt64, *storageWriteStatsSet.Min)
	require.NotEqual(t, 0, *storageWriteStatsSet.Max)
	require.Equal(t, int64(queueLength), *storageWriteStatsSet.SampleCount)
	require.Equal(t, int64(12467777475), *storageWriteStatsSet.Sum)

	netStatsSet, err := queue.GetNetworkStatsSet()
	require.NoError(t, err, "error getting network stats set")
	validateNetStatsSet(t, netStatsSet, queueLength)
}

func validateNetStatsSet(t *testing.T, netStats *ecstcs.NetworkStatsSet, queueLen int) {
	// checking only the fields RxBytes, RxDropped, TxBytes, TxErrors since others are similar
	assert.NotEqual(t, int64(math.MaxInt64), *netStats.RxBytes.Min, "incorrect rxbytes min")
	assert.Equal(t, int64(0), *netStats.RxBytes.OverflowMin, "incorrect rxbytes overlfowMin")
	assert.NotEqual(t, int64(0), *netStats.RxBytes.Max, "incorrect rxbytes max")
	assert.Equal(t, int64(0), *netStats.RxBytes.OverflowMax, "incorrect rxbytes overlfowMax")
	assert.Equal(t, int64(queueLen), *netStats.RxBytes.SampleCount, "incorrect rxbytes sampleCount")
	assert.NotEqual(t, int64(0), *netStats.RxBytes.Sum, "incorrect rxbytes sum")
	assert.Equal(t, int64(0), *netStats.RxBytes.OverflowSum, "incorrect rxbytes overlfowSum")

	assert.Equal(t, int64(0), *netStats.RxDropped.Min, "incorrect RxDropped min")
	assert.Equal(t, int64(0), *netStats.RxDropped.OverflowMin, "incorrect RxDropped overlfowMin")
	assert.Equal(t, int64(0), *netStats.RxDropped.Max, "incorrect RxDropped max")
	assert.Equal(t, int64(0), *netStats.RxDropped.OverflowMax, "incorrect RxDropped overlfowMax")
	assert.Equal(t, int64(queueLen), *netStats.RxDropped.SampleCount, "incorrect RxDropped sampleCount")
	assert.Equal(t, int64(0), *netStats.RxDropped.Sum, "incorrect RxDropped sum")
	assert.Equal(t, int64(0), *netStats.RxDropped.OverflowSum, "incorrect RxDropped overlfowSum")

	assert.NotEqual(t, int64(math.MaxInt64), *netStats.TxBytes.Min, "incorrect TxBytes min")
	assert.Equal(t, int64(0), *netStats.TxBytes.OverflowMin, "incorrect TxBytes overlfowMin")
	assert.NotEqual(t, int64(0), *netStats.TxBytes.Max, "incorrect TxBytes max")
	assert.Equal(t, int64(0), *netStats.TxBytes.OverflowMax, "incorrect TxBytes overlfowMax")
	assert.Equal(t, int64(queueLen), *netStats.TxBytes.SampleCount, "incorrect TxBytes sampleCount")
	assert.NotEqual(t, int64(0), *netStats.TxBytes.Sum, "incorrect TxBytes sum")
	assert.Equal(t, int64(0), *netStats.TxBytes.OverflowSum, "incorrect TxBytes overlfowSum")

	assert.Equal(t, int64(0), *netStats.TxErrors.Min, "incorrect TxErrors min")
	assert.Equal(t, int64(0), *netStats.TxErrors.OverflowMin, "incorrect TxErrors overlfowMin")
	assert.Equal(t, int64(0), *netStats.TxErrors.Max, "incorrect TxErrors max")
	assert.Equal(t, int64(0), *netStats.TxErrors.OverflowMax, "incorrect TxErrors overlfowMax")
	assert.Equal(t, int64(queueLen), *netStats.TxErrors.SampleCount, "incorrect TxErrors sampleCount")
	assert.Equal(t, int64(0), *netStats.TxErrors.Sum, "incorrect TxErrors sum")
	assert.Equal(t, int64(0), *netStats.TxErrors.OverflowSum, "incorrect TxErrors overlfowSum")

	assert.NotNil(t, *netStats.RxBytesPerSecond, "incorrect RxBytesPerSecond set")
	assert.Equal(t, float64(1.26999248e+08), *netStats.RxBytesPerSecond.Min, "incorrect RxBytesPerSecond min")
	assert.Equal(t, float64(1.373376384e+09), *netStats.RxBytesPerSecond.Max, "incorrect RxBytesPerSecond max")
	assert.Equal(t, int64(queueLen), *netStats.RxBytesPerSecond.SampleCount, "incorrect RxBytesPerSecond sampleCount")
	assert.Equal(t, float64(5.540383824e+09), *netStats.RxBytesPerSecond.Sum, "incorrect RxBytesPerSecond sum")

	assert.NotNil(t, *netStats.TxBytesPerSecond, "incorrect TxBytesPerSecond set")
	assert.Equal(t, float64(1.26999248e+08), *netStats.TxBytesPerSecond.Min, "incorrect TxBytesPerSecond min")
	assert.Equal(t, float64(1.373376384e+09), *netStats.TxBytesPerSecond.Max, "incorrect TxBytesPerSecond max")
	assert.Equal(t, int64(queueLen), *netStats.TxBytesPerSecond.SampleCount, "incorrect TxBytesPerSecond sampleCount")
	assert.Equal(t, float64(5.540383824e+09), *netStats.TxBytesPerSecond.Sum, "incorrect TxBytesPerSecond sum")
}

func TestQueueUintStats(t *testing.T) {
	queueLength := 3
	queue := createQueue(queueLength, true)
	buf := queue.buffer
	if len(buf) != queueLength {
		t.Errorf("Buffer size is incorrect. Expected: %d, Got: %d", queueLength, len(buf))
	}

	storageStatsSet, err := queue.GetStorageStatsSet()
	storageReadStatsSet := storageStatsSet.ReadSizeBytes

	if err != nil {
		t.Error("Error getting storage read stats set:", err)
	}
	// assuming min is initialized to math.MaxUint64 then truncated
	// min/max should be the same as predictableInt64Overflow
	// their overflow should be 0
	assert.Equal(t, *storageReadStatsSet.Min, predictableInt64Overflow)
	assert.Equal(t, *storageReadStatsSet.OverflowMin, int64(0))
	assert.Equal(t, *storageReadStatsSet.Max, predictableInt64Overflow)
	assert.Equal(t, *storageReadStatsSet.OverflowMax, int64(0))
	// the sum of three predictableInt64Overflow should be equal to MaxInt64
	// with an overflow of predictableInt64Overflow - 1
	// (see the definition of predictableInt64Overflow for why -1)
	assert.Equal(t, *storageReadStatsSet.Sum, int64(math.MaxInt64))
	assert.Equal(t, *storageReadStatsSet.OverflowSum, predictableInt64Overflow-1)
}

func TestQueueAddPredictableHighMemoryUtilization(t *testing.T) {
	timestamps := getTimestamps()
	queueLength := 5
	// Set predictableHighUtilization to true
	// This lets us compare the computed values against pre-computed expected values
	queue := createQueue(queueLength, true)
	buf := queue.buffer
	require.Len(t, buf, queueLength, "Buffer size is incorrect.")

	timestampsIndex := len(timestamps) - len(buf)
	for i, stat := range buf {
		if stat.Timestamp != timestamps[timestampsIndex+i] {
			t.Error("Unexpected value for Stats element in buffer")
		}
	}

	memStatsSet, err := queue.GetMemoryStatsSet()
	if err != nil {
		t.Error("Error getting memory stats set:", err)
	}

	// Test if both min and max for memory utilization are set to 7035MiB
	// Also test if sum  == queue length * 7035
	expectedMemoryUsageInMiB := float64(predictableHighMemoryUtilizationInMiB)
	expectedMemoryUsageInMiBSum := expectedMemoryUsageInMiB * float64(queueLength)
	require.Equal(t, *memStatsSet.Min, expectedMemoryUsageInMiB)
	require.Equal(t, *memStatsSet.Max, expectedMemoryUsageInMiB)
	require.Equal(t, *memStatsSet.SampleCount, int64(queueLength))
	require.Equal(t, *memStatsSet.Sum, expectedMemoryUsageInMiBSum)
}

// tests just below and just above the threshold (+/- 1) of int64
func TestUintOverflow(t *testing.T) {
	var underUint, overUint uint64
	underUint = uint64(math.MaxInt64 - 1)
	overUint = uint64(math.MaxInt64 + 1)

	baseUnderUint, overflowUnderUint := getInt64WithOverflow(underUint)
	baseMaxInt, overflowMaxInt := getInt64WithOverflow(uint64(math.MaxInt64))
	baseOverUint, overflowOverUint := getInt64WithOverflow(overUint)

	assert.Equal(t, baseUnderUint, int64(math.MaxInt64-1))
	assert.Equal(t, overflowUnderUint, int64(0))
	assert.Equal(t, baseMaxInt, int64(math.MaxInt64))
	assert.Equal(t, overflowMaxInt, int64(0))
	assert.Equal(t, baseOverUint, int64(math.MaxInt64))
	assert.Equal(t, overflowOverUint, int64(1))
}

func TestCpuStatsSetNotSetToInfinity(t *testing.T) {
	// timestamps will be used to simulate +Inf CPU Usage
	// timestamps[0] = timestamps[1]
	now := time.Now()
	timestamps := []time.Time{
		now.Add(-time.Microsecond * 1000),
		now.Add(-time.Microsecond * 1000),
		now,
	}
	cpuTimes := []uint64{
		0,
		10000000,
		20000000,
	}

	// Create and add container stats
	queueLength := 3
	queue := NewQueue(queueLength)
	for i, time := range timestamps {
		queue.add(&ContainerStats{cpuUsage: cpuTimes[i], memoryUsage: 1024, timestamp: time})
	}
	cpuStatsSet, err := queue.GetCPUStatsSet()
	if err != nil {
		t.Errorf("Error getting cpu stats set: %v", err)
	}
	require.Equal(t, float64(1000), *cpuStatsSet.Max)
	require.Equal(t, float64(1000), *cpuStatsSet.Min)
	require.Equal(t, float64(1000), *cpuStatsSet.Sum)
	require.Equal(t, int64(1), *cpuStatsSet.SampleCount)
}

func TestCPUStatSetFailsWhenSampleCountIsZero(t *testing.T) {
	timestamps := []time.Time{
		parseNanoTime("2015-02-12T21:22:05.131117533Z"),
		parseNanoTime("2015-02-12T21:22:05.131117533Z"),
	}
	cpuTimes := []uint64{
		22400432,
		116499979,
	}
	memoryUtilizationInBytes := []uint64{
		3649536,
		3649536,
	}
	// create a queue
	queue := NewQueue(3)

	for i, time := range timestamps {
		queue.add(&ContainerStats{cpuUsage: cpuTimes[i], memoryUsage: memoryUtilizationInBytes[i], timestamp: time})
	}

	// if two cpu had identical timestamps,
	// then there will not be enough valid cpu percentage stats to create
	// a valid CpuStatsSet, and this function call should fail.
	_, err := queue.GetCPUStatsSet()
	require.Error(t, err)
}

func TestCPUStatsWithIdenticalTimestampsGetSameUsagePercent(t *testing.T) {
	now := time.Now()
	timestamps := []time.Time{
		now.Add(-time.Nanosecond * 2),
		now.Add(-time.Nanosecond * 1),
		now,
		now,
	}
	cpuTimes := []uint64{
		0,
		1,
		3,
		4,
	}

	// create a queue
	queue := NewQueue(4)

	for i, time := range timestamps {
		queue.add(&ContainerStats{cpuUsage: cpuTimes[i], memoryUsage: 3649536, timestamp: time})
	}

	// if there were three cpu metrics, and two had identical timestamps,
	// then there will not be enough valid cpu percentage stats to create
	// a valid CpuStatsSet, and this function call should fail.
	statSet, err := queue.GetCPUStatsSet()
	require.NoError(t, err)
	require.Equal(t, float64(200), *statSet.Max)
	require.Equal(t, float64(100), *statSet.Min)
	require.Equal(t, int64(3), *statSet.SampleCount)
	require.Equal(t, float64(500), *statSet.Sum)
}

func TestHugeCPUUsagePercentDoesntGetCapped(t *testing.T) {
	now := time.Now()
	timestamps := []time.Time{
		now.Add(-time.Nanosecond * 2),
		now.Add(-time.Nanosecond * 1),
		now,
	}
	cpuTimes := []uint64{
		0,
		1,
		300000000,
	}
	// create a queue
	queue := NewQueue(4)

	for i, time := range timestamps {
		queue.add(&ContainerStats{cpuUsage: cpuTimes[i], memoryUsage: 3649536, timestamp: time})
	}

	statSet, err := queue.GetCPUStatsSet()
	require.NoError(t, err)
	require.Equal(t, float64(30000001024), *statSet.Max)
	require.Equal(t, float64(100), *statSet.Min)
	require.Equal(t, int64(2), *statSet.SampleCount)
	require.Equal(t, float64(30000001124), *statSet.Sum)
}

// If there are only 2 datapoints, and both have the same timestamp,
// then sample count will be 0 for per sec metrics and GetNetworkStats should return error
func TestPerSecNetworkStatSetFailsWhenSampleCountIsZero(t *testing.T) {
	timestamps := []time.Time{
		parseNanoTime("2015-02-12T21:22:05.131117533Z"),
		parseNanoTime("2015-02-12T21:22:05.131117533Z"),
	}
	cpuTimes := []uint64{
		22400432,
		116499979,
	}
	memoryUtilizationInBytes := []uint64{
		3649536,
		3649536,
	}

	bytesReceivedTransmitted := []uint64{
		364953689,
		364953689,
	}

	queue := NewQueue(3)

	for i, time := range timestamps {
		queue.add(&ContainerStats{
			cpuUsage:    cpuTimes[i],
			memoryUsage: memoryUtilizationInBytes[i],
			networkStats: &NetworkStats{
				RxBytes:          bytesReceivedTransmitted[i],
				RxDropped:        0,
				RxErrors:         bytesReceivedTransmitted[i],
				RxPackets:        bytesReceivedTransmitted[i],
				TxBytes:          bytesReceivedTransmitted[i],
				TxDropped:        bytesReceivedTransmitted[i],
				TxErrors:         0,
				TxPackets:        bytesReceivedTransmitted[i],
				RxBytesPerSecond: float32(nan32()),
				TxBytesPerSecond: float32(nan32()),
			},
			timestamp: time})
	}

	// if we have identical timestamps and 2 datapoints
	// then there will not be enough valid network stats  to create
	// a valid network stats set, and this function call should fail.
	stats, err := queue.GetNetworkStatsSet()
	require.Errorf(t, err, "Received unexpected network stats set %v", stats)
}

// If there are only 3 datapoints in total and among them 2 are identical, then GetNetworkStats should not return error
func TestPerSecNetworkStatSetPassWithThreeDatapoints(t *testing.T) {
	timestamps := []time.Time{
		parseNanoTime("2015-02-12T21:22:05.131117533Z"),
		parseNanoTime("2015-02-12T21:22:05.131117533Z"),
		parseNanoTime("2015-02-12T21:32:05.131117533Z"),
	}
	cpuTimes := []uint64{
		22400432,
		116499979,
		115436856,
	}
	memoryUtilizationInBytes := []uint64{
		3649536,
		3649536,
		3649536,
	}

	bytesReceivedTransmitted := []uint64{
		364953689,
		364953689,
		364953689,
	}

	queue := NewQueue(3)

	for i, time := range timestamps {
		queue.add(&ContainerStats{
			cpuUsage:    cpuTimes[i],
			memoryUsage: memoryUtilizationInBytes[i],
			networkStats: &NetworkStats{
				RxBytes:          bytesReceivedTransmitted[i],
				RxDropped:        0,
				RxErrors:         bytesReceivedTransmitted[i],
				RxPackets:        bytesReceivedTransmitted[i],
				TxBytes:          bytesReceivedTransmitted[i],
				TxDropped:        bytesReceivedTransmitted[i],
				TxErrors:         0,
				TxPackets:        bytesReceivedTransmitted[i],
				RxBytesPerSecond: float32(nan32()),
				TxBytesPerSecond: float32(nan32()),
			},
			timestamp: time})
	}

	// if we have identical timestamps and 2 datapoints
	// then there will not be enough valid network stats  to create
	// a valid network stats set, and this function call should fail.
	stats, err := queue.GetNetworkStatsSet()
	require.NoErrorf(t, err, "Received unexpected network stats set %v", stats)
}

// If there are only 2 datapoints, and both have different timestamp, GetNetworkStats should not return error
func TestPerSecNetworkStatSetPassWithTwoDatapoints(t *testing.T) {
	timestamps := []time.Time{
		parseNanoTime("2015-02-12T21:22:05.131117533Z"),
		parseNanoTime("2015-02-12T21:32:05.131117533Z"),
	}
	cpuTimes := []uint64{
		22400432,
		116499979,
	}
	memoryUtilizationInBytes := []uint64{
		3649536,
		3649536,
	}

	bytesReceivedTransmitted := []uint64{
		364953689,
		364953689,
	}

	queue := NewQueue(3)

	for i, time := range timestamps {
		queue.add(&ContainerStats{
			cpuUsage:    cpuTimes[i],
			memoryUsage: memoryUtilizationInBytes[i],
			networkStats: &NetworkStats{
				RxBytes:          bytesReceivedTransmitted[i],
				RxDropped:        0,
				RxErrors:         bytesReceivedTransmitted[i],
				RxPackets:        bytesReceivedTransmitted[i],
				TxBytes:          bytesReceivedTransmitted[i],
				TxDropped:        bytesReceivedTransmitted[i],
				TxErrors:         0,
				TxPackets:        bytesReceivedTransmitted[i],
				RxBytesPerSecond: float32(nan32()),
				TxBytesPerSecond: float32(nan32()),
			},
			timestamp: time})
	}

	// if we have identical timestamps and 2 datapoints
	// then there will not be enough valid network stats  to create
	// a valid network stats set, and this function call should fail.
	stats, err := queue.GetNetworkStatsSet()
	require.NoErrorf(t, err, "Received unexpected network stats set %v", stats)
}

// If there are only 1 datapoint, then GetNetworkStats should return error
func TestPerSecNetworkStatSetFailWithOneDatapoint(t *testing.T) {
	timestamps := []time.Time{
		parseNanoTime("2015-02-12T21:22:05.131117533Z"),
	}

	cpuTimes := []uint64{
		22400432,
	}
	memoryUtilizationInBytes := []uint64{
		3649536,
	}

	bytesReceivedTransmitted := []uint64{
		364953689,
	}

	queue := NewQueue(3)

	for i, time := range timestamps {
		queue.add(&ContainerStats{
			cpuUsage:    cpuTimes[i],
			memoryUsage: memoryUtilizationInBytes[i],
			networkStats: &NetworkStats{
				RxBytes:          bytesReceivedTransmitted[i],
				RxDropped:        0,
				RxErrors:         bytesReceivedTransmitted[i],
				RxPackets:        bytesReceivedTransmitted[i],
				TxBytes:          bytesReceivedTransmitted[i],
				TxDropped:        bytesReceivedTransmitted[i],
				TxErrors:         0,
				TxPackets:        bytesReceivedTransmitted[i],
				RxBytesPerSecond: float32(nan32()),
				TxBytesPerSecond: float32(nan32()),
			},
			timestamp: time})
	}

	// if we have identical timestamps and 2 datapoints
	// then there will not be enough valid network stats  to create
	// a valid network stats set, and this function call should fail.
	stats, err := queue.GetNetworkStatsSet()
	require.Errorf(t, err, "Received unexpected network stats set %v", stats)
}

func TestAggregateOSIndependentStats(t *testing.T) {
	dockerStat := getTestStatsJSONForOSIndependentStats(1, 2, 3, 4, 5, 6, 7, 8, 9)
	lastStatBeforeLastRestart := getTestStatsJSONForOSIndependentStats(9, 8, 7, 6, 5, 4, 3, 2, 1)
	expectedAggregatedStat := types.StatsJSON{
		Stats: types.Stats{
			CPUStats: types.CPUStats{
				CPUUsage: types.CPUUsage{
					TotalUsage: dockerStat.CPUStats.CPUUsage.TotalUsage +
						lastStatBeforeLastRestart.CPUStats.CPUUsage.TotalUsage,
					UsageInKernelmode: dockerStat.CPUStats.CPUUsage.UsageInKernelmode +
						lastStatBeforeLastRestart.CPUStats.CPUUsage.UsageInKernelmode,
					UsageInUsermode: dockerStat.CPUStats.CPUUsage.UsageInUsermode +
						lastStatBeforeLastRestart.CPUStats.CPUUsage.UsageInUsermode,
				},
			},
		},
		Networks: map[string]types.NetworkStats{
			testNetworkNameA: {
				RxBytes: dockerStat.Networks[testNetworkNameA].RxBytes +
					lastStatBeforeLastRestart.Networks[testNetworkNameA].RxBytes,
				RxPackets: dockerStat.Networks[testNetworkNameA].RxPackets +
					lastStatBeforeLastRestart.Networks[testNetworkNameA].RxPackets,
				RxDropped: dockerStat.Networks[testNetworkNameA].RxDropped +
					lastStatBeforeLastRestart.Networks[testNetworkNameA].RxDropped,
				TxBytes: dockerStat.Networks[testNetworkNameA].TxBytes +
					lastStatBeforeLastRestart.Networks[testNetworkNameA].TxBytes,
				TxPackets: dockerStat.Networks[testNetworkNameA].TxPackets +
					lastStatBeforeLastRestart.Networks[testNetworkNameA].TxPackets,
				TxDropped: dockerStat.Networks[testNetworkNameA].TxDropped +
					lastStatBeforeLastRestart.Networks[testNetworkNameA].TxDropped,
			},
			testNetworkNameB: {
				RxBytes: dockerStat.Networks[testNetworkNameB].RxBytes +
					lastStatBeforeLastRestart.Networks[testNetworkNameB].RxBytes,
				RxPackets: dockerStat.Networks[testNetworkNameB].RxPackets +
					lastStatBeforeLastRestart.Networks[testNetworkNameB].RxPackets,
				RxDropped: dockerStat.Networks[testNetworkNameB].RxDropped +
					lastStatBeforeLastRestart.Networks[testNetworkNameB].RxDropped,
				TxBytes: dockerStat.Networks[testNetworkNameB].TxBytes +
					lastStatBeforeLastRestart.Networks[testNetworkNameB].TxBytes,
				TxPackets: dockerStat.Networks[testNetworkNameB].TxPackets +
					lastStatBeforeLastRestart.Networks[testNetworkNameB].TxPackets,
				TxDropped: dockerStat.Networks[testNetworkNameB].TxDropped +
					lastStatBeforeLastRestart.Networks[testNetworkNameB].TxDropped,
			},
		},
	}

	dockerStat = aggregateOSIndependentStats(dockerStat, lastStatBeforeLastRestart)
	require.Equal(t, expectedAggregatedStat, *dockerStat)
}

func TestGetAggregatedDockerStatAcrossRestarts(t *testing.T) {
	var dockerStat, lastStatBeforeLastRestart, lastStatInStatsQueue types.StatsJSON
	lastStatInStatsQueue.Stats.CPUStats.CPUUsage.TotalUsage = uint64(123)

	dockerStat = *getAggregatedDockerStatAcrossRestarts(&dockerStat, &lastStatBeforeLastRestart, &lastStatInStatsQueue)
	require.Equal(t, lastStatInStatsQueue.Stats.CPUStats.CPUUsage.TotalUsage,
		dockerStat.PreCPUStats.CPUUsage.TotalUsage)
}

func getTestStatsJSONForOSIndependentStats(totalCPUUsage, usageInKernelMode, usageInUserMode, rxBytes, rxPackets,
	rxDropped, txBytes, txPackets, txDropped uint64) *types.StatsJSON {
	return &types.StatsJSON{
		Stats: types.Stats{
			CPUStats: types.CPUStats{
				CPUUsage: types.CPUUsage{
					TotalUsage:        totalCPUUsage,
					UsageInKernelmode: usageInKernelMode,
					UsageInUsermode:   usageInUserMode,
				},
			},
		},
		Networks: map[string]types.NetworkStats{
			testNetworkNameA: {
				RxBytes:   rxBytes,
				RxPackets: rxPackets,
				RxDropped: rxDropped,
				TxBytes:   txBytes,
				TxPackets: txPackets,
				TxDropped: txDropped,
			},
			testNetworkNameB: {
				RxBytes:   rxBytes + 1,
				RxPackets: rxPackets + 1,
				RxDropped: rxDropped + 1,
				TxBytes:   txBytes + 1,
				TxPackets: txPackets + 1,
				TxDropped: txDropped + 1,
			},
		},
	}
}

func TestQueueAdd(t *testing.T) {
	testCases := []struct {
		name                string
		includeRestartCount bool
	}{
		{
			name:                "restart count not included",
			includeRestartCount: false,
		},
		{
			name:                "restart count included",
			includeRestartCount: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			queueMaxSize := 3
			queue := NewQueue(queueMaxSize)

			for i := 0; i < queueMaxSize; i++ {
				dockerStat := getTestStatsJSONForOSIndependentStats(rand.Uint64(), rand.Uint64(), rand.Uint64(),
					rand.Uint64(), rand.Uint64(), rand.Uint64(), rand.Uint64(), rand.Uint64(), rand.Uint64())
				nonDockerContainerStats := NonDockerContainerStats{}
				if tc.includeRestartCount {
					restartCount := int64(i)
					nonDockerContainerStats.restartCount = &restartCount
				}
				queue.Add(dockerStat, nonDockerContainerStats)
				require.Equal(t, dockerStat, queue.GetLastStat())
			}
			restartStats, err := queue.GetRestartStatsSet()
			if tc.includeRestartCount {
				require.NoError(t, err)
				require.NotNil(t, restartStats)
				require.NotNil(t, restartStats.RestartCount)
				require.Equal(t, int64(queueMaxSize-1), *restartStats.RestartCount)
			} else {
				require.Error(t, err)
				require.Nil(t, restartStats)
			}
		})
	}
}

func TestQueueAddContainerStat(t *testing.T) {
	testCases := []struct {
		name                        string
		containerHasRestartedBefore bool
		lastStatBeforeLastRestart   types.StatsJSON
	}{
		{
			name:                        "container has not restarted before",
			containerHasRestartedBefore: false,
		},
		{
			name:                        "container has restarted before",
			containerHasRestartedBefore: true,
			lastStatBeforeLastRestart:   *getTestStatsJSONForOSIndependentStats(9, 8, 7, 6, 5, 4, 3, 2, 1),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up queue.
			queueMaxSize := 3
			queue := NewQueue(queueMaxSize)
			restartCount := int64(1)
			nonDockerContainerStats := NonDockerContainerStats{restartCount: &restartCount}
			firstStat := getTestStatsJSONForOSIndependentStats(rand.Uint64(), rand.Uint64(), rand.Uint64(),
				rand.Uint64(), rand.Uint64(), rand.Uint64(), rand.Uint64(), rand.Uint64(), rand.Uint64())
			queue.Add(firstStat, nonDockerContainerStats)
			require.Equal(t, firstStat, queue.GetLastStat())

			// Validate queue.AddContainerStat behavior.
			dockerStat := getTestStatsJSONForOSIndependentStats(1, 2, 3, 4, 5, 6, 7, 8, 9)
			queue.AddContainerStat(dockerStat, nonDockerContainerStats, &tc.lastStatBeforeLastRestart,
				tc.containerHasRestartedBefore)
			if tc.containerHasRestartedBefore {
				require.Equal(t, getAggregatedDockerStatAcrossRestarts(dockerStat, &tc.lastStatBeforeLastRestart,
					firstStat), queue.GetLastStat())
			} else {
				require.Equal(t, dockerStat, queue.GetLastStat())
			}
		})
	}
}
