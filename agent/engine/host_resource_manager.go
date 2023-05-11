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

// Package engine contains the core logic for managing tasks

package engine

import (
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

// HostResourceManager keeps account of each task in
type HostResourceManager struct {
	hostResource     map[string]*ecs.Resource
	consumedResource map[string]*ecs.Resource

	taskConsumed map[string]bool //task.arn to boolean whether host resources consumed or not
}

func NewHostResourceManager(resourceMap map[string]*ecs.Resource) HostResourceManager {
	consumedResourceMap := make(map[string]*ecs.Resource)
	taskConsumed := make(map[string]bool)
	// assigns CPU, MEMORY, PORTS, PORTS_UDP from host
	//CPU
	CPUs := int64(0)
	consumedResourceMap["CPU"] = &ecs.Resource{
		Name:         utils.Strptr("CPU"),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: &CPUs,
	}
	//MEMORY
	memory := int64(0)
	consumedResourceMap["MEMORY"] = &ecs.Resource{
		Name:         utils.Strptr("MEMORY"),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: &memory,
	}
	//PORTS
	//Copying ports from host resources as consumed ports for initializing
	consumedResourceMap["PORTS"] = &ecs.Resource{
		Name:           utils.Strptr("PORTS"),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: resourceMap["PORTS"].StringSetValue,
	}

	//PORTS_UDP
	consumedResourceMap["PORTS_UDP"] = &ecs.Resource{
		Name:           utils.Strptr("PORTS_UDP"),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: resourceMap["PORTS_UDP"].StringSetValue,
	}

	//GPUs
	numGPUs := int64(0)
	consumedResourceMap["GPU"] = &ecs.Resource{
		Name:         utils.Strptr("GPU"),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: &numGPUs,
	}

	logger.Info("Initializing host resource manager, host resource", logger.Fields{"hostResource": resourceMap})
	logger.Info("Initializing host resource manager, consumed resource", logger.Fields{"consumedResource": consumedResourceMap})
	return HostResourceManager{
		hostResource:     resourceMap,
		consumedResource: consumedResourceMap,
		taskConsumed:     taskConsumed,
	}
}
