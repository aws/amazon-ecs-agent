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

const (
	S3ConfigPathFluentd   = ""
	S3ConfigPathFluentbit = ""
	inputBridgeBindValue  = ""
	inputAWSVPCBindValue  = ""
)

// Specify log stream input, which is a unix socket that will be used for communication between the Firelens
// container and other containers.
func (firelens *FirelensResource) addSocketInput(config generator.FluentConfig) {

}

// addHealthcheckSections adds a health check input section and a health check output section to the config.
func (firelens *FirelensResource) addHealthcheckSections(config generator.FluentConfig) {

}

func (firelens *FirelensResource) getS3ConfPath() string {
	return ""
}
