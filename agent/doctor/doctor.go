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

	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

var (
	// EmptyHealthcheckError indicates an error when there are no healthcheck metrics to report
	EmptyHealthcheckError = errors.New("No instance healthcheck status metrics to report")
)

type Doctor struct {
	healthchecks         []Healthcheck
	lock                 sync.RWMutex
	cluster              string
	containerInstanceArn string
	statusReported       bool
}

func NewDoctor(healthchecks []Healthcheck, cluster string, containerInstanceArn string) (*Doctor, error) {
	newDoctor := &Doctor{
		healthchecks:         []Healthcheck{},
		cluster:              cluster,
		containerInstanceArn: containerInstanceArn,
		statusReported:       false,
	}
	for _, hc := range healthchecks {
		newDoctor.AddHealthcheck(hc)
	}
	return newDoctor, nil
}

// GetCluster returns the cluster that was provided to the doctor while
// being initialized
func (doc *Doctor) GetCluster() string {
	doc.lock.RLock()
	defer doc.lock.RUnlock()

	return doc.cluster
}

// GetContainerInstanceArn returns the container instance arn that was
// provided to the doctor while being initialized
func (doc *Doctor) GetContainerInstanceArn() string {
	doc.lock.RLock()
	defer doc.lock.RUnlock()

	return doc.containerInstanceArn
}

// SetStatusReported tells the doctor that we have already reported the
// current status of the healthchecks to the backend
func (doc *Doctor) SetStatusReported(statusReported bool) {
	doc.lock.Lock()
	defer doc.lock.Unlock()

	doc.statusReported = statusReported
}

// HasStatusBeenReported returns whether we have already sent the current
// state of the healthchecks to the backend or not
func (doc *Doctor) HasStatusBeenReported() bool {
	doc.lock.RLock()
	defer doc.lock.RUnlock()

	return doc.statusReported
}

// AddHealthcheck adds a healthcheck to the list of healthchecks that the
// doctor will run every time doctor.RunHealthchecks() is called
func (doc *Doctor) AddHealthcheck(healthcheck Healthcheck) {
	doc.lock.Lock()
	defer doc.lock.Unlock()
	doc.healthchecks = append(doc.healthchecks, healthcheck)
}

// RunHealthchecks runs every healthcheck that the doctor knows about and
// returns a cumulative result; true if they all pass, false otherwise
func (doc *Doctor) RunHealthchecks() bool {
	doc.lock.Lock()
	defer doc.lock.Unlock()
	allChecksResult := []HealthcheckStatus{}

	for _, healthcheck := range doc.healthchecks {
		res := healthcheck.RunCheck()
		seelog.Debugf("instance healthcheck result: %v", res)
		allChecksResult = append(allChecksResult, res)
	}

	doc.statusReported = false
	return doc.allRight(allChecksResult)
}

// GetHealthchecks returns a copy of list of healthchecks that the
// doctor is holding internally.
func (doc *Doctor) GetHealthchecks() *[]Healthcheck {
	doc.lock.RLock()
	defer doc.lock.RUnlock()

	healthcheckCopy := make([]Healthcheck, len(doc.healthchecks))
	copy(healthcheckCopy, doc.healthchecks)
	return &healthcheckCopy
}

func (doc *Doctor) allRight(checksResult []HealthcheckStatus) bool {
	overallResult := true
	for _, checkResult := range checksResult {
		overallResult = overallResult && checkResult.Ok()
	}
	return overallResult
}
