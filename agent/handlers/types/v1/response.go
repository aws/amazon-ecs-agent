// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package v1

// MetadataResponse is the schema for the metadata response JSON object
type MetadataResponse struct {
	Cluster              string
	ContainerInstanceArn *string
	Version              string
}

// TaskResponse is the schema for the task response JSON object
type TaskResponse struct {
	Arn           string
	DesiredStatus string `json:",omitempty"`
	KnownStatus   string
	Family        string
	Version       string
	Containers    []ContainerResponse
}

// TasksResponse is the schema for the tasks response JSON object
type TasksResponse struct {
	Tasks []*TaskResponse
}

// ContainerResponse is the schema for the container response JSON object
type ContainerResponse struct {
	DockerId   string
	DockerName string
	Name       string
}
