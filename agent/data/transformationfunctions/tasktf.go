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
	"encoding/json"
	"fmt"

	"github.com/aws/amazon-ecs-agent/agent/data/models"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/modeltransformer"
)

// RegisterTaskTransformationFunctions calls all registerTaskTransformationFunctions<x_y_z> in ascending order.
// (from lower threshold version to higher threshold version) thresholdVersion is the version we introduce a breaking change in.
// All versions below threshold version need to go through that specific transformation function
func RegisterTaskTransformationFunctions(t *modeltransformer.Transformer) {
	registerTaskTransformationFunction1_x_0(t)
}

// registerTaskTransformationFunction1_x_0 is a template RegisterTaskTransformation function.
// It registers the transformation functions that translate the task model from models.Task_1_0_0 to models.Task_1_x_0
// Future addition to transformation functions should follow the same pattern. This current performs noop
// TODO: edit this function when introducing first actual transformation function, and add unit test
func registerTaskTransformationFunction1_x_0(t *modeltransformer.Transformer) {
	thresholdVersion := "1.0.0" // this assures it never actually gets executed
	t.AddTaskTransformationFunctions(thresholdVersion, func(dataIn []byte) ([]byte, error) {
		logger.Info(fmt.Sprintf("Executing transformation function with threshold %s.", thresholdVersion))
		oldModel := models.Task_1_0_0{}
		newModel := models.Task_1_x_0{}
		var intermediate map[string]interface{}

		// Load json to old model (so that we can capture some fields before it is deleted)
		err := json.Unmarshal(dataIn, &oldModel)
		if err != nil {
			return nil, err
		}

		// Load json to intermediate model to process
		err = json.Unmarshal(dataIn, &intermediate)
		if err != nil {
			return nil, err
		}

		// Actual process to process
		delete(intermediate, "ENIs")
		modifiedJSON, err := json.Marshal(intermediate)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(modifiedJSON, &newModel)
		newModel.NetworkInterfaces = oldModel.ENIs
		dataOut, err := json.Marshal(&newModel)
		logger.Info(fmt.Sprintf("Transform associated with version %s finished.", thresholdVersion))
		return dataOut, err
	})
	logger.Info(fmt.Sprintf("Registered transformation function with threshold %s.", thresholdVersion))
}
