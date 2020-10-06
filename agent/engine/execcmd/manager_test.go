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

package execcmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	assert.Equal(t, HostBinDir, m.hostBinDir)
	assert.Equal(t, defaultInspectRetryTimeout, m.inspectRetryTimeout)
	assert.Equal(t, defaultRetryMinDelay, m.retryMinDelay)
	assert.Equal(t, defaultRetryMaxDelay, m.retryMaxDelay)
	assert.Equal(t, defaultStartRetryTimeout, m.startRetryTimeout)
}

func TestNewManagerWithBinDir(t *testing.T) {
	const customHostBinDir = "/test"
	m := NewManagerWithBinDir(customHostBinDir)
	assert.Equal(t, customHostBinDir, m.hostBinDir)
	assert.Equal(t, defaultInspectRetryTimeout, m.inspectRetryTimeout)
	assert.Equal(t, defaultRetryMinDelay, m.retryMinDelay)
	assert.Equal(t, defaultRetryMaxDelay, m.retryMaxDelay)
	assert.Equal(t, defaultStartRetryTimeout, m.startRetryTimeout)
}
