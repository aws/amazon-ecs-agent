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

package data

import (
	"github.com/aws/amazon-ecs-agent/agent/engine/image"
)

func (c *client) SaveImageState(img *image.ImageState) error {
	// TODO: implementation
	return nil
}

func (c *client) DeleteImageState(id string) error {
	// TODO: implementation
	return nil
}

func (c *client) GetImageStates() ([]*image.ImageState, error) {
	// TODO: implementation
	return nil, nil
}
