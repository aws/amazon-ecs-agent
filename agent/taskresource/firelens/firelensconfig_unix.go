// +build linux
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

import (
	generator "github.com/awslabs/go-config-generator-for-fluentd-and-fluentbit"
)

const (
	// socketInputNameFluentd is the name of the socket input plugin for fluentd.
	socketInputNameFluentd = "unix"

	// socketInputPathOptionFluentd is the key for specifying socket path for fluentd.
	socketInputPathOptionFluentd = "path"

	// socketInputPathOptionFluentbit is the key for specifying socket path for fluentbit.
	socketInputPathOptionFluentbit = "unix_path"

	// socketPath is the path for socket file.
	socketPath = "/var/run/fluent.sock"

	// S3ConfigPathFluentd and S3ConfigPathFluentbit are the paths where we bind mount the config downloaded from S3 to.
	S3ConfigPathFluentd   = "/fluentd/etc/external.conf"
	S3ConfigPathFluentbit = "/fluent-bit/etc/external.conf"

	// inputBridgeBindValue is the value for specifying host for Bridge mode.
	inputBridgeBindValue = "0.0.0.0"

	// inputAWSVPCBindValue is the value for specifying host for AWSVPC mode.
	inputAWSVPCBindValue = "127.0.0.1"
)

// Specify log stream input, which is a unix socket that will be used for communication between the Firelens
// container and other containers.
func (firelens *FirelensResource) addSocketInput(config generator.FluentConfig) {
	var inputName, inputPathOption string
	if firelens.firelensConfigType == FirelensConfigTypeFluentd {
		inputName = socketInputNameFluentd
		inputPathOption = socketInputPathOptionFluentd
	} else {
		inputName = inputNameForward
		inputPathOption = socketInputPathOptionFluentbit
	}
	config.AddInput(inputName, "", map[string]string{
		inputPathOption: socketPath,
	})
}

// addHealthcheckSections adds a health check input section and a health check output section to the config.
func (firelens *FirelensResource) addHealthcheckSections(config generator.FluentConfig) {
	// Health check supported is only added for fluentbit.
	if firelens.firelensConfigType != FirelensConfigTypeFluentbit {
		return
	}

	// Add healthcheck input section.
	inputName := healthcheckInputNameFluentbit
	inputOptions := map[string]string{
		inputPortOptionFluentbit:   healthcheckInputPortValue,
		inputListenOptionFluentbit: healthcheckInputBindValue,
	}
	config.AddInput(inputName, healthcheckTag, inputOptions)

	// Add healthcheck output section.
	config.AddOutput(healthcheckOutputName, healthcheckTag, nil)
}

func (firelens *FirelensResource) getS3ConfPath() string {
	if firelens.firelensConfigType == FirelensConfigTypeFluentd {
		return S3ConfigPathFluentd
	} else {
		return S3ConfigPathFluentbit
	}
}
