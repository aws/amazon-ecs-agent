// +build unit

// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package httpclient

import (
	"net/http"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/utils/cipher"
	"github.com/stretchr/testify/assert"
)

func TestNewHttpClient(t *testing.T) {
	expectedResult := New(time.Duration(10),true)
	transport := expectedResult.Transport.(*ecsRoundTripper)
	assert.Equal(t, cipher.SupportedCipherSuites, transport.transport.(*http.Transport).TLSClientConfig.CipherSuites)
	assert.Equal(t, true, transport.transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
}
