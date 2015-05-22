// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
)

func getTimestamps() []time.Time {
	return []time.Time{
		ParseNanoTime("2015-02-12T21:22:05.131117533Z"),
		ParseNanoTime("2015-02-12T21:22:05.232291187Z"),
		ParseNanoTime("2015-02-12T21:22:05.333776335Z"),
		ParseNanoTime("2015-02-12T21:22:05.434753595Z"),
		ParseNanoTime("2015-02-12T21:22:05.535746779Z"),
		ParseNanoTime("2015-02-12T21:22:05.638709495Z"),
		ParseNanoTime("2015-02-12T21:22:05.739985398Z"),
		ParseNanoTime("2015-02-12T21:22:05.840941705Z"),
		ParseNanoTime("2015-02-12T21:22:05.94164351Z"),
		ParseNanoTime("2015-02-12T21:22:06.042625519Z"),
		ParseNanoTime("2015-02-12T21:22:06.143665077Z"),
		ParseNanoTime("2015-02-12T21:22:06.244769169Z"),
		ParseNanoTime("2015-02-12T21:22:06.345847001Z"),
		ParseNanoTime("2015-02-12T21:22:06.447151399Z"),
		ParseNanoTime("2015-02-12T21:22:06.548213586Z"),
		ParseNanoTime("2015-02-12T21:22:06.650013301Z"),
		ParseNanoTime("2015-02-12T21:22:06.751120187Z"),
		ParseNanoTime("2015-02-12T21:22:06.852163377Z"),
		ParseNanoTime("2015-02-12T21:22:06.952980001Z"),
		ParseNanoTime("2015-02-12T21:22:07.054047217Z"),
		ParseNanoTime("2015-02-12T21:22:07.154840095Z"),
		ParseNanoTime("2015-02-12T21:22:07.256075769Z"),
	}

}

func getCPUTimes() []uint64 {
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

func getMemBytes() []uint64 {
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

func createQueue(size int) *Queue {
	timestamps := getTimestamps()
	cpuTimes := getCPUTimes()
	memBytes := getMemBytes()
	queue := NewQueue(size)
	for i, time := range timestamps {
		queue.Add(&ContainerStats{cpuUsage: cpuTimes[i], memoryUsage: memBytes[i], timestamp: time})
	}
	return queue
}

func TestQueueAddRemove(t *testing.T) {
	timestamps := getTimestamps()
	queueLength := 5
	queue := createQueue(queueLength)
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
		t.Error("Error gettting cpu stats set:", err)
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
		t.Error("Error gettting cpu stats set:", err)
	}
	if *memStatsSet.Min == float64(-math.MaxFloat32) {
		t.Error("Min value incorrectly set: ", *memStatsSet.Min)
	}
	if *memStatsSet.Max == 0 {
		t.Error("Max value incorrectly set: ", *memStatsSet.Max)
	}
	if *memStatsSet.SampleCount != int64(queueLength) {
		t.Error("Expected samplecount: ", queueLength, " got: ", *memStatsSet.SampleCount)
	}
	if *memStatsSet.Sum == 0 {
		t.Error("Sum value incorrectly set: ", *memStatsSet.Sum)
	}

	rawUsageStats, err := queue.GetRawUsageStats(2 * queueLength)
	if err != nil {
		t.Error("Error gettting raw usage stats: ", err)
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
