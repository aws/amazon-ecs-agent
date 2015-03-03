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

package auth

import (
	"errors"
	"sync"
	"time"

	. "github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"

	"github.com/aws/amazon-ecs-agent/agent/ec2"
)

const (
	TOKEN_REFRESH_FREQUENCY = 60.00 // Minutes

	// How many minutes early to refresh expiring tokens
	TOKEN_EXPIRATION_REFRESH_MINUTES time.Duration = 15.00 * time.Minute
)

type InstanceMetadataCredentialProvider struct {
	expiration  *time.Time
	lastCheck   *time.Time
	credentials AWSCredentials
	lock        sync.Mutex
}

func NewInstanceMetadataCredentialProvider() *InstanceMetadataCredentialProvider {
	imcp := new(InstanceMetadataCredentialProvider)
	return imcp
}

func (imcp *InstanceMetadataCredentialProvider) shouldRefresh() bool {
	if imcp.expiration != nil {
		if time.Now().Add(TOKEN_EXPIRATION_REFRESH_MINUTES).After(*imcp.expiration) {
			// Within the expiration window
			return true
		}
		// They'll expire, but haven't yet
		return false
	}

	if imcp.lastCheck == nil ||
		time.Since(*imcp.lastCheck).Minutes() > TOKEN_REFRESH_FREQUENCY {

		return true
	}
	return false
}

func (imcp *InstanceMetadataCredentialProvider) Credentials() (*AWSCredentials, error) {

	imcp.lock.Lock()
	defer imcp.lock.Unlock()

	if imcp.shouldRefresh() {
		now := time.Now()
		imcp.lastCheck = &now
		credStruct, err := ec2.DefaultCredentials()
		if err != nil {
			return &imcp.credentials, err
		}
		imcp.credentials = AWSCredentials{
			AccessKey: credStruct.AccessKeyId,
			SecretKey: credStruct.SecretAccessKey,
			Token:     credStruct.Token,
		}
		imcp.expiration = &credStruct.Expiration
	}

	if len(imcp.credentials.AccessKey) == 0 || len(imcp.credentials.SecretKey) == 0 {
		return nil, errors.New("Unable to find credentials in the instance metadata service")
	}

	return &imcp.credentials, nil
}
