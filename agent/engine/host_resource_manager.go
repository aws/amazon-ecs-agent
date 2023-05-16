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

// initialHostResource keeps account of each task in
type HostResourceManager struct {
	initialHostResource map[string]*ecs.Resource
	consumedResource    map[string]*ecs.Resource

	//task.arn to boolean whether host resources consumed or not
	taskConsumed map[string]bool
}

// NewHostResourceManager initialize host resource manager with available host resource values
func NewHostResourceManager(resourceMap map[string]*ecs.Resource) HostResourceManager {
	// for resources in resourceMap, some are "available resources" like CPU, mem, while
	// some others are "reserved/consumed resources" like ports
	consumedResourceMap := make(map[string]*ecs.Resource)
	taskConsumed := make(map[string]bool)
	// assigns CPU, MEMORY, PORTS_TCP, PORTS_UDP from host
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
	//PORTS_TCP
	//Copying ports from host resources as consumed ports for initializing
	portsTcp := []*string{}
	if resourceMap != nil && resourceMap["PORTS_TCP"] != nil {
		portsTcp = resourceMap["PORTS_TCP"].StringSetValue
	}
	consumedResourceMap["PORTS_TCP"] = &ecs.Resource{
		Name:           utils.Strptr("PORTS_TCP"),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: portsTcp,
	}

	//PORTS_UDP
	portsUdp := []*string{}
	if resourceMap != nil && resourceMap["PORTS_UDP"] != nil {
		portsUdp = resourceMap["PORTS_UDP"].StringSetValue
	}
	consumedResourceMap["PORTS_UDP"] = &ecs.Resource{
		Name:           utils.Strptr("PORTS_UDP"),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: portsUdp,
	}

	//GPUs
	numGPUs := int64(0)
	consumedResourceMap["GPU"] = &ecs.Resource{
		Name:         utils.Strptr("GPU"),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: &numGPUs,
	}

	logger.Info("Initializing host resource manager, initialHostResource", logger.Fields{"initialHostResource": resourceMap})
	logger.Info("Initializing host resource manager, consumed resource", logger.Fields{"consumedResource": consumedResourceMap})
	return HostResourceManager{
		initialHostResource: resourceMap,
		consumedResource:    consumedResourceMap,
		taskConsumed:        taskConsumed,
	}
}
