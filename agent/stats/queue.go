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
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
)

const (
	// BytesInMiB is the number of bytes in a MebiByte.
	BytesInMiB                     = 1024 * 1024
	minimumQueueDatapoints         = 2
	MaxCPUUsagePerc        float32 = 1024 * 1024
)

// Queue abstracts a queue using UsageStats slice.
type Queue struct {
	buffer        []UsageStats
	maxSize       int
	lastResetTime time.Time
	lastStat      *types.StatsJSON
	lock          sync.RWMutex
}

// NewQueue creates a queue.
func NewQueue(maxSize int) *Queue {
	return &Queue{
		buffer:  make([]UsageStats, 0, maxSize),
		maxSize: maxSize,
	}
}

// Reset resets the stats queue.
func (queue *Queue) Reset() {
	queue.lock.Lock()
	defer queue.lock.Unlock()
	queue.lastResetTime = time.Now()
	queue.buffer = queue.buffer[:0]
}

// Add adds a new set of container stats to the queue.
func (queue *Queue) Add(dockerStat *types.StatsJSON) error {
	queue.setLastStat(dockerStat)
	stat, err := dockerStatsToContainerStats(dockerStat)
	if err != nil {
		return err
	}
	queue.add(stat)
	return nil
}

func (queue *Queue) setLastStat(stat *types.StatsJSON) {
	queue.lock.Lock()
	defer queue.lock.Unlock()

	queue.lastStat = stat
}

func (queue *Queue) add(rawStat *ContainerStats) {
	queue.lock.Lock()
	defer queue.lock.Unlock()

	queueLength := len(queue.buffer)
	stat := UsageStats{
		CPUUsagePerc:      float32(nan32()),
		MemoryUsageInMegs: uint32(rawStat.memoryUsage / BytesInMiB),
		StorageReadBytes:  rawStat.storageReadBytes,
		StorageWriteBytes: rawStat.storageWriteBytes,
		NetworkStats:      rawStat.networkStats,
		Timestamp:         rawStat.timestamp,
		cpuUsage:          rawStat.cpuUsage,
	}
	if queueLength != 0 {
		// % utilization can be calculated only when queue is non-empty.
		lastStat := queue.buffer[queueLength-1]
		timeSinceLastStat := float32(rawStat.timestamp.Sub(lastStat.Timestamp).Nanoseconds())
		if timeSinceLastStat <= 0 {
			// if we got a duplicate timestamp, set cpu percentage to the same value as the previous stat
			seelog.Errorf("Received a docker stat object with duplicate timestamp")
			stat.CPUUsagePerc = lastStat.CPUUsagePerc
		} else {
			cpuUsageSinceLastStat := float32(rawStat.cpuUsage - lastStat.cpuUsage)
			stat.CPUUsagePerc = 100 * cpuUsageSinceLastStat / timeSinceLastStat
		}

		if queue.maxSize == queueLength {
			// Remove first element if queue is full.
			queue.buffer = queue.buffer[1:queueLength]
		}

		if stat.CPUUsagePerc > MaxCPUUsagePerc {
			// what in the world happened
			seelog.Errorf("Calculated CPU usage percent (%.1f) is larger than backend maximum (%.1f). lastStatTS=%s lastStatCPUTime=%d thisStatTS=%s thisStatCPUTime=%d queueLength=%d",
				stat.CPUUsagePerc, MaxCPUUsagePerc, lastStat.Timestamp.Format(time.RFC3339Nano), lastStat.cpuUsage, rawStat.timestamp.Format(time.RFC3339Nano), rawStat.cpuUsage, queueLength)
		}
	}

	queue.buffer = append(queue.buffer, stat)
}

// GetLastStat returns the last recorded raw statistics object from docker
func (queue *Queue) GetLastStat() *types.StatsJSON {
	queue.lock.RLock()
	defer queue.lock.RUnlock()

	return queue.lastStat
}

// GetCPUStatsSet gets the stats set for CPU utilization.
func (queue *Queue) GetCPUStatsSet() (*ecstcs.CWStatsSet, error) {
	return queue.getCWStatsSet(getCPUUsagePerc)
}

// GetMemoryStatsSet gets the stats set for memory utilization.
func (queue *Queue) GetMemoryStatsSet() (*ecstcs.CWStatsSet, error) {
	return queue.getCWStatsSet(getMemoryUsagePerc)
}

