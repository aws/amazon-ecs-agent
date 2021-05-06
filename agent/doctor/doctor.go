// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//      http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package doctor

import (
	"sync"
	"time"

	"github.com/cihub/seelog"
)

type Doctor struct {
	healthchecks []Healthcheck
	lock         sync.RWMutex
}

func NewDoctor(healthchecks []Healthcheck) (*Doctor, error) {
	newDoctor := &Doctor{
		healthchecks: []Healthcheck{},
	}
	for _, hc := range healthchecks {
		newDoctor.AddHealthcheck(hc)
	}
	return newDoctor, nil
}

func (doc *Doctor) AddHealthcheck(healthcheck Healthcheck) {
	doc.lock.Lock()
	defer doc.lock.Unlock()
	doc.healthchecks = append(doc.healthchecks, healthcheck)
}

func (doc *Doctor) RunHealthchecks() bool {
	doc.lock.RLock()
	defer doc.lock.RUnlock()
	allChecksResult := []HealthcheckStatus{}

	for _, healthcheck := range doc.healthchecks {
		res := healthcheck.RunCheck()
		seelog.Debugf("instance healthcheck result: %v", res)
		allChecksResult = append(allChecksResult, res)
	}
	return doc.allRight(allChecksResult)
}

func (doc *Doctor) allRight(checksResult []HealthcheckStatus) bool {
	overallResult := true
	for _, checkResult := range checksResult {
		overallResult = overallResult && checkResult.Ok()
	}
	return overallResult
}

type Healthcheck interface {
	GetLastHealthcheckStatus() HealthcheckStatus
	GetLastHealthcheckTime() time.Time
	GetHealthcheckStatus() HealthcheckStatus
	GetHealthcheckTime() time.Time
	RunCheck() HealthcheckStatus
	SetHealthcheckStatus(status HealthcheckStatus)
}
