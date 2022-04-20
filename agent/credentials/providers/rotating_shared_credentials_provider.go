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

package providers

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/cihub/seelog"
)

const (
	// defaultRotationInterval is how frequently to expire and re-retrieve the credentials from file.
	defaultRotationInterval = time.Minute
	// RotatingSharedCredentialsProviderName is the name of this provider
	RotatingSharedCredentialsProviderName = "RotatingSharedCredentialsProvider"
)

// RotatingSharedCredentialsProvider is a provider that retrieves credentials via the
// shared credentials provider, and adds the functionality of expiring and re-retrieving
// those credentials from the file.
type RotatingSharedCredentialsProvider struct {
	credentials.Expiry

	RotationInterval          time.Duration
	sharedCredentialsProvider *credentials.SharedCredentialsProvider
}

// NewRotatingSharedCredentials returns a rotating shared credentials provider
// with default values set.
func NewRotatingSharedCredentialsProvider() *RotatingSharedCredentialsProvider {
	return &RotatingSharedCredentialsProvider{
		RotationInterval: defaultRotationInterval,
		sharedCredentialsProvider: &credentials.SharedCredentialsProvider{
			Filename: defaultRotatingCredentialsFilename,
			Profile:  "default",
		},
	}
}

// Retrieve will use the given filename and profile and retrieve AWS credentials.
func (p *RotatingSharedCredentialsProvider) Retrieve() (credentials.Value, error) {
	v, err := p.sharedCredentialsProvider.Retrieve()
	v.ProviderName = RotatingSharedCredentialsProviderName
	if err != nil {
		return v, err
	}
	p.SetExpiration(time.Now().Add(p.RotationInterval), 0)
	seelog.Infof("Successfully got instance credentials from file %s. %s",
		p.sharedCredentialsProvider.Filename, credValueToString(v))
	return v, err
}

func credValueToString(v credentials.Value) string {
	akid := ""
	// only print last 4 chars if it's less than half the full AKID
	if len(v.AccessKeyID) > 8 {
		akid = v.AccessKeyID[len(v.AccessKeyID)-4:]
	}
	return fmt.Sprintf("Provider: %s. Access Key ID XXXX%s", v.ProviderName, akid)
}
