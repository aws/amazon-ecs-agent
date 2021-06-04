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
	"github.com/pborman/uuid"
	"github.com/pkg/errors"

	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/aws/aws-sdk-go/aws"
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
}

func NewDoctor(healthchecks []Healthcheck, cluster string, containerInstanceArn string) (*Doctor, error) {
	newDoctor := &Doctor{
		healthchecks:         []Healthcheck{},
		cluster:              cluster,
		containerInstanceArn: containerInstanceArn,
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

// GetPublishInstanceStatusRequest will get all healthcheck statuses and generate
// a sendable PublishInstanceStatusRequest
func (doc *Doctor) GetPublishInstanceStatusRequest() (*ecstcs.PublishInstanceStatusRequest, error) {
	metadata := &ecstcs.InstanceStatusMetadata{
		Cluster:           aws.String(doc.cluster),
		ContainerInstance: aws.String(doc.containerInstanceArn),
		RequestId:         aws.String(uuid.NewRandom().String()),
	}
	instanceStatuses := doc.getInstanceStatuses()

	if instanceStatuses != nil {
		return &ecstcs.PublishInstanceStatusRequest{
			Metadata:  metadata,
			Statuses:  instanceStatuses,
			Timestamp: aws.Time(time.Now()),
		}, nil
	} else {
		return nil, EmptyHealthcheckError
	}
}

func (doc *Doctor) getInstanceStatuses() []*ecstcs.InstanceStatus {
	var instanceStatuses []*ecstcs.InstanceStatus
	for _, healthcheck := range doc.healthchecks {
		instanceStatus := &ecstcs.InstanceStatus{
			LastStatusChange: aws.Time(healthcheck.GetStatusChangeTime()),
			LastUpdated:      aws.Time(healthcheck.GetLastHealthcheckTime()),
			Status:           aws.String(healthcheck.GetHealthcheckStatus().String()),
			Type:             aws.String(healthcheck.GetHealthcheckType()),
		}
		instanceStatuses = append(instanceStatuses, instanceStatus)
	}
	return instanceStatuses
}

func (doc *Doctor) allRight(checksResult []HealthcheckStatus) bool {
	overallResult := true
	for _, checkResult := range checksResult {
		overallResult = overallResult && checkResult.Ok()
	}
	return overallResult
}
