//+build unit

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
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/aws/aws-sdk-go/aws"
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

func getTimestamps() []time.Time {
	return []time.Time{
		parseNanoTime("2015-02-12T21:22:05.131117533Z"),
		parseNanoTime("2015-02-12T21:22:05.232291187Z"),
		parseNanoTime("2015-02-12T21:22:05.333776335Z"),
		parseNanoTime("2015-02-12T21:22:05.434753595Z"),
		parseNanoTime("2015-02-12T21:22:05.535746779Z"),
		parseNanoTime("2015-02-12T21:22:05.638709495Z"),
		parseNanoTime("2015-02-12T21:22:05.739985398Z"),
		parseNanoTime("2015-02-12T21:22:05.840941705Z"),
		parseNanoTime("2015-02-12T21:22:05.94164351Z"),
		parseNanoTime("2015-02-12T21:22:06.042625519Z"),
		parseNanoTime("2015-02-12T21:22:06.143665077Z"),
		parseNanoTime("2015-02-12T21:22:06.244769169Z"),
		parseNanoTime("2015-02-12T21:22:06.345847001Z"),
		parseNanoTime("2015-02-12T21:22:06.447151399Z"),
		parseNanoTime("2015-02-12T21:22:06.548213586Z"),
		parseNanoTime("2015-02-12T21:22:06.650013301Z"),
		parseNanoTime("2015-02-12T21:22:06.751120187Z"),
		parseNanoTime("2015-02-12T21:22:06.852163377Z"),
		parseNanoTime("2015-02-12T21:22:06.952980001Z"),
		parseNanoTime("2015-02-12T21:22:07.054047217Z"),
		parseNanoTime("2015-02-12T21:22:07.154840095Z"),
		parseNanoTime("2015-02-12T21:22:07.256075769Z"),
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

func createQueue(size int, predictableHighUtilization bool) *Queue {
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
	queue := NewQueue(size)
	for i, time := range timestamps {
		queue.add(&ContainerStats{
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
			timestamp: time})
	}
	return queue
}

func TestQueueAddRemove(t *testing.T) {
	timestamps := getTimestamps()
	queueLength := 5
	// Set predictableHighUtilization to false, expect random values when aggregated.
	queue := createQueue(queueLength, false)
	buf := queue.buffer
	if len(buf) != queueLength {
		t.Error("Buffer size is incorrect. Expected: 4, Got: ", len(buf))
	}

	timestampsIndex := len(timestamps) - len(buf)
	for i, stat := range buf {
		if stat.Timestamp != timestamps[timestampsIndex+i] {
			t.Error("Unexpected value for Stats element in buffer")
		}
	}

	cpuStatsSet, err := queue.GetCPUStatsSet()
	if err != nil {
		t.Error("Error getting cpu stats set:", err)
	}
	if *cpuStatsSet.Min == math.MaxFloat64 || math.IsNaN(*cpuStatsSet.Min) {
		t.Error("Min value incorrectly set: ", *cpuStatsSet.Min)
	}
	if *cpuStatsSet.Max == -math.MaxFloat64 || math.IsNaN(*cpuStatsSet.Max) {
		t.Error("Max value incorrectly set: ", *cpuStatsSet.Max)
	}
	if *cpuStatsSet.SampleCount != int64(queueLength) {
		t.Error("Expected samplecount: ", queueLength, " got: ", *cpuStatsSet.SampleCount)
	}
	if *cpuStatsSet.Sum == 0 {
		t.Error("Sum value incorrectly set: ", *cpuStatsSet.Sum)
	}

	memStatsSet, err := queue.GetMemoryStatsSet()
	if err != nil {
		t.Error("Error getting memory stats set:", err)
	}
	if *memStatsSet.Min == math.MaxFloat64 || math.IsNaN(*memStatsSet.Min) {
		t.Error("Min value incorrectly set: ", *memStatsSet.Min)
	}
	if *memStatsSet.Max == -math.MaxFloat64 || math.IsNaN(*memStatsSet.Max) {
		t.Error("Max value incorrectly set: ", *memStatsSet.Max)
	}
	if *memStatsSet.SampleCount != int64(queueLength) {
		t.Error("Expected samplecount: ", queueLength, " got: ", *memStatsSet.SampleCount)
	}
	if *memStatsSet.Sum == 0 {
		t.Error("Sum value incorrectly set: ", *memStatsSet.Sum)
	}

	storageStatsSet, err := queue.GetStorageStatsSet()
	if err != nil {
		t.Error("Error getting storage stats set:", err)
	}
	// assuming min is initialized to math.MaxUint64 then truncated
	storageReadStatsSet := storageStatsSet.ReadSizeBytes
	if *storageReadStatsSet.Min == int64(math.MaxInt64) &&
		*storageReadStatsSet.OverflowMin == int64(math.MaxInt64) {
		t.Error("Min value incorrectly set: ", *storageReadStatsSet.Min)
	}
	if *storageReadStatsSet.Max == 0 {
		t.Error("Max value incorrectly set: ", *storageReadStatsSet.Max)
	}
	if *storageReadStatsSet.SampleCount != int64(queueLength) {
		t.Error("Expected samplecount: ", queueLength, " got: ", *storageReadStatsSet.SampleCount)
	}
	if *storageReadStatsSet.Sum == 0 {
		t.Error("Sum value incorrectly set: ", *storageReadStatsSet.Sum)
	}

	storageWriteStatsSet := storageStatsSet.WriteSizeBytes
	if *storageWriteStatsSet.Min == int64(math.MaxInt64) {
		t.Error("Min value incorrectly set: ", *storageWriteStatsSet.Min)
	}
	if *storageWriteStatsSet.Max == 0 {
		t.Error("Max value incorrectly set: ", *storageWriteStatsSet.Max)
	}
	if *storageWriteStatsSet.SampleCount != int64(queueLength) {
		t.Error("Expected samplecount: ", queueLength, " got: ", *storageWriteStatsSet.SampleCount)
	}
	if *storageWriteStatsSet.Sum == 0 {
		t.Error("Sum value incorrectly set: ", *storageWriteStatsSet.Sum)
	}

	netStatsSet, err := queue.GetNetworkStatsSet()
	assert.NoError(t, err, "error getting network stats set")
	validateNetStatsSet(t, netStatsSet, queueLength)

	rawUsageStats, err := queue.GetRawUsageStats(2 * queueLength)
	if err != nil {
		t.Error("Error getting raw usage stats: ", err)
	}

	if len(rawUsageStats) != queueLength {
		t.Error("Expected to get ", queueLength, " raw usage stats. Got: ", len(rawUsageStats))
	}

	prevRawUsageStats := rawUsageStats[0]
	for i := 1; i < queueLength; i++ {
		curRawUsageStats := rawUsageStats[i]
		if prevRawUsageStats.Timestamp.Before(curRawUsageStats.Timestamp) {
			t.Error("Raw usage stats not ordered as expected, index: ", i, " prev stat: ", prevRawUsageStats.Timestamp, " current stat: ", curRawUsageStats.Timestamp)
		}
		prevRawUsageStats = curRawUsageStats
	}

	emptyQueue := NewQueue(queueLength)
	rawUsageStats, err = emptyQueue.GetRawUsageStats(1)
	if err == nil {
		t.Error("Empty queue query did not throw an error")
	}

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
	if len(buf) != queueLength {
		t.Error("Buffer size is incorrect. Expected: 4, Got: ", len(buf))
	}

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
	if *memStatsSet.Min != expectedMemoryUsageInMiB {
		t.Errorf("Min value incorrectly set: %.0f, expected: %.0f", *memStatsSet.Min, expectedMemoryUsageInMiB)
	}
	if *memStatsSet.Max != expectedMemoryUsageInMiB {
		t.Errorf("Max value incorrectly set: %.0f, expected: %.0f", *memStatsSet.Max, expectedMemoryUsageInMiB)
	}
	if *memStatsSet.SampleCount != int64(queueLength) {
		t.Errorf("Incorrect samplecount, expected: %d got: %d", queueLength, *memStatsSet.SampleCount)
	}

	expectedMemoryUsageInMiBSum := expectedMemoryUsageInMiB * float64(queueLength)
	if *memStatsSet.Sum != expectedMemoryUsageInMiBSum {
		t.Errorf("Sum value incorrectly set: %.0f, expected %.0f", *memStatsSet.Sum, expectedMemoryUsageInMiBSum)
	}
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
	timestamps := []time.Time{
		parseNanoTime("2015-02-12T21:22:05.131117533Z"),
		parseNanoTime("2015-02-12T21:22:05.131117533Z"),
		parseNanoTime("2015-02-12T21:22:05.333776335Z"),
	}
	cpuTimes := []uint64{
		22400432,
		116499979,
		248503503,
	}
	memoryUtilizationInBytes := []uint64{
		3649536,
		3649536,
		3649536,
	}

	// Create and add container stats
	queueLength := 3
	queue := NewQueue(queueLength)
	for i, time := range timestamps {
		queue.add(&ContainerStats{cpuUsage: cpuTimes[i], memoryUsage: memoryUtilizationInBytes[i], timestamp: time})
	}
	cpuStatsSet, err := queue.GetCPUStatsSet()
	if err != nil {
		t.Errorf("Error getting cpu stats set: %v", err)
	}

	// Compute expected usage by using the 1st and 2nd data point in the input queue
	// queue.Add should have ignored the 0th item as it has the same timestamp as the
	// 1st item
	expectedCpuUsage := 100 * float32(cpuTimes[2]-cpuTimes[1]) / float32(timestamps[2].Nanosecond()-timestamps[1].Nanosecond())
	max := float32(aws.Float64Value(cpuStatsSet.Max))
	if max != expectedCpuUsage {
		t.Errorf("Computed cpuStatsSet.Max (%f) != expected value (%f)", max, expectedCpuUsage)
	}
	sum := float32(aws.Float64Value(cpuStatsSet.Sum))
	if sum != expectedCpuUsage {
		t.Errorf("Computed cpuStatsSet.Sum (%f) != expected value (%f)", sum, expectedCpuUsage)
	}
	min := float32(aws.Float64Value(cpuStatsSet.Min))
	if min != expectedCpuUsage {
		t.Errorf("Computed cpuStatsSet.Min (%f) != expected value (%f)", min, expectedCpuUsage)
	}

	// Expected sample count is 1 and not 2 as one data point would be discarded on
	// account of invalid timestamp
	sampleCount := aws.Int64Value(cpuStatsSet.SampleCount)
	if sampleCount != 1 {
		t.Errorf("Computed cpuStatsSet.SampleCount (%d) != expected value (%d)", sampleCount, 1)
	}
}

func TestResetThresholdElapsed(t *testing.T) {
	// create a queue
	queueLength := 3
	queue := NewQueue(queueLength)

	queue.Reset()

	thresholdElapsed := queue.resetThresholdElapsed(2 * time.Millisecond)
	assert.False(t, thresholdElapsed, "Queue reset threshold is not expected to elapse right after reset")

	time.Sleep(3 * time.Millisecond)
	thresholdElapsed = queue.resetThresholdElapsed(2 * time.Millisecond)

	assert.True(t, thresholdElapsed, "Queue reset threshold is expected to elapse after waiting")
}

func TestEnoughDatapointsInBuffer(t *testing.T) {
	// timestamps will be used to simulate +Inf CPU Usage
	// timestamps[0] = timestamps[1]
	timestamps := []time.Time{
		parseNanoTime("2015-02-12T21:22:05.131117533Z"),
		parseNanoTime("2015-02-12T21:22:05.131117533Z"),
		parseNanoTime("2015-02-12T21:22:05.333776335Z"),
	}
	cpuTimes := []uint64{
		22400432,
		116499979,
		248503503,
	}
	memoryUtilizationInBytes := []uint64{
		3649536,
		3649536,
		3649536,
	}
	// create a queue
	queueLength := 3
	queue := NewQueue(queueLength)

	enoughDataPoints := queue.enoughDatapointsInBuffer()
	assert.False(t, enoughDataPoints, "Queue is expected to not have enough data points right after creation")
	for i, time := range timestamps {
		queue.add(&ContainerStats{cpuUsage: cpuTimes[i], memoryUsage: memoryUtilizationInBytes[i], timestamp: time})
	}

	enoughDataPoints = queue.enoughDatapointsInBuffer()
	assert.True(t, enoughDataPoints, "Queue is expected to have enough data points when it has more than 2 msgs queued")

	queue.Reset()
	enoughDataPoints = queue.enoughDatapointsInBuffer()
	assert.False(t, enoughDataPoints, "Queue is expected to not have enough data points right after RESET")
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
	timestamps := []time.Time{
		parseNanoTime("2015-02-12T21:22:05.131117000Z"),
		parseNanoTime("2015-02-12T21:22:05.131117001Z"),
		parseNanoTime("2015-02-12T21:22:05.131117002Z"),
		parseNanoTime("2015-02-12T21:22:05.131117002Z"),
	}
	cpuTimes := []uint64{
		0,
		1,
		3,
		4,
	}
	memoryUtilizationInBytes := []uint64{
		3649536,
		3649536,
		3649536,
		3649536,
	}
	// create a queue
	queue := NewQueue(4)

	for i, time := range timestamps {
		queue.add(&ContainerStats{cpuUsage: cpuTimes[i], memoryUsage: memoryUtilizationInBytes[i], timestamp: time})
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
	timestamps := []time.Time{
		parseNanoTime("2015-02-12T21:22:05.131117000Z"),
		parseNanoTime("2015-02-12T21:22:05.131117001Z"),
		parseNanoTime("2015-02-12T21:22:05.131117002Z"),
	}
	cpuTimes := []uint64{
		0,
		1,
		300000000,
	}
	memoryUtilizationInBytes := []uint64{
		3649536,
		3649536,
		3649536,
	}
	// create a queue
	queue := NewQueue(4)

	for i, time := range timestamps {
		queue.add(&ContainerStats{cpuUsage: cpuTimes[i], memoryUsage: memoryUtilizationInBytes[i], timestamp: time})
	}

	statSet, err := queue.GetCPUStatsSet()
	require.NoError(t, err)
	require.Equal(t, float64(30000001024), *statSet.Max)
	require.Equal(t, float64(100), *statSet.Min)
	require.Equal(t, int64(2), *statSet.SampleCount)
	require.Equal(t, float64(30000001124), *statSet.Sum)
}
