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
	"github.com/aws/amazon-ecs-agent/agent/logger/field"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

// TODO remove this once resource, consume are used
//lint:file-ignore U1000 Ignore all unused code

const (
	CPU      = "CPU"
	GPU      = "GPU"
	MEMORY   = "MEMORY"
	PORTSTCP = "PORTS_TCP"
	PORTSUDP = "PORTS_UDP"
)

// HostResourceManager keeps account of host resources allocated for tasks set to be created/running tasks
type HostResourceManager struct {
	initialHostResource       map[string]*ecs.Resource
	consumedResource          map[string]*ecs.Resource
	hostResourceManagerRWLock sync.Mutex

	//task.arn to boolean whether host resources consumed or not
	taskConsumed map[string]bool
}

type ResourceNotFoundForTask struct {
	resource string
}

func (e *ResourceNotFoundForTask) Error() string {
	return fmt.Sprintf("no %s in task resources", e.resource)
}

type ResourceIsNilForTask struct {
	resource string
}

func (e *ResourceIsNilForTask) Error() string {
	return fmt.Sprintf("resource %s is nil in task resources", e.resource)
}

func (h *HostResourceManager) logResources(msg string, taskArn string) {
	logger.Debug(msg, logger.Fields{
		"taskArn":   taskArn,
		"CPU":       *h.consumedResource[CPU].IntegerValue,
		"MEMORY":    *h.consumedResource[MEMORY].IntegerValue,
		"PORTS_TCP": h.consumedResource[PORTSTCP].StringSetValue,
		"PORTS_UDP": h.consumedResource[PORTSUDP].StringSetValue,
		"GPU":       *h.consumedResource[GPU].IntegerValue,
	})
}

// Returns if resources consumed or not and error status
// false, nil -> did not consume, task should stay pending
// false, err -> resources map has errors, task should fail as cannot schedule with 'wrong' resource map (this basically never happens)
// true, nil -> successfully consumed, task should progress with task creation
func (h *HostResourceManager) consume(taskArn string, resources map[string]*ecs.Resource) (bool, error) {
	h.hostResourceManagerRWLock.Lock()
	defer h.hostResourceManagerRWLock.Unlock()
	defer h.logResources("Consumed resources after task consume call", taskArn)

	// Check if already consumed
	_, ok := h.taskConsumed[taskArn]
	if ok {
		// Nothing to do, already consumed, return
		logger.Info("Resources pre-consumed, continue to task creation", logger.Fields{"taskArn": taskArn})
		return true, nil
	}

	ok, err := h.consumable(resources)
	if err != nil {
		logger.Error("Resources failing to consume, error in task resources", logger.Fields{
			"taskArn":   taskArn,
			field.Error: err,
		})
		return false, err
	}
	if ok {
		// CPU
		cpu := *h.consumedResource[CPU].IntegerValue + *resources[CPU].IntegerValue
		h.consumedResource[CPU].SetIntegerValue(cpu)

		// MEM
		mem := *h.consumedResource[MEMORY].IntegerValue + *resources[MEMORY].IntegerValue
		h.consumedResource[MEMORY].SetIntegerValue(mem)

		// PORTS
		portsResource, ok := resources[PORTSTCP]
		if ok {
			taskPortsSlice := portsResource.StringSetValue
			for _, port := range taskPortsSlice {
				// Create a copy to assign it back as "PORTS_TCP"
				newPortResource := h.consumedResource[PORTSTCP]
				newPorts := append(h.consumedResource[PORTSTCP].StringSetValue, port)
				newPortResource.StringSetValue = newPorts
				h.consumedResource[PORTSTCP] = newPortResource
			}
		}

		// PORTS_UDP
		portsResource, ok = resources[PORTSUDP]
		if ok {
			taskPortsSlice := portsResource.StringSetValue
			for _, port := range taskPortsSlice {
				newPortResource := h.consumedResource[PORTSUDP]
				newPorts := append(h.consumedResource[PORTSUDP].StringSetValue, port)
				newPortResource.StringSetValue = newPorts
				h.consumedResource[PORTSUDP] = newPortResource
			}
		}

		// GPU
		*h.consumedResource[GPU].IntegerValue += *resources[GPU].IntegerValue

		// Set consumed status
		h.taskConsumed[taskArn] = true
		logger.Info("Resources successfully consumed, continue to task creation", logger.Fields{"taskArn": taskArn})
		return true, nil
	}
	logger.Info("Resources not consumed, enough resources not available", logger.Fields{"taskArn": taskArn})
	return false, nil
}

