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

package v4

import "github.com/aws/amazon-ecs-agent/agent/handlers/utils"

const (
	// metadataContainerIDMuxName is the key that's used in gorilla/mux to get the container ID
	// for container metadata.
	metadataContainerIDMuxName = "metadataContainerIDMuxName"

	// TaskMetadataPath specifies the relative URI path for serving task metadata.
	TaskMetadataPath = "/v4/metadata"

	// TaskWithTagsMetadataPath specifies the relative URI path for serving task metadata with Container Instance and Task Tags.
	TaskWithTagsMetadataPath = "/v4/metadataWithTags"

	// TaskMetadataPathWithSlash specifies the relative URI path for serving task metadata.
	TaskMetadataPathWithSlash = TaskMetadataPath + "/"

	// TaskWithTagsMetadataPathWithSlash specifies the relative URI path for serving task metadata with Container Instance and Task Tags.
	TaskWithTagsMetadataPathWithSlash = TaskWithTagsMetadataPath + "/"
)

// ContainerMetadataPath specifies the relative URI path for serving container metadata.
var ContainerMetadataPath = TaskMetadataPathWithSlash + utils.ConstructMuxVar(metadataContainerIDMuxName, utils.AnythingButEmptyRegEx)
