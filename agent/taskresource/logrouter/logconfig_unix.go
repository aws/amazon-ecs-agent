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

package logrouter

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	generator "github.com/awslabs/go-config-generator-for-fluentd-and-fluentbit"
)

const (
	// LogRouterTypeFluentd is the type of a fluentd log router.
	LogRouterTypeFluentd = "fluentd"

	// LogRouterTypeFluentbit is the type of a fluentbit log router.
	LogRouterTypeFluentbit = "fluentbit"

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
)

// generateConfig generates a FluentConfig object that contains all necessary information to construct
// a fluentd or fluentbit config file for the log router.
func (logRouter *LogRouterResource) generateConfig() (generator.FluentConfig, error) {
	config := generator.New()

	// Specify log stream input, which is a unix socket that will be used for communication between log router container
	// and other containers.
	var inputName, inputPathOption string
	if logRouter.logRouterType == LogRouterTypeFluentd {
		inputName = socketInputNameFluentd
		inputPathOption = socketInputPathOptionFluentd
	} else {
		inputName = socketInputNameFluentbit
		inputPathOption = socketInputPathOptionFluentbit
	}
	config.AddInput(inputName, "", map[string]string{
		inputPathOption: socketPath,
	})

	if logRouter.ecsMetadataEnabled {
		// Add ecs metadata fields to the log stream.
		config.AddFieldToRecord("ecs_cluster", logRouter.cluster, "*").
			AddFieldToRecord("ecs_task_arn", logRouter.taskARN, "*").
			AddFieldToRecord("ecs_task_definition", logRouter.taskDefinition, "*")
		if logRouter.ec2InstanceID != "" {
			config.AddFieldToRecord("ec2_instance_id", logRouter.ec2InstanceID, "*")
		}
	}

	// Specify log stream output. Each container that uses the log router container to stream logs
	// will have its own output section, with its own log options.
	fields := strings.Split(logRouter.taskARN, "/")
	taskID := fields[len(fields)-1]
	for containerName, logOptions := range logRouter.containerToLogOptions {
		tag := containerName + "-" + taskID // Each output section is distinguished by a tag specific to a container.
		newConfig, err := addOutputSection(tag, logRouter.logRouterType, logOptions, config)
		if err != nil {
			return nil, fmt.Errorf("unable to apply log options of container %s to log router config: %v", containerName, err)
		}
		config = newConfig
	}

	return config, nil
}

// addOutputSection adds an output section in the log config for a container. It's constructed based on the
// container's log options.
// logOptions is a set of key-value pairs, which includes the following:
//     1. The name of the output plugin (required). For fluentd, the key is "@type", for fluentbit, the key is "Name".
//     2. include-pattern (optional): a regex specifying the logs to be included.
//     3. exclude-pattern (optional): a regex specifying the logs to be excluded.
//     4. All other key-value pairs are customer specified options for the plugin. They are unique for each plugin and
//        we don't check them.
func addOutputSection(tag, logRouterType string, logOptions map[string]string, config generator.FluentConfig) (generator.FluentConfig, error) {
	var outputKey string
	if logRouterType == LogRouterTypeFluentd {
		outputKey = outputTypeLogOptionKeyFluentd
	} else {
		outputKey = outputTypeLogOptionKeyFluentbit
	}

	output, ok := logOptions[outputKey]
	if !ok {
		return config, errors.New(
			fmt.Sprintf("missing output option %s which is required for log router type %s",
				outputKey, logRouterType))
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
