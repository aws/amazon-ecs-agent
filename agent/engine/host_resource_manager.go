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
	"fmt"
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

// TODO remove this once resource, consume are used
//lint:file-ignore U1000 Ignore all unused code

// initialHostResource keeps account of each task in
type HostResourceManager struct {
	initialHostResource       map[string]*ecs.Resource
	consumedResource          map[string]*ecs.Resource
	hostResourceManagerRWLock sync.Mutex

	//task.arn to boolean whether host resources consumed or not
	taskConsumed map[string]bool
}

// Returns if resources consumed or not and error status
// false, nil -> did not consume, should stay pending
// false, err -> resources map has errors, something went wrong
// true, nil -> successfully consumed
func (h *HostResourceManager) consume(taskArn string, resources map[string]*ecs.Resource) (bool, error) {
	h.hostResourceManagerRWLock.Lock()
	defer h.hostResourceManagerRWLock.Unlock()

	defer logger.Debug("Consumed resources after task consume call", logger.Fields{
		"taskArn":   taskArn,
		"CPU":       *h.consumedResource["CPU"].IntegerValue,
		"MEMORY":    *h.consumedResource["MEMORY"].IntegerValue,
		"PORTS_TCP": h.consumedResource["PORTS_TCP"].StringSetValue,
		"PORTS_UDP": h.consumedResource["PORTS_UDP"].StringSetValue,
		"GPU":       *h.consumedResource["GPU"].IntegerValue,
	})

	// Check if already consumed
	_, ok := h.consumedResource[taskArn]
	if ok {
		// Nothing to do, already consumed, return
		return true, nil
	}

	ok, err := h.consumable(resources)
	if err != nil {
		return false, err
	}
	if ok {
		// CPU
		cpu := *h.consumedResource["CPU"].IntegerValue + *resources["CPU"].IntegerValue
		h.consumedResource["CPU"].SetIntegerValue(cpu)

		// MEM
		mem := *h.consumedResource["MEMORY"].IntegerValue + *resources["MEMORY"].IntegerValue
		h.consumedResource["MEMORY"].SetIntegerValue(mem)

		// PORTS
		portsResource, ok := resources["PORTS_TCP"]
		if ok {
			taskPortsSlice := portsResource.StringSetValue
			for _, port := range taskPortsSlice {
				// Create a copy to assign it back as "PORTS_TCP"
				newPortResource := h.consumedResource["PORTS_TCP"]
				newPorts := append(h.consumedResource["PORTS_TCP"].StringSetValue, port)
				newPortResource.StringSetValue = newPorts
				h.consumedResource["PORTS_TCP"] = newPortResource
			}
		}

		// PORTS_UDP
		portsResource, ok = resources["PORTS_UDP"]
		if ok {
			taskPortsSlice := portsResource.StringSetValue
			for _, port := range taskPortsSlice {
				newPortResource := h.consumedResource["PORTS_UDP"]
				newPorts := append(h.consumedResource["PORTS_UDP"].StringSetValue, port)
				newPortResource.StringSetValue = newPorts
				h.consumedResource["PORTS_UDP"] = newPortResource
			}
		}

		// GPU
		*h.consumedResource["GPU"].IntegerValue += *resources["GPU"].IntegerValue

		// Set consumed status
		h.taskConsumed[taskArn] = true
		return true, nil
	}
	return false, nil
}

