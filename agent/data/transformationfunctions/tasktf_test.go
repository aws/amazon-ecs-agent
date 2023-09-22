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

package transformationfunctions

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/modeltransformer"

	"github.com/stretchr/testify/assert"
)

const (
	expectedTaskTransformationChainLength = 1
)

func TestRegisterTaskTransformationFunctions(t *testing.T) {
	transformer := modeltransformer.NewTransformer()
	RegisterTaskTransformationFunctions(transformer)
	assert.Equal(t, expectedTaskTransformationChainLength, transformer.GetNumberOfTransformationFunctions("Task"))
}
