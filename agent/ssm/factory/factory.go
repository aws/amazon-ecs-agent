// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"github.com/aws/amazon-ecs-agent/agent/httpclient"
	ssmclient "github.com/aws/amazon-ecs-agent/agent/ssm"
	"github.com/aws/aws-sdk-go/aws"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
)

const (
	roundtripTimeout = 5 * time.Second
)

type SSMClientCreator interface {
	NewSSMClient(region string, creds credentials.IAMRoleCredentials) ssmclient.SSMClient
}

func NewSSMClientCreator() SSMClientCreator {
	return &ssmClientCreator{}
}

type ssmClientCreator struct{}

//SSM Client will automatically retry 3 times when has throttling error
func (*ssmClientCreator) NewSSMClient(region string,
	creds credentials.IAMRoleCredentials) ssmclient.SSMClient {
	cfg := aws.NewConfig().
		WithHTTPClient(httpclient.New(roundtripTimeout, false)).
		WithRegion(region).
		WithCredentials(
			awscreds.NewStaticCredentials(creds.AccessKeyID, creds.SecretAccessKey,
				creds.SessionToken))
	sess := session.Must(session.NewSession(cfg))
	return ssm.New(sess)
}
