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

package logger

import (
	"runtime"
	"strconv"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/cihub/seelog"
)

var (
	prevStats          = &runtime.MemStats{}
	runtimeStatsLogger seelog.LoggerInterface
	initTime           = time.Now()
	statsTicker        = time.NewTicker(time.Minute * 2)
)

func StartRuntimeStatsLogger(cfg *config.Config) (flush func()) {
	l, err := seelog.LoggerFromConfigAsString(runtimeStatsLogConfig(cfg))
	if err != nil {
		seelog.Errorf("Error initializing the runtime stats log: %v", err)
		// If the logger cannot be initialized, use the provided dummy seelog.LoggerInterface, seelog.Disabled.
		l = seelog.Disabled
	}
	runtimeStatsLogger = l
	go run()
	return func() {
		statsTicker.Stop()
		runtimeStatsLogger.Flush()
	}
}

func run() {
	logStats()
	for range statsTicker.C {
		logStats()
	}
}

func logStats() {
	curStats := &runtime.MemStats{}
	runtimeStatsStart := time.Now()
	runtime.ReadMemStats(curStats)
	numGoRoutines := runtime.NumGoroutine()
	readMemStatsOverhead := time.Since(runtimeStatsStart)

	runtimeStatsLogger.Infof(
		"Time=%s, "+
			"FreesCount=%d, "+
			"HeapMemoryAlloc=%d, "+
			"HeapMemoryInUse=%d, "+
			"HeapMemoryIdle=%d, "+
			"HeapMemoryReleased=%d, "+
			"MallocCount=%d, "+
			"StackMemoryInUse=%d, "+
			"GarbageCollectionCount=%d, "+
			"GarbageCollectionTime=%v, "+
			"GCCPUFraction=%f, "+
			"GoroutineCount=%d, "+
			"UpTime=%v, "+
			"ReadMemStatsOverhead=%s",
		time.Now().UTC().Format(time.RFC3339),
		curStats.Frees-prevStats.Frees,
		curStats.HeapAlloc,
		curStats.HeapInuse,
		curStats.HeapIdle,
		curStats.HeapReleased,
		curStats.Mallocs-prevStats.Mallocs,
		curStats.StackInuse,
		curStats.NumGC-prevStats.NumGC,
		time.Duration(curStats.PauseTotalNs-prevStats.PauseTotalNs)*time.Nanosecond,
		curStats.GCCPUFraction,
		numGoRoutines,
		time.Since(initTime),
		readMemStatsOverhead,
	)
	prevStats = curStats
}

func runtimeStatsLogConfig(cfg *config.Config) string {
	config := `
<seelog type="asyncloop" minlevel="info">
	<outputs formatid="main">
		<console />
		<rollingfile filename="` + cfg.RuntimeStatsLogFile + `" type="size"
		 maxsize="` + strconv.Itoa(int(Config.MaxFileSizeMB*1000000)) + `" archivetype="none" maxrolls="3" />
	</outputs>
	<formats>
		<format id="main" format="%Msg%n" />
	</formats>
</seelog>
`
	return config
}
