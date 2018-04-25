// Copyright 2017-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package container

import (
	"encoding/json"

	"github.com/aws/amazon-ecs-agent/agent/taskresource"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// TransitionDependencySet contains dependencies that impact transitions of
// containers.
type TransitionDependencySet struct {
	// ContainerDependencies is the set of containers on which a transition is
	// dependent.
	ContainerDependencies []ContainerDependency `json:"ContainerDependencies"`
	// ResourceDependencies is the set of resources on which a transition is
	// dependent.
	ResourceDependencies []ResourceDependency `json:"ResourceDependencies"`
}

// ContainerDependency defines the relationship between a dependent container
// and its dependency.
type ContainerDependency struct {
	// ContainerName defines the container on which a transition depends
	ContainerName string `json:"ContainerName"`
	// SatisfiedStatus defines the status that satisfies the dependency
	SatisfiedStatus ContainerStatus `json:"SatisfiedStatus"`
	// DependentStatus defines the status that cannot be reached until the
	// resource satisfies the dependency
	DependentStatus ContainerStatus `json:"DependentStatus,omitempty"`
}

// ResourceDependency defines the relationship between a dependent container
// and its resource dependency.
type ResourceDependency struct {
	// Name defines the Resource on which a transition depends
	Name string `json:"Name"`
	// RequiredStatus defines the status that satisfies the dependency
	RequiredStatus taskresource.ResourceStatus `json:"RequiredStatus"`
}

// GetRequiredStatus returns the required status for the dependency
func (rd *ResourceDependency) GetRequiredStatus() taskresource.ResourceStatus {
	return rd.RequiredStatus
}

// TransitionDependenciesMap is a map of the dependent container status to other
// dependencies that must be satisfied.
type TransitionDependenciesMap map[ContainerStatus]TransitionDependencySet

// UnmarshalJSON decodes the TransitionDependencySet tag in the JSON encoded string
// into the TransitionDependenciesMap object
func (td *TransitionDependenciesMap) UnmarshalJSON(b []byte) error {
	depMap := make(map[ContainerStatus]TransitionDependencySet)
	err := json.Unmarshal(b, &depMap)
	if err == nil {
		*td = depMap
		return nil
	}
	seelog.Debugf("Unmarshal 'TransitionDependencySet': %s, not a map: %v", string(b), err)
	// Unmarshal to deprecated 'TransitionDependencySet' and then convert to a map
	tdSet := TransitionDependencySet{}
	if err := json.Unmarshal(b, &tdSet); err != nil {
		return errors.Wrapf(err,
			"Unmarshal 'TransitionDependencySet': does not comply with any of the dependency types")
	}
	for _, dep := range tdSet.ContainerDependencies {
		dependentStatus := dep.DependentStatus
		// no need for DependentStatus field anymore, since it becomes the map's key
		dep.DependentStatus = 0
		if _, ok := depMap[dependentStatus]; !ok {
			depMap[dependentStatus] = TransitionDependencySet{}
		}
		deps := depMap[dependentStatus]
		deps.ContainerDependencies = append(deps.ContainerDependencies, dep)
		depMap[dependentStatus] = deps
	}
	*td = depMap
	return nil
}
