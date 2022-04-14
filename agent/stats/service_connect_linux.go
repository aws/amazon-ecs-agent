//go:build linux

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
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/cihub/seelog"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
)

const (
	statsURLPath      = ":9901/stats/prometheus"
	bridgeNetworkMode = "bridge"
)

type ServiceConnectStats struct {
	//TODO [SC]: Change the type of Service Connect stats when it is defined.
	stats string
	lock  sync.RWMutex
}

func newServiceConnectStats() (*ServiceConnectStats, error) {
	return &ServiceConnectStats{}, nil
}

func (sc *ServiceConnectStats) retrieveServiceConnectStats(task *apitask.Task) {
	var ipAddress string
	if task.IsNetworkModeAWSVPC() {
		ipAddress = task.GetLocalIPAddress()
	} else {
		for _, container := range task.Containers {
			// TODO [SC]: Implement a proper check for appnet agent container when it is defined
			if strings.Contains(container.Name, "envoy") &&
				(container.GetNetworkModeFromHostConfig() == "" || container.GetNetworkModeFromHostConfig() == bridgeNetworkMode) {
				network := *container.GetNetworkSettings()
				networkSettings := network.DefaultNetworkSettings
				ipAddress = networkSettings.IPAddress
				break
			}
		}
	}
	// TODO [SC]: Replace the stats url when it is defined
	statsEndpoint, _ := url.Parse(fmt.Sprintf("http://" + ipAddress + statsURLPath))
	resp, err := http.Get(statsEndpoint.String())
	if err != nil {
		seelog.Errorf("Could not connect to service connect stats endpoint for task %s: %v", task.Arn, err)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		seelog.Errorf("Could not read metrics from service connect stats endpoint for task %s: %v", task.Arn, err)
		return
	}

	sc.setStats(string(body))
}

func (sc *ServiceConnectStats) setStats(stats string) {
	sc.lock.Lock()
	defer sc.lock.Unlock()

	sc.stats = stats
}

func (sc *ServiceConnectStats) GetStats() string {
	sc.lock.RLock()
	defer sc.lock.RUnlock()

	return sc.stats
}
