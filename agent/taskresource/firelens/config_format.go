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

// Package firelens: this file has no build constraints because the external config file format needs to be
// determined both on platforms where firelens is implemented (linux) and where it isn't (windows), e.g. from
// agent/api/task package when building container bind mounts.
package firelens

import "strings"

const (
	// ExternalConfigValueOption is the option that specifies the location of the external config file. When
	// ExternalConfigTypeOption is s3, the value for this option should be an s3 arn; when ExternalConfigTypeOption is
	// file, the value for this option should be a path to the config file inside the firelens container.
	ExternalConfigValueOption = "config-file-value"

	// S3ConfigPathFluentbitYAML is the path where we bind mount a YAML formatted config downloaded from S3 for a
	// fluentbit firelens container. Fluent Bit determines the config file format (classic INI-style vs YAML) it
	// should use to parse a given file based on that file's extension, so YAML formatted external configs need to
	// be mounted at a path ending in ".yaml"/".yml" rather than at S3ConfigPathFluentbit.
	S3ConfigPathFluentbitYAML = "/fluent-bit/etc/external.yaml"
)

// IsYAMLExternalConfigValue returns true if the given external firelens config file path/ARN (the value of the
// "config-file-value" firelens option) indicates a YAML formatted Fluent Bit configuration file, as opposed to the
// classic INI-style format, based on its file extension. Fluent Bit itself uses this same extension based
// heuristic (".yaml"/".yml" vs everything else) to decide how to parse a config file, so we need to replicate it
// here in order to generate a wrapper config file and bind mounts that are consistent with what Fluent Bit expects.
func IsYAMLExternalConfigValue(value string) bool {
	lower := strings.ToLower(value)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}
