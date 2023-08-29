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

package modeltransformer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
)

const (
	modelTypeTask = "Task"
)

// Transformer stores transformation functions for all types of objects.
// Transform<type> will execute a series of transformation functions to make it compatible with current agent version.
// add<type>TransformationFunctions will add more <type> transformation functions to the transformation functions chain.
// Add other transformation functions as needed. e.g. ContainerTransformationFunctions.
// Add corresponding Transform<Type> and Add<Type>TransformationFunctions while adding other transformation functions.
type Transformer struct {
	taskTransformFunctions []*TransformFunc
}

// TransformFunc contains the threshold version string for transformation function and the transformationFunction itself.
// During upgrade, all models from versions below threshold version should execute the transformation function.
type TransformFunc struct {
	version  string
	function interface{}
}

func NewTransFormer() *Transformer {
	t := &Transformer{}
	return t
}

// GetNumberOfTransformationFunctions returns the number of transformation functions given a model type
func (t *Transformer) GetNumberOfTransformationFunctions(modelType string) int {
	switch modelType {
	case modelTypeTask:
		return len(t.taskTransformFunctions)
	default:
		return 0
	}
}

// TransformTask executes the transformation functions when version associated with model in boltdb is below the
func (t *Transformer) TransformTask(version string, data []byte) ([]byte, error) {
	var err error
	// execute transformation functions sequentially and skip those not applicable
	for _, transformFunc := range t.taskTransformFunctions {
		if closure, ok := transformFunc.function.(func([]byte) ([]byte, error)); ok {
			if checkVersionSmaller(version, transformFunc.version) {
				logger.Info(fmt.Sprintf("Agent version associated with task model in boltdb %s is below threshold %s. Transformation needed.", version, transformFunc.version))
				data, err = closure(data)
				if err != nil {
					return nil, err
				}
			} else {
				logger.Info(fmt.Sprintf("Agent version associated with task model in boltdb %s is bigger or equal to threshold %s. Skipping transformation.", version, transformFunc.version))
				continue
			}
		} else {
			return nil, fmt.Errorf("transformation function for version %s is not valid", transformFunc.version)
		}
	}
	return data, err
}

// AddTaskTransformationFunctions adds the transformationFunction to the handling chain
func (t *Transformer) AddTaskTransformationFunctions(version string, transformationFunc interface{}) {
	t.taskTransformFunctions = append(t.taskTransformFunctions, &TransformFunc{
		version:  version,
		function: transformationFunc,
	})
}

// IsUpgrade checks whether the load of a persisted model to running agent is an upgrade
func (t *Transformer) IsUpgrade(runningAgentVersion, persistedAgentVersion string) bool {
	return checkVersionSmaller(persistedAgentVersion, runningAgentVersion)
}

func checkVersionSmaller(version, threshold string) bool {
	versionParams := strings.Split(version, ".")
	thresholdParams := strings.Split(threshold, ".")

	for i := 0; i < len(versionParams); i++ {
		versionNumber, _ := strconv.Atoi(versionParams[i])
		thresholdNumber, _ := strconv.Atoi(thresholdParams[i])

		if thresholdNumber > versionNumber {
			return true
		}
	}
	return false
}
