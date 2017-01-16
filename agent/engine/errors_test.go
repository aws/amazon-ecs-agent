// +build !integration
// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package engine

import (
	"errors"
	"testing"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/stretchr/testify/assert"
)

func TestUnretriableErrorReturnsTrueForNoSuchContainer(t *testing.T) {
	err := CannotStopContainerError{&docker.NoSuchContainer{}}
	assert.True(t, err.IsUnretriableError(), "No such container error should be treated as unretriable docker error")
}

func TestUnretriableErrorReturnsTrueForContainerNotRunning(t *testing.T) {
	err := CannotStopContainerError{&docker.ContainerNotRunning{}}
	assert.True(t, err.IsUnretriableError(), "ContainerNotRunning error should be treated as unretriable docker error")
}

func TestUnretriableErrorReturnsFalse(t *testing.T) {
	err := CannotStopContainerError{errors.New("error")}
	assert.False(t, err.IsUnretriableError(), "Non unretriable error treated as unretriable docker error")
}
