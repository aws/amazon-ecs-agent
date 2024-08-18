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

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	loggerfield "github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/stats"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
)

const (
	// BytesInMiB is the number of bytes in a MebiByte.
	BytesInMiB              = 1024 * 1024
	MaxCPUUsagePerc float32 = 1024 * 1024
	NanoSecToSec    float32 = 1000000000
)

// Queue abstracts a queue using UsageStats slice.
type Queue struct {
	buffer                []UsageStats
	maxSize               int
	lastStat              *types.StatsJSON
	lastNetworkStatPerSec *stats.NetworkStatsPerSec
	lock                  sync.RWMutex
}

// NewQueue creates a queue.
func NewQueue(maxSize int) *Queue {
	return &Queue{
		buffer:  make([]UsageStats, 0, maxSize),
		maxSize: maxSize,
	}
}

// Reset resets the queue's buffer so that only new metrics added after
// this point will be sent to the backend when calling stat getter functions like
// GetCPUStatsSet, GetMemoryStatSet, etc.
func (queue *Queue) Reset() {
	queue.lock.Lock()
	defer queue.lock.Unlock()
	for i := range queue.buffer {
		queue.buffer[i].sent = true
	}
}

// AddContainerStat adds a new set of stats for a container to the queue. This method is only intended for use while
// processing docker stats for a stats container.
func (queue *Queue) AddContainerStat(dockerStat *types.StatsJSON, nonDockerStats NonDockerContainerStats,
	lastStatBeforeLastRestart *types.StatsJSON, containerHasRestartedBefore bool) error {
	if containerHasRestartedBefore {
		dockerStat = getAggregatedDockerStatAcrossRestarts(dockerStat, lastStatBeforeLastRestart, queue.GetLastStat())
	}

	return queue.Add(dockerStat, nonDockerStats)
}

