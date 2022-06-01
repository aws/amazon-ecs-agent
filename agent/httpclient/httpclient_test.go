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

package httpclient

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/utils/cipher"
	"github.com/stretchr/testify/assert"
)

func TestNewHttpClient(t *testing.T) {
	expectedResult := New(time.Duration(10), true)
	transport := expectedResult.Transport.(*ecsRoundTripper)
	assert.Equal(t, cipher.SupportedCipherSuites, transport.transport.(*http.Transport).TLSClientConfig.CipherSuites)
	assert.Equal(t, true, transport.transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
}

func TestNewHttpClientProxy(t *testing.T) {
	proxy_url := "127.0.0.1:1234"
	// Set Proxy
	os.Setenv("HTTP_PROXY", proxy_url)
	defer os.Unsetenv("HTTP_PROXY")

	client := New(time.Duration(10*time.Second), true)
	_, err := client.Get("http://www.amazon.com")
	// Client won't be able to connect because we have given a arbitrary proxy
	assert.Error(t, err)
	// Error message should contain the proxy url which shows that client tried to use the proxy url to connect
	assert.True(t, strings.Contains(err.Error(), proxy_url), "proxy url not found in: %s", err.Error())
}
