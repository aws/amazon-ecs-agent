// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//    http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package factory

import (
	"time"

	"github.com/aws/amazon-ecs-agent/agent/credentials"
	fsxclient "github.com/aws/amazon-ecs-agent/agent/fsx"
	"github.com/aws/amazon-ecs-agent/agent/httpclient"
	"github.com/aws/aws-sdk-go/aws"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/fsx"
)

const (
	roundtripTimeout = 5 * time.Second
)

type FSxClientCreator interface {
	NewFSxClient(region string, creds credentials.IAMRoleCredentials) fsxclient.FSxClient
}

func NewFSxClientCreator() FSxClientCreator {
	return &fsxClientCreator{}
}

type fsxClientCreator struct{}

func (*fsxClientCreator) NewFSxClient(region string,
	creds credentials.IAMRoleCredentials) fsxclient.FSxClient {
	cfg := aws.NewConfig().
		WithHTTPClient(httpclient.New(roundtripTimeout, false)).
		WithRegion(region).
		WithCredentials(
			awscreds.NewStaticCredentials(creds.AccessKeyID, creds.SecretAccessKey,
				creds.SessionToken))
	sess := session.Must(session.NewSession(cfg))
	return fsx.New(sess)
}
