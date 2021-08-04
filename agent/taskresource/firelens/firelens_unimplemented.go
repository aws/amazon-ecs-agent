// +build !linux,!windows

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

package firelens

import generator "github.com/awslabs/go-config-generator-for-fluentd-and-fluentbit"

// generateConfig generates a FluentConfig object that contains all necessary information to construct
// a fluentd or fluentbit config file for a firelens container.
func (firelens *FirelensResource) generateConfig() (generator.FluentConfig, error) {
	config := generator.New()
	return config, nil
}

// createDirectories creates two directories:
//  - $(DATA_DIR)/firelens/$(TASK_ID)/config: used to store firelens config file. The config file under this directory
//    will be mounted to the firelens container at an expected path.
//  - $(DATA_DIR)/firelens/$(TASK_ID)/socket: used to store the unix socket. This directory will be mounted to
//    the firelens container and it will generate a socket file under this directory. Containers that use firelens to
//    send logs will then use this socket to send logs to the firelens container.
// Note: socket path has a limit of at most 108 characters on Linux. If using default data dir, the
// resulting socket path will be 79 characters (/var/lib/ecs/data/firelens/<task-id>/socket/fluent.sock) which is fine.
// However if ECS_HOST_DATA_DIR is specified to be a longer path, we will exceed the limit and fail. I don't really
// see a way to avoid this failure since ECS_HOST_DATA_DIR can be arbitrary long..
func (firelens *FirelensResource) createDirectories() error {
	return nil
}
