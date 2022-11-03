//go:build unit
// +build unit

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

package credentialspec

import (
	"testing"
	"time"

	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/stretchr/testify/assert"
)

func TestGetResourceName(t *testing.T) {
	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{},
	}

	assert.Equal(t, ResourceName, cs.GetName())
}

func TestGetDesiredStatus(t *testing.T) {
	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{},
	}

	cs.SetDesiredStatus(resourcestatus.ResourceCreated)
	assert.Equal(t, resourcestatus.ResourceCreated, cs.GetDesiredStatus())
}

func TestGetTerminalReason(t *testing.T) {
	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{},
	}

	reason := "failed to read credentialspec"
	cs.setTerminalReason(reason)
	assert.Equal(t, reason, cs.GetTerminalReason())
}

func TestKnownCreated(t *testing.T) {
	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{},
	}

	cs.SetKnownStatus(resourcestatus.ResourceCreated)
	assert.True(t, cs.KnownCreated())
}

func TestNextKnownState(t *testing.T) {
	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{},
	}

	cs.SetKnownStatus(resourcestatus.ResourceCreated)
	assert.Equal(t, resourcestatus.ResourceRemoved, cs.NextKnownState())
}

func TestCreatedAt(t *testing.T) {
	time := time.Now()

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{},
	}

	cs.SetCreatedAt(time)
	assert.Equal(t, time, cs.GetCreatedAt())
}

func TestMarshallandUnMarshallCredSpec(t *testing.T) {
	containerName := "webapp"

	credentialSpecSSMARN := "arn:aws:ssm:us-west-2:123456789012:parameter/test"

	credentialSpecContainerMap := map[string]string{
		credentialSpecSSMARN: containerName,
	}

	cs := &CredentialSpecResource{
		CredentialSpecResourceCommon: &CredentialSpecResourceCommon{
			knownStatusUnsafe:          resourcestatus.ResourceCreated,
			desiredStatusUnsafe:        resourcestatus.ResourceCreated,
			CredSpecMap:                map[string]string{},
			taskARN:                    taskARN,
			credentialSpecContainerMap: credentialSpecContainerMap,
		},
	}

	parsedBytes, err := cs.MarshalJSON()
	assert.NoError(t, err)

	err = cs.UnmarshalJSON(parsedBytes)
	assert.NoError(t, err)
}