// Add adds a new set of stats to the queue.
func (queue *Queue) Add(dockerStat *types.StatsJSON, nonDockerStats NonDockerContainerStats) error {
	queue.setLastStat(dockerStat)
	stat, err := dockerStatsToContainerStats(dockerStat)
	if err != nil {
		return err
	}
	if nonDockerStats.restartCount != nil {
		stat.restartCount = nonDockerStats.restartCount
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
		RestartCount:      rawStat.restartCount,
		MemoryUsageInMegs: uint32(rawStat.memoryUsage / BytesInMiB),
		StorageReadBytes:  rawStat.storageReadBytes,
		StorageWriteBytes: rawStat.storageWriteBytes,
		NetworkStats:      rawStat.networkStats,
		Timestamp:         rawStat.timestamp,
		cpuUsage:          rawStat.cpuUsage,
		sent:              false,
	}
	if queueLength != 0 {
		// % utilization can be calculated only when queue is non-empty.
		lastStat := queue.buffer[queueLength-1]
		timeSinceLastStat := float32(rawStat.timestamp.Sub(lastStat.Timestamp).Nanoseconds())
		if timeSinceLastStat <= 0 {
			// if we got a duplicate timestamp, set cpu percentage to the same value as the previous stat
			seelog.Errorf("Received a docker stat object with duplicate timestamp")
			stat.CPUUsagePerc = lastStat.CPUUsagePerc
			if stat.NetworkStats != nil && lastStat.NetworkStats != nil {
				stat.NetworkStats.RxBytesPerSecond = lastStat.NetworkStats.RxBytesPerSecond
				stat.NetworkStats.TxBytesPerSecond = lastStat.NetworkStats.TxBytesPerSecond
			}
		} else {
			cpuUsageSinceLastStat := float32(rawStat.cpuUsage - lastStat.cpuUsage)
			stat.CPUUsagePerc = 100 * cpuUsageSinceLastStat / timeSinceLastStat

			//calculate per second Network metrics
			if stat.NetworkStats != nil && lastStat.NetworkStats != nil {
				rxBytesSinceLastStat := float32(stat.NetworkStats.RxBytes - lastStat.NetworkStats.RxBytes)
				txBytesSinceLastStat := float32(stat.NetworkStats.TxBytes - lastStat.NetworkStats.TxBytes)
				stat.NetworkStats.RxBytesPerSecond = NanoSecToSec * (rxBytesSinceLastStat / timeSinceLastStat)
				stat.NetworkStats.TxBytesPerSecond = NanoSecToSec * (txBytesSinceLastStat / timeSinceLastStat)
			}
		}

		if queueLength >= queue.maxSize {
			// Remove first element if queue is full.
			queue.buffer = queue.buffer[1:queueLength]
		}

		if stat.CPUUsagePerc > MaxCPUUsagePerc {
			// what in the world happened
			seelog.Errorf("Calculated CPU usage percent (%.1f) is larger than backend maximum (%.1f). lastStatTS=%s lastStatCPUTime=%d thisStatTS=%s thisStatCPUTime=%d queueLength=%d",
				stat.CPUUsagePerc, MaxCPUUsagePerc, lastStat.Timestamp.Format(time.RFC3339Nano), lastStat.cpuUsage, rawStat.timestamp.Format(time.RFC3339Nano), rawStat.cpuUsage, queueLength)
		}

		if stat.NetworkStats != nil {
			networkStatPerSec := &stats.NetworkStatsPerSec{
				RxBytesPerSecond: float64(stat.NetworkStats.RxBytesPerSecond),
				TxBytesPerSecond: float64(stat.NetworkStats.TxBytesPerSecond),
			}
			queue.lastNetworkStatPerSec = networkStatPerSec
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

func (queue *Queue) GetLastNetworkStatPerSec() *stats.NetworkStatsPerSec {
	queue.lock.RLock()
	defer queue.lock.RUnlock()

	return queue.lastNetworkStatPerSec
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
	var errStr string
	storageStatsSet.ReadSizeBytes, err = queue.getULongStatsSet(getStorageReadBytes)
	if err != nil {
		errStr += fmt.Sprintf("error getting storage read size bytes: %v - ", err)
	}
	storageStatsSet.WriteSizeBytes, err = queue.getULongStatsSet(getStorageWriteBytes)
	if err != nil {
		errStr += fmt.Sprintf("error getting storage write size bytes: %v - ", err)
	}
	var errOut error
	if len(errStr) > 0 {
		errOut = fmt.Errorf(errStr)
	}
	return storageStatsSet, errOut
}

// GetRestartStatsSet gets the stats set for container restarts
func (queue *Queue) GetRestartStatsSet() (*ecstcs.RestartStatsSet, error) {
	return queue.getRestartStatsSet(getRestartCount)
}

func (queue *Queue) getRestartStatsSet(getInt getIntPointerFunc) (*ecstcs.RestartStatsSet, error) {
	queue.lock.Lock()
	defer queue.lock.Unlock()

	var firstStat, lastStat int64
	firstStat = -1

	queueLength := len(queue.buffer)
	if queueLength < 2 {
		// Need at least 2 data points to calculate this.
		return nil, fmt.Errorf("need at least 2 data points in queue to calculate restart stats set")
	}

	for i, stat := range queue.buffer {
		if stat.sent {
			// don't send stats to TACS if already sent
			continue
		}
		thisStat := getInt(&stat)
		if thisStat == nil {
			continue
		}
		if i == queueLength-1 {
			// get final unsent stat in the queue
			lastStat = *thisStat
		}

		if firstStat == -1 {
			// firstStat is unset so set it here
			if i > 0 {
				// some stats in queue were sent already, so use the stat previous to the
				// first to diff with the last stat.
				thisStat = getInt(&queue.buffer[i-1])
				if thisStat == nil {
					continue
				}
			}
			firstStat = *thisStat
		}
	}

	if firstStat == -1 {
		// no non-nil stats found, most likely this means that the container
		// does not have a restart policy set.
		return nil, fmt.Errorf("No non-nil restart count stats found, not sending RestartStatsSet." +
			" Most likely the container does not have a restart policy configured")
	}

	// get the diff in restart count between the first stat and the last stat in the
	// queue. Examples:
	//    [ 0 1 3 3 4 5 ] = 5 - 0 = 5 restarts
	//    [ 0(sent) 1(sent) 3(sent) 4(unsent) 5(unsent) 7(unsent) ] = 7 - 3 = 4 restarts
	result := lastStat - firstStat
	if result < 0 {
		return nil, fmt.Errorf("Negative restart count calculated, firstStat=%d lastStat=%d result=%d", firstStat, lastStat, result)
	}

	return &ecstcs.RestartStatsSet{
		RestartCount: &result,
	}, nil
}

// GetNetworkStatsSet gets the stats set for network metrics.
func (queue *Queue) GetNetworkStatsSet() (*ecstcs.NetworkStatsSet, error) {
	networkStatsSet := &ecstcs.NetworkStatsSet{}
	var err error
	var errStr string
	networkStatsSet.RxBytes, err = queue.getULongStatsSet(getNetworkRxBytes)
	if err != nil {
		errStr += fmt.Sprintf("error getting network rx bytes: %v - ", err)
	}
	networkStatsSet.RxDropped, err = queue.getULongStatsSet(getNetworkRxDropped)
	if err != nil {
		errStr += fmt.Sprintf("error getting network rx dropped: %v - ", err)
	}
	networkStatsSet.RxErrors, err = queue.getULongStatsSet(getNetworkRxErrors)
	if err != nil {
		errStr += fmt.Sprintf("error getting network rx errors: %v - ", err)
	}
	networkStatsSet.RxPackets, err = queue.getULongStatsSet(getNetworkRxPackets)
	if err != nil {
		errStr += fmt.Sprintf("error getting network rx packets: %v - ", err)
	}
	networkStatsSet.TxBytes, err = queue.getULongStatsSet(getNetworkTxBytes)
	if err != nil {
		errStr += fmt.Sprintf("error getting network tx bytes: %v - ", err)
	}
	networkStatsSet.TxDropped, err = queue.getULongStatsSet(getNetworkTxDropped)
	if err != nil {
		errStr += fmt.Sprintf("error getting network tx dropped: %v - ", err)
	}
	networkStatsSet.TxErrors, err = queue.getULongStatsSet(getNetworkTxErrors)
	if err != nil {
		errStr += fmt.Sprintf("error getting network tx errors: %v - ", err)
	}
	networkStatsSet.TxPackets, err = queue.getULongStatsSet(getNetworkTxPackets)
	if err != nil {
		errStr += fmt.Sprintf("error getting network tx packets: %v - ", err)
	}
	networkStatsSet.RxBytesPerSecond, err = queue.getUDoubleCWStatsSet(getNetworkRxPacketsPerSecond)
	if err != nil {
		errStr += fmt.Sprintf("error getting network rx bytes per second: %v - ", err)
	}
	networkStatsSet.TxBytesPerSecond, err = queue.getUDoubleCWStatsSet(getNetworkTxPacketsPerSecond)
	if err != nil {
		errStr += fmt.Sprintf("error getting network tx bytes per second: %v - ", err)
	}
	var errOut error
	if len(errStr) > 0 {
		errOut = fmt.Errorf(errStr)
	}
	return networkStatsSet, errOut
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

func getNetworkRxPacketsPerSecond(s *UsageStats) float64 {
	if s.NetworkStats != nil {
		return float64(s.NetworkStats.RxBytesPerSecond)
	}
	return float64(0)
}

func getNetworkTxPacketsPerSecond(s *UsageStats) float64 {
	if s.NetworkStats != nil {
		return float64(s.NetworkStats.TxBytesPerSecond)
	}
	return float64(0)
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

func getRestartCount(s *UsageStats) *int64 {
	return s.RestartCount
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
type getIntPointerFunc func(*UsageStats) *int64

// getCWStatsSet gets the stats set for either CPU or Memory based on the
// function pointer.
func (queue *Queue) getCWStatsSet(getUsageFloat getUsageFloatFunc) (*ecstcs.CWStatsSet, error) {
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
		if stat.sent {
			// don't send stats to TACS if already sent
			continue
		}
		thisStat := getUsageFloat(&stat)
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
func (queue *Queue) getULongStatsSet(getUsageInt getUsageIntFunc) (*ecstcs.ULongStatsSet, error) {
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
		if stat.sent {
			// don't send stats to TACS if already sent
			continue
		}
		thisStat := getUsageInt(&stat)
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

// getUDoubleCWStatsSet gets the stats set for per second network metrics
func (queue *Queue) getUDoubleCWStatsSet(getUsageFloat getUsageFloatFunc) (*ecstcs.UDoubleCWStatsSet, error) {
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
		if stat.sent {
			// don't send stats to TACS if already sent
			continue
		}
		thisStat := getUsageFloat(&stat)
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

	return &ecstcs.UDoubleCWStatsSet{
		Max:         &max,
		Min:         &min,
		SampleCount: &sampleCount,
		Sum:         &sum,
	}, nil
}

// getAggregatedDockerStatAcrossRestarts gets the aggregated docker stat for a container across container restarts.
func getAggregatedDockerStatAcrossRestarts(dockerStat, lastStatBeforeLastRestart,
	lastStatInStatsQueue *types.StatsJSON) *types.StatsJSON {
	dockerStat = aggregateOSIndependentStats(dockerStat, lastStatBeforeLastRestart)
	dockerStat = aggregateOSDependentStats(dockerStat, lastStatBeforeLastRestart)

	// Stats relevant to PreCPU.
	preCPUStats := types.CPUStats{}
	preRead := time.Time{}
	if lastStatInStatsQueue != nil {
		preCPUStats = lastStatInStatsQueue.CPUStats
		preRead = lastStatInStatsQueue.Read
	}
	dockerStat.PreCPUStats = preCPUStats
	dockerStat.PreRead = preRead

	logger.Debug("Aggregated Docker stat across restart(s)", logger.Fields{
		loggerfield.DockerId: dockerStat.ID,
	})

	return dockerStat
}

// aggregateOSIndependentStats aggregates stats that are measured cumulatively against container start time and
// populated regardless of what OS is being used.
func aggregateOSIndependentStats(dockerStat, lastStatBeforeLastRestart *types.StatsJSON) *types.StatsJSON {
	// CPU stats.
	dockerStat.CPUStats.CPUUsage.TotalUsage += lastStatBeforeLastRestart.CPUStats.CPUUsage.TotalUsage
	dockerStat.CPUStats.CPUUsage.UsageInKernelmode += lastStatBeforeLastRestart.CPUStats.CPUUsage.UsageInKernelmode
	dockerStat.CPUStats.CPUUsage.UsageInUsermode += lastStatBeforeLastRestart.CPUStats.CPUUsage.UsageInUsermode

	// Network stats.
	for key, dockerStatNetwork := range dockerStat.Networks {
		lastStatBeforeLastRestartNetwork, ok := lastStatBeforeLastRestart.Networks[key]
		if ok {
			dockerStatNetwork.RxBytes += lastStatBeforeLastRestartNetwork.RxBytes
			dockerStatNetwork.RxPackets += lastStatBeforeLastRestartNetwork.RxPackets
			dockerStatNetwork.RxDropped += lastStatBeforeLastRestartNetwork.RxDropped
			dockerStatNetwork.TxBytes += lastStatBeforeLastRestartNetwork.TxBytes
			dockerStatNetwork.TxPackets += lastStatBeforeLastRestartNetwork.TxPackets
			dockerStatNetwork.TxDropped += lastStatBeforeLastRestartNetwork.TxDropped
		}
		dockerStat.Networks[key] = dockerStatNetwork
	}
	return dockerStat
}
