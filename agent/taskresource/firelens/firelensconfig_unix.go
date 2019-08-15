// +build linux
// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"fmt"

	"github.com/pkg/errors"

	generator "github.com/awslabs/go-config-generator-for-fluentd-and-fluentbit"
)

const (
	// FirelensConfigTypeFluentd is the type of a fluentd firelens container.
	FirelensConfigTypeFluentd = "fluentd"

	// FirelensConfigTypeFluentbit is the type of a fluentbit firelens container.
	FirelensConfigTypeFluentbit = "fluentbit"

	// socketInputNameFluentd is the name of the socket input plugin for fluentd.
	socketInputNameFluentd = "unix"

	// socketInputNameFluentbit is the name of the socket input plugin for fluentbit.
	socketInputNameFluentbit = "forward"

	// socketInputPathOptionFluentd is the key for specifying socket path for fluentd.
	socketInputPathOptionFluentd = "path"

	// socketInputPathOptionFluentbit is the key for specifying socket path for fluentbit.
	socketInputPathOptionFluentbit = "unix_path"

	// outputTypeLogOptionKeyFluentd is the key for the log option that specifies output plugin type for fluentd.
	outputTypeLogOptionKeyFluentd = "@type"

	// outputTypeLogOptionKeyFluentbit is the key for the log option that specifies output plugin type for fluentbit.
	outputTypeLogOptionKeyFluentbit = "Name"

	// includePatternKey is the key for include pattern.
	includePatternKey = "include-pattern"

	// excludePatternKey is the key for exclude pattern.
	excludePatternKey = "exclude-pattern"

	// socketPath is the path for socket file.
	socketPath = "/var/run/fluent.sock"

	// fluentTagOutputFormat is the format for the log tag, which is container name with "-firelens" appended
	fluentTagOutputFormat = "%s-firelens*"
)

// generateConfig generates a FluentConfig object that contains all necessary information to construct
// a fluentd or fluentbit config file for a firelens container.
func (firelens *FirelensResource) generateConfig() (generator.FluentConfig, error) {
	config := generator.New()

	// Specify log stream input, which is a unix socket that will be used for communication between the Firelens
	// container and other containers.
	var inputName, inputPathOption string
	if firelens.firelensConfigType == FirelensConfigTypeFluentd {
		inputName = socketInputNameFluentd
		inputPathOption = socketInputPathOptionFluentd
	} else {
		inputName = socketInputNameFluentbit
		inputPathOption = socketInputPathOptionFluentbit
	}
	config.AddInput(inputName, "", map[string]string{
		inputPathOption: socketPath,
	})

	if firelens.ecsMetadataEnabled {
		// Add ecs metadata fields to the log stream.
		config.AddFieldToRecord("ecs_cluster", firelens.cluster, "*").
			AddFieldToRecord("ecs_task_arn", firelens.taskARN, "*").
			AddFieldToRecord("ecs_task_definition", firelens.taskDefinition, "*")
		if firelens.ec2InstanceID != "" {
			config.AddFieldToRecord("ec2_instance_id", firelens.ec2InstanceID, "*")
		}
	}

	// Specify log stream output. Each container that uses the firelens container to stream logs
	// will have its own output section, with its own log options.
	for containerName, logOptions := range firelens.containerToLogOptions {
		tag := fmt.Sprintf(fluentTagOutputFormat, containerName) // Each output section is distinguished by a tag specific to a container.
		newConfig, err := addOutputSection(tag, firelens.firelensConfigType, logOptions, config)
		if err != nil {
			return nil, fmt.Errorf("unable to apply log options of container %s to firelens config: %v", containerName, err)
		}
		config = newConfig
	}

	return config, nil
}

// addOutputSection adds an output section to the firelens container's config that specifies how it routes another
// container's logs. It's constructed based on that container's log options.
// logOptions is a set of key-value pairs, which includes the following:
//     1. The name of the output plugin (required). For fluentd, the key is "@type", for fluentbit, the key is "Name".
//     2. include-pattern (optional): a regex specifying the logs to be included.
//     3. exclude-pattern (optional): a regex specifying the logs to be excluded.
//     4. All other key-value pairs are customer specified options for the plugin. They are unique for each plugin and
//        we don't check them.
func addOutputSection(tag, firelensConfigType string, logOptions map[string]string, config generator.FluentConfig) (generator.FluentConfig, error) {
	var outputKey string
	if firelensConfigType == FirelensConfigTypeFluentd {
		outputKey = outputTypeLogOptionKeyFluentd
	} else {
		outputKey = outputTypeLogOptionKeyFluentbit
	}

	output, ok := logOptions[outputKey]
	if !ok {
		return config, errors.New(
			fmt.Sprintf("missing output option %s which is required for firelens configuration of type %s",
				outputKey, firelensConfigType))
	}

	outputOptions := make(map[string]string)
	for key, value := range logOptions {
		switch key {
		case outputKey:
			continue
		case includePatternKey:
			config.AddIncludeFilter(value, "log", tag)
		case excludePatternKey:
			config.AddExcludeFilter(value, "log", tag)
		default: // This is a plugin specific option.
			outputOptions[key] = value
		}
	}

	config.AddOutput(output, tag, outputOptions)
	return config, nil
}
