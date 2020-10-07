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

package instancecreds

import (
	"sync"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/cihub/seelog"
)

var (
	credentialChain *credentials.Credentials
	mu              sync.RWMutex
)

func GetCredentials() *credentials.Credentials {
	mu.Lock()
	if credentialChain == nil {
		credentialChain = credentials.NewCredentials(&credentials.ChainProvider{
			VerboseErrors: false,
			Providers:     defaults.CredProviders(defaults.Config(), defaults.Handlers()),
		})
	}
	mu.Unlock()

	// credentials.Credentials is concurrency-safe, so lock not needed here
	v, err := credentialChain.Get()
	if err != nil {
		seelog.Errorf("Error getting ECS instance credentials from default chain: %s", err)
	} else {
		seelog.Infof("Successfully got ECS instance credentials from provider: %s", v.ProviderName)
	}
	return credentialChain
}
