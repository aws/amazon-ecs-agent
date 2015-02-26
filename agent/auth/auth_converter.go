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
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"
	"github.com/awslabs/aws-sdk-go/aws"
)

type toSdkProvider struct {
	credentials credentials.AWSCredentialProvider
}

func (t *toSdkProvider) Credentials() (*aws.Credentials, error) {
	creds, err := t.credentials.Credentials()
	if err != nil {
		return nil, err
	}
	return &aws.Credentials{
		AccessKeyID:     creds.AccessKey,
		SecretAccessKey: creds.SecretKey,
		SessionToken:    creds.Token,
	}, nil
}

func ToSDK(creds credentials.AWSCredentialProvider) aws.CredentialsProvider {
	return &toSdkProvider{creds}
}
