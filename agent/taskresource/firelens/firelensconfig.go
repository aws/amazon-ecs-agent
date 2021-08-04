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
	"fmt"

	"github.com/pkg/errors"

	generator "github.com/awslabs/go-config-generator-for-fluentd-and-fluentbit"
)

const (
	// FirelensConfigTypeFluentd is the type of a fluentd firelens container.
	FirelensConfigTypeFluentd = "fluentd"

	// FirelensConfigTypeFluentbit is the type of a fluentbit firelens container.
	FirelensConfigTypeFluentbit = "fluentbit"

	// outputTypeLogOptionKeyFluentd is the key for the log option that specifies output plugin type for fluentd.
	outputTypeLogOptionKeyFluentd = "@type"

	// outputTypeLogOptionKeyFluentbit is the key for the log option that specifies output plugin type for fluentbit.
	outputTypeLogOptionKeyFluentbit = "Name"

	// includePatternKey is the key for include pattern.
	includePatternKey = "include-pattern"

	// excludePatternKey is the key for exclude pattern.
	excludePatternKey = "exclude-pattern"
)

// addOutputSection adds an output section to the firelens container's config that specifies how it routes another
// container's logs. It's constructed based on that container's log options.
// logOptions is a set of key-value pairs, which includes the following:
//     1. The name of the output plugin (required when there are output options specified, i.e. the ones in 4). For
//     fluentd, the key is "@type", for fluentbit, the key is "Name".
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

	output, ok := logOptions[outputKey]
	// If there are some output options specified, there must be an output key so that we know what is the output plugin.
	if len(outputOptions) > 0 && !ok {
		return config, errors.New(
			fmt.Sprintf("missing output key %s which is required for firelens configuration of type %s",
				outputKey, firelensConfigType))
	} else if !ok { // Otherwise it's ok to not generate an output section, since customers may specify the output in external config.
		return config, nil
	}

	// Output key is specified. Add an output section.
	config.AddOutput(output, tag, outputOptions)
	return config, nil
}