// GetStorageStatsSet gets the stats set for aggregate storage
func (queue *Queue) GetStorageStatsSet() (*ecstcs.StorageStatsSet, error) {
	storageStatsSet := &ecstcs.StorageStatsSet{}
	var err error
	storageStatsSet.ReadSizeBytes, err = queue.getULongStatsSet(getStorageReadBytes)
	if err != nil {
		seelog.Warnf("Error getting storage read size bytes: %v", err)
	}
	storageStatsSet.WriteSizeBytes, err = queue.getULongStatsSet(getStorageWriteBytes)
	if err != nil {
		seelog.Warnf("Error getting storage write size bytes: %v", err)
	}
	return storageStatsSet, err
}

// GetNetworkStatsSet gets the stats set for network metrics.
func (queue *Queue) GetNetworkStatsSet() (*ecstcs.NetworkStatsSet, error) {
	networkStatsSet := &ecstcs.NetworkStatsSet{}
	var err error
	networkStatsSet.RxBytes, err = queue.getULongStatsSet(getNetworkRxBytes)
	if err != nil {
		seelog.Warnf("Error getting network rx bytes: %v", err)
	}
	networkStatsSet.RxDropped, err = queue.getULongStatsSet(getNetworkRxDropped)
	if err != nil {
		seelog.Warnf("Error getting network rx dropped: %v", err)
	}
	networkStatsSet.RxErrors, err = queue.getULongStatsSet(getNetworkRxErrors)
	if err != nil {
		seelog.Warnf("Error getting network rx errors: %v", err)
	}
	networkStatsSet.RxPackets, err = queue.getULongStatsSet(getNetworkRxPackets)
	if err != nil {
		seelog.Warnf("Error getting network rx packets: %v", err)
	}
	networkStatsSet.TxBytes, err = queue.getULongStatsSet(getNetworkTxBytes)
	if err != nil {
		seelog.Warnf("Error getting network tx bytes: %v", err)
	}
	networkStatsSet.TxDropped, err = queue.getULongStatsSet(getNetworkTxDropped)
	if err != nil {
		seelog.Warnf("Error getting network tx dropped: %v", err)
	}
	networkStatsSet.TxErrors, err = queue.getULongStatsSet(getNetworkTxErrors)
	if err != nil {
		seelog.Warnf("Error getting network tx errors: %v", err)
	}
	networkStatsSet.TxPackets, err = queue.getULongStatsSet(getNetworkTxPackets)
	if err != nil {
		seelog.Warnf("Error getting network tx packets: %v", err)
	}
	return networkStatsSet, err
}

func getNetworkRxBytes(s *UsageStats) uint64 {
	if s.NetworkStats != nil {
		return s.NetworkStats.RxBytes
	}
	return uint64(0)
}

func getNetworkRxDropped(s *UsageStats) uint64 {
	if s.NetworkStats != nil {
		return s.NetworkStats.RxDropped
	}
	return uint64(0)
}

func getNetworkRxErrors(s *UsageStats) uint64 {
	if s.NetworkStats != nil {
		return s.NetworkStats.RxErrors
	}
	return uint64(0)
}

func getNetworkRxPackets(s *UsageStats) uint64 {
	if s.NetworkStats != nil {
		return s.NetworkStats.RxPackets
	}
	return uint64(0)
}

func getNetworkTxBytes(s *UsageStats) uint64 {
	if s.NetworkStats != nil {
		return s.NetworkStats.TxBytes
	}
	return uint64(0)
}

func getNetworkTxDropped(s *UsageStats) uint64 {
	if s.NetworkStats != nil {
		return s.NetworkStats.TxDropped
	}
	return uint64(0)
}

func getNetworkTxErrors(s *UsageStats) uint64 {
	if s.NetworkStats != nil {
		return s.NetworkStats.TxErrors
	}
	return uint64(0)
}

func getNetworkTxPackets(s *UsageStats) uint64 {
	if s.NetworkStats != nil {
		return s.NetworkStats.TxPackets
	}
	return uint64(0)
}

// GetRawUsageStats gets the array of most recent raw UsageStats, in descending
// order of timestamps.
func (queue *Queue) GetRawUsageStats(numStats int) ([]UsageStats, error) {
	queue.lock.Lock()
	defer queue.lock.Unlock()

	queueLength := len(queue.buffer)
	if queueLength == 0 {
		return nil, fmt.Errorf("No data in the queue")
	}

	if numStats > queueLength {
		numStats = queueLength
	}

	usageStats := make([]UsageStats, numStats)
	for i := 0; i < numStats; i++ {
		// Order such that usageStats[i].timestamp > usageStats[i+1].timestamp
		rawUsageStat := queue.buffer[queueLength-i-1]
		usageStats[i] = UsageStats{
			CPUUsagePerc:      rawUsageStat.CPUUsagePerc,
			MemoryUsageInMegs: rawUsageStat.MemoryUsageInMegs,
			StorageReadBytes:  rawUsageStat.StorageReadBytes,
			StorageWriteBytes: rawUsageStat.StorageWriteBytes,
			NetworkStats:      rawUsageStat.NetworkStats,
			Timestamp:         rawUsageStat.Timestamp,
		}
	}

	return usageStats, nil
}