// Helper function for consume to check if resources are consumable with the current account
// we have for the host resources. Should not call host resource manager lock in this func
// return values
func (h *HostResourceManager) consumable(resources map[string]*ecs.Resource) (bool, error) {
	// CPU
	cpuResource, ok := resources["CPU"]
	if ok {
		if *(h.initialHostResource["CPU"].IntegerValue) < *(h.consumedResource["CPU"].IntegerValue)+*(cpuResource.IntegerValue) {
			return false, nil
		}
	} else {
		return false, fmt.Errorf("No CPU in task resources")
	}

	// MEM
	memResource, ok := resources["MEMORY"]
	if ok {
		if *(h.initialHostResource["MEMORY"].IntegerValue) < *(h.consumedResource["MEMORY"].IntegerValue)+*(memResource.IntegerValue) {
			return false, nil
		}
	} else {
		return false, fmt.Errorf("No MEMORY in task resources")
	}

	// PORTS
	portsResource, ok := resources["PORTS_TCP"]
	if ok {
		taskPortsSlice := portsResource.StringSetValue
		// For each port in current resource object, check if it is already 'consumed'. This is
		// done by maintaining a list of consumed ports, i.e. list of all ports reserved by tasks
		// being accounted for by host resources manager.
		for _, port := range taskPortsSlice {
			for _, consumedPort := range h.consumedResource["PORTS_TCP"].StringSetValue {
				// If port is already reserved by some other task, this 'resources' object can not be consumed
				if *port == *consumedPort {
					return false, nil
				}
			}
		}
	}

	// PORTS_UDP
	portsUDPResource, ok := resources["PORTS_UDP"]
	if ok {
		taskPortsUDPSlice := portsUDPResource.StringSetValue
		// Same for UDP ports
		for _, port := range taskPortsUDPSlice {
			for _, consumedPort := range h.consumedResource["PORTS_UDP"].StringSetValue {
				if *port == *consumedPort {
					return false, nil
				}
			}
		}
	}
	// GPU
	gpuResouce, ok := resources["GPU"]
	if ok {
		if *(h.initialHostResource["GPU"].IntegerValue) < *(h.consumedResource["GPU"].IntegerValue)+*(gpuResouce.IntegerValue) {
			return false, nil
		}
	}
	return true, nil
}

func (h *HostResourceManager) release(taskArn string, resources map[string]*ecs.Resource) {
	h.hostResourceManagerRWLock.Lock()
	defer h.hostResourceManagerRWLock.Unlock()
	defer logger.Debug("Consumed resources after task release call", logger.Fields{
		"taskArn":   taskArn,
		"CPU":       *h.consumedResource["CPU"].IntegerValue,
		"MEMORY":    *h.consumedResource["MEMORY"].IntegerValue,
		"PORTS_TCP": h.consumedResource["PORTS_TCP"].StringSetValue,
		"PORTS_UDP": h.consumedResource["PORTS_UDP"].StringSetValue,
		"GPU":       *h.consumedResource["GPU"].IntegerValue,
	})
	if h.taskConsumed[taskArn] {
		// CPU
		*h.consumedResource["CPU"].IntegerValue -= *resources["CPU"].IntegerValue

		// MEM
		*h.consumedResource["MEMORY"].IntegerValue -= *resources["MEMORY"].IntegerValue

		// PORTS
		portsResource, ok := resources["PORTS_TCP"]
		if ok {
			taskPortsSlice := portsResource.StringSetValue
			// Start removing ports one by one
			for _, port := range taskPortsSlice {
				// Create a copy of ports slice, iterate and find the port index in this slice
				// then remove the port, create a new slice and assign it back to consumedResource["PORTS_TCP"].StringSetValue
				itrPortSlice := h.consumedResource["PORTS_TCP"].StringSetValue
				idx := 0
				for i, consumedPort := range itrPortSlice {
					if *consumedPort == *port {
						idx = i
					}
				}
				newPortSlice := append(itrPortSlice[:idx], itrPortSlice[idx+1:]...)
				h.consumedResource["PORTS_TCP"].StringSetValue = newPortSlice
			}
		}

		// PORTS_UDP
		portsResource, ok = resources["PORTS_UDP"]
		if ok {
			taskPortsSlice := portsResource.StringSetValue
			for _, port := range taskPortsSlice {
				itrPortSlice := h.consumedResource["PORTS_UDP"].StringSetValue
				idx := 0
				for i, consumedPort := range itrPortSlice {
					if *consumedPort == *port {
						idx = i
					}
				}
				newPortSlice := append(itrPortSlice[:idx], itrPortSlice[idx+1:]...)
				h.consumedResource["PORTS_UDP"].StringSetValue = newPortSlice
			}
		}

		// GPU
		*h.consumedResource["GPU"].IntegerValue -= *resources["GPU"].IntegerValue

		// Set consumed status
		delete(h.taskConsumed, taskArn)
	}
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
