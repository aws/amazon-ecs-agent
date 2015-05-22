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

// checkPointSleep is the sleep duration in milliseconds between
// starting/stopping containers in the test code.
const checkPointSleep = 300 * time.Millisecond

type MockStatsCollector struct {
	index int
	stats []ContainerStats
}

func newMockStatsCollector() *MockStatsCollector {
	collector := &MockStatsCollector{index: 0}
	timestamps := []time.Time{
		ParseNanoTime("2015-02-12T21:22:05.131117533Z"),
		ParseNanoTime("2015-02-12T21:22:05.232291187Z"),
		ParseNanoTime("2015-02-12T21:22:05.333776335Z"),
		ParseNanoTime("2015-02-12T21:22:05.434753595Z"),
		ParseNanoTime("2015-02-12T21:22:05.535746779Z"),
		ParseNanoTime("2015-02-12T21:22:05.638709495Z"),
		ParseNanoTime("2015-02-12T21:22:05.739985398Z"),
		ParseNanoTime("2015-02-12T21:22:05.840941705Z"),
	}
	cpuTimes := []uint64{
		22400432,
		116499979,
		248503503,
		372167097,
		502862518,
		638485801,
		780707806,
		911624529,
	}
	memBytes := []uint64{
		1839104,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
		3649536,
	}
	collector.stats = make([]ContainerStats, len(timestamps))
	for i := range timestamps {
		collector.stats[i] = *CreateContainerStats(cpuTimes[i], memBytes[i], timestamps[i])
	}
	return collector
}

func (collector *MockStatsCollector) getContainerStats(container *CronContainer) (*ContainerStats, error) {
	cs := collector.stats[collector.index]
	collector.index++
	return &cs, nil
}

func TestContainerStatsAggregation(t *testing.T) {
	var container *CronContainer
	dockerID := "container1"
	name := "docker-container1"
	container = &CronContainer{
		containerMetadata: &ContainerMetadata{
			DockerID: &dockerID,
			Name:     &name,
		},
	}
	container.statsCollector = newMockStatsCollector()
	container.StartStatsCron()
	time.Sleep(checkPointSleep)
	container.StopStatsCron()
	cpuStatsSet, err := container.statsQueue.GetCPUStatsSet()
	if err != nil {
		t.Error("Error gettting cpu stats set:", err)
	}
	if *cpuStatsSet.Min == math.MaxFloat64 || math.IsNaN(*cpuStatsSet.Min) {
		t.Error("Min value incorrectly set: ", *cpuStatsSet.Min)
	}
	if *cpuStatsSet.Max == -math.MaxFloat64 || math.IsNaN(*cpuStatsSet.Max) {
		t.Error("Max value incorrectly set: ", *cpuStatsSet.Max)
	}
	if *cpuStatsSet.SampleCount == 0 {
		t.Error("Samplecount is 0")
	}
	if *cpuStatsSet.Sum == 0 {
		t.Error("Sum value incorrectly set: ", *cpuStatsSet.Sum)
	}

	memStatsSet, err := container.statsQueue.GetMemoryStatsSet()
	if err != nil {
		t.Error("Error gettting cpu stats set:", err)
	}
	if *memStatsSet.Min == math.MaxFloat64 {
		t.Error("Min value incorrectly set: ", *memStatsSet.Min)
	}
	if *memStatsSet.Max == 0 {
		t.Error("Max value incorrectly set: ", *memStatsSet.Max)
	}
	if *memStatsSet.SampleCount == 0 {
		t.Error("Samplecount is 0")
	}
	if *memStatsSet.Sum == 0 {
		t.Error("Sum value incorrectly set: ", *memStatsSet.Sum)
	}
}
