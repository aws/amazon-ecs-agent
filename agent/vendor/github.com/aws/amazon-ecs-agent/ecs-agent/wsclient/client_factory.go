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

package wsclient

import (
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// ClientFactory interface abstracts the method that creates new ClientServer
// objects. This is helpful when writing unit tests.
type ClientFactory interface {
	New(url string, credentialCache *aws.CredentialsCache, rwTimeout time.Duration, cfg *WSClientMinAgentConfig, metricsFactory metrics.EntryFactory) ClientServer
}