// Functions checkConsumableIntType and checkConsumableStringSetType to be called
// only after checking for resource map health
func (h *HostResourceManager) checkConsumableIntType(resourceName string, resources map[string]*ecs.Resource) bool {
	resourceConsumableStatus := *(h.initialHostResource[resourceName].IntegerValue) >= *(h.consumedResource[resourceName].IntegerValue)+*(resources[resourceName].IntegerValue)
	return resourceConsumableStatus
}

func (h *HostResourceManager) checkConsumableStringSetType(resourceName string, resources map[string]*ecs.Resource) bool {
	resourceSlice := resources[resourceName].StringSetValue
	// Check intersection of resource StringSetValue is empty with consumedResource
	for _, obj1 := range resourceSlice {
		for _, obj2 := range h.consumedResource[resourceName].StringSetValue {
			// If port is already reserved by some other task, this 'resources' object can not be consumed
			if *obj1 == *obj2 {
				return false
			}
		}
	}
	return true
}

func checkResourceExistsInt(resourceName string, resources map[string]*ecs.Resource) error {
	_, ok := resources[resourceName]
	if ok {
		if resources[resourceName].IntegerValue == nil {
			return &ResourceIsNilForTask{resourceName}
		}
	} else {
		return &ResourceNotFoundForTask{resourceName}
	}
	return nil
}

func checkResourceExistsStringSet(resourceName string, resources map[string]*ecs.Resource) error {
	_, ok := resources[resourceName]
	if ok {
		for _, obj := range resources[resourceName].StringSetValue {
			if obj == nil {
				return &ResourceIsNilForTask{resourceName}
			}
		}
	} else {
		return &ResourceNotFoundForTask{resourceName}
	}
	return nil
}

// Checks all resources exists and their values are not nil
func checkResourcesHealth(resources map[string]*ecs.Resource) error {
	// CPU
	errCpu := checkResourceExistsInt(CPU, resources)
	if errCpu != nil {
		return errCpu
	}

	// MEMORY
	errMemory := checkResourceExistsInt(MEMORY, resources)
	if errMemory != nil {
		return errMemory
	}

	// PORTS_TCP
	errPortsTcp := checkResourceExistsStringSet(PORTSTCP, resources)
	if errPortsTcp != nil {
		return errPortsTcp
	}

	// PORTS_UDP
	errPortsUdp := checkResourceExistsStringSet(PORTSUDP, resources)
	if errPortsUdp != nil {
		return errPortsUdp
	}

	// GPU
	errGpu := checkResourceExistsInt(GPU, resources)
	if errGpu != nil {
		return errGpu
	}
	return nil
}

// Helper function for consume to check if resources are consumable with the current account
// we have for the host resources. Should not call host resource manager lock in this func
// return values
func (h *HostResourceManager) consumable(resources map[string]*ecs.Resource) (bool, error) {
	err := checkResourcesHealth(resources)
	if err != nil {
		return false, err
	}

	// CPU
	cpuConsumable := h.checkConsumableIntType(CPU, resources)
	if !cpuConsumable {
		return false, nil
	}

	// MEM
	memConsumable := h.checkConsumableIntType(MEMORY, resources)
	if !memConsumable {
		return false, nil
	}

	// PORTS
	portsTcpConsumable := h.checkConsumableStringSetType(PORTSTCP, resources)
	if !portsTcpConsumable {
		return false, nil
	}

	// PORTS_UDP
	portsUdpConsumable := h.checkConsumableStringSetType(PORTSUDP, resources)
	if !portsUdpConsumable {
		return false, nil
	}

	// GPU
	gpuConsumable := h.checkConsumableIntType(GPU, resources)
	if !gpuConsumable {
		return false, nil
	}

	return true, nil
}