func getCPUUsagePerc(s *UsageStats) float64 {
	return float64(s.CPUUsagePerc)
}

func getMemoryUsagePerc(s *UsageStats) float64 {
	return float64(s.MemoryUsageInMegs)
}

func getStorageReadBytes(s *UsageStats) uint64 {
	return s.StorageReadBytes
}

func getStorageWriteBytes(s *UsageStats) uint64 {
	return s.StorageWriteBytes
}

// getInt64WithOverflow truncates a uint64 to fit an int64
// it returns overflow as a second int64
func getInt64WithOverflow(uintStat uint64) (int64, int64) {
	if uintStat > math.MaxInt64 {
		overflow := int64(uintStat % uint64(math.MaxInt64))
		return math.MaxInt64, overflow
	}
	return int64(uintStat), int64(0)
}

type getUsageFloatFunc func(*UsageStats) float64
type getUsageIntFunc func(*UsageStats) uint64

func (queue *Queue) resetThresholdElapsed(timeout time.Duration) bool {
	queue.lock.RLock()
	defer queue.lock.RUnlock()
	duration := time.Since(queue.lastResetTime)
	return duration.Seconds() > timeout.Seconds()
}

func (queue *Queue) enoughDatapointsInBuffer() bool {
	queue.lock.RLock()
	defer queue.lock.RUnlock()
	return len(queue.buffer) >= minimumQueueDatapoints
}

// getCWStatsSet gets the stats set for either CPU or Memory based on the
// function pointer.
func (queue *Queue) getCWStatsSet(f getUsageFloatFunc) (*ecstcs.CWStatsSet, error) {
	queue.lock.Lock()
	defer queue.lock.Unlock()

	queueLength := len(queue.buffer)
	if queueLength < 2 {
		// Need at least 2 data points to calculate this.
		return nil, fmt.Errorf("need at least 2 data points in queue to calculate CW stats set")
	}

	var min, max, sum float64
	var sampleCount int64
	min = math.MaxFloat64
	max = -math.MaxFloat64
	sum = 0
	sampleCount = 0

	for _, stat := range queue.buffer {
		thisStat := f(&stat)
		if math.IsNaN(thisStat) {
			continue
		}

		min = math.Min(min, thisStat)
		max = math.Max(max, thisStat)
		sampleCount++
		sum += thisStat
	}

	// don't emit metrics when sampleCount == 0
	if sampleCount == 0 {
		return nil, fmt.Errorf("need at least 1 non-NaN data points in queue to calculate CW stats set")
	}

	return &ecstcs.CWStatsSet{
		Max:         &max,
		Min:         &min,
		SampleCount: &sampleCount,
		Sum:         &sum,
	}, nil
}

// getULongStatsSet gets the stats set for the specified raw stat type
// stats come from docker as uint64 type, and by neccesity are packed into int64 type
// where there is overflow (math.MaxInt64 + 1 or greater)
// we capture the excess in optional overflow fields.
func (queue *Queue) getULongStatsSet(f getUsageIntFunc) (*ecstcs.ULongStatsSet, error) {
	queue.lock.Lock()
	defer queue.lock.Unlock()

	queueLength := len(queue.buffer)
	if queueLength < 2 {
		// Need at least 2 data points to calculate this.
		return nil, fmt.Errorf("need at least 2 data points in the queue to calculate int stats")
	}

	var min, max, sum uint64
	var sampleCount int64
	min = math.MaxUint64
	max = 0
	sum = 0
	sampleCount = 0

	for _, stat := range queue.buffer {
		thisStat := f(&stat)
		if thisStat < min {
			min = thisStat
		}
		if thisStat > max {
			max = thisStat
		}
		sum += thisStat
		sampleCount++
	}

	// don't emit metrics when sampleCount == 0
	if sampleCount == 0 {
		return nil, fmt.Errorf("need at least 1 non-NaN data points in queue to calculate CW stats set")
	}

	baseMin, overflowMin := getInt64WithOverflow(min)
	baseMax, overflowMax := getInt64WithOverflow(max)
	baseSum, overflowSum := getInt64WithOverflow(sum)

	return &ecstcs.ULongStatsSet{
		Max:         &baseMax,
		OverflowMax: &overflowMax,
		Min:         &baseMin,
		OverflowMin: &overflowMin,
		SampleCount: &sampleCount,
		Sum:         &baseSum,
		OverflowSum: &overflowSum,
	}, nil
}