// Utility function to manage release of ports
// s2 is contiguous sub slice of s1, each is unique (ports)
// returns a slice after removing s2 from s1, if found
func removeSubSlice(s1 []*string, s2 []*string) []*string {
	begin := 0
	end := len(s1) - 1
	for ; begin < len(s1); begin++ {
		if *s1[begin] == *s2[0] {
			break
		}
	}
	// no intersection found
	if begin == len(s1) {
		return s1
	}

	for ; end >= 0; end-- {
		if *s1[end] == *s2[len(s2)-1] {
			break
		}
	}
	newSlice := append(s1[:begin], s1[end+1:]...)
	return newSlice
}

// Returns error if task resource map has error, else releases resources
// Task resource map should never have errors as it is made by task ToHostResources method
// In cases releases fails due to errors, those resources will be failed to be released
// by HostResourceManager
func (h *HostResourceManager) release(taskArn string, resources map[string]*ecs.Resource) error {
	h.hostResourceManagerRWLock.Lock()
	defer h.hostResourceManagerRWLock.Unlock()
	defer h.logResources("Consumed resources after task release call", taskArn)

	if h.taskConsumed[taskArn] {
		err := checkResourcesHealth(resources)
		if err != nil {
			return err
		}
		// CPU
		*h.consumedResource[CPU].IntegerValue -= *resources[CPU].IntegerValue

		// MEM
		*h.consumedResource[MEMORY].IntegerValue -= *resources[MEMORY].IntegerValue

		// PORTS_TCP
		newPortsTcp := removeSubSlice(h.consumedResource[PORTSTCP].StringSetValue, resources[PORTSTCP].StringSetValue)
		h.consumedResource[PORTSTCP].StringSetValue = newPortsTcp

		// PORTS_UDP
		newPortsUdp := removeSubSlice(h.consumedResource[PORTSUDP].StringSetValue, resources[PORTSUDP].StringSetValue)
		h.consumedResource[PORTSUDP].StringSetValue = newPortsUdp

		// GPU
		*h.consumedResource[GPU].IntegerValue -= *resources[GPU].IntegerValue

		// Set consumed status
		delete(h.taskConsumed, taskArn)
	}
	return nil
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
	consumedResourceMap[CPU] = &ecs.Resource{
		Name:         utils.Strptr(CPU),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: &CPUs,
	}
	//MEMORY
	memory := int64(0)
	consumedResourceMap[MEMORY] = &ecs.Resource{
		Name:         utils.Strptr(MEMORY),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: &memory,
	}
	//PORTS_TCP
	//Copying ports from host resources as consumed ports for initializing
	portsTcp := []*string{}
	if resourceMap != nil && resourceMap[PORTSTCP] != nil {
		portsTcp = resourceMap[PORTSTCP].StringSetValue
	}
	consumedResourceMap[PORTSTCP] = &ecs.Resource{
		Name:           utils.Strptr("PORTS_TCP"),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: portsTcp,
	}

	//PORTS_UDP
	portsUdp := []*string{}
	if resourceMap != nil && resourceMap[PORTSUDP] != nil {
		portsUdp = resourceMap[PORTSUDP].StringSetValue
	}
	consumedResourceMap[PORTSUDP] = &ecs.Resource{
		Name:           utils.Strptr(PORTSUDP),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: portsUdp,
	}

	//GPUs
	numGPUs := int64(0)
	consumedResourceMap[GPU] = &ecs.Resource{
		Name:         utils.Strptr(GPU),
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
