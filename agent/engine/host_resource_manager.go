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

	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

const (
	CPU      = "CPU"
	GPU      = "GPU"
	MEMORY   = "MEMORY"
	PORTSTCP = "PORTS_TCP"
	PORTSUDP = "PORTS_UDP"
)

// HostResourceManager keeps account of host resources allocated for tasks set to be created/running tasks
type HostResourceManager struct {
	initialHostResource       map[string]types.Resource
	consumedResource          map[string]types.Resource
	hostResourceManagerRWLock sync.Mutex

	//task.arn to boolean whether host resources consumed or not
	taskConsumed map[string]bool
}

type InvalidHostResource struct {
	resource string
}

func (e *InvalidHostResource) Error() string {
	return fmt.Sprintf("no %s resource found in host resources", e.resource)
}

func (h *HostResourceManager) logResources(msg string, taskArn string) {
	logger.Debug(msg, logger.Fields{
		"taskArn":   taskArn,
		"CPU":       h.consumedResource[CPU].IntegerValue,
		"MEMORY":    h.consumedResource[MEMORY].IntegerValue,
		"PORTS_TCP": h.consumedResource[PORTSTCP].StringSetValue,
		"PORTS_UDP": h.consumedResource[PORTSUDP].StringSetValue,
		"GPU":       h.consumedResource[GPU].StringSetValue,
	})
}

func (h *HostResourceManager) consumeIntType(resourceType string, resources map[string]types.Resource) {
	consumedResource := h.consumedResource[resourceType]
	consumedResource.IntegerValue += resources[resourceType].IntegerValue
	h.consumedResource[resourceType] = consumedResource
}

func (h *HostResourceManager) consumeStringSetType(resourceType string, resources map[string]types.Resource) {
	resource, ok := resources[resourceType]
	if ok {
		consumedResource := h.consumedResource[resourceType]
		consumedResource.StringSetValue = append(h.consumedResource[resourceType].StringSetValue, resource.StringSetValue...)
		h.consumedResource[resourceType] = consumedResource
	}
}

func (h *HostResourceManager) checkTaskConsumed(taskArn string) bool {
	h.hostResourceManagerRWLock.Lock()
	defer h.hostResourceManagerRWLock.Unlock()
	_, ok := h.taskConsumed[taskArn]
	return ok
}

// Returns if resources consumed or not and error status
// false, nil -> did not consume, task should stay pending
// false, err -> resources map has errors, task should fail as cannot schedule with 'wrong' resource map (this basically never happens)
// true, nil -> successfully consumed, task should progress with task creation
func (h *HostResourceManager) consume(taskArn string, resources map[string]types.Resource) (bool, error) {
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

	ok, failedResourceKeys, err := h.consumable(resources)
	if err != nil {
		logger.Error("Resources failing to consume, error in task resources", logger.Fields{
			"taskArn":   taskArn,
			field.Error: err,
		})
		return false, err
	}
	if ok {
		for resourceKey := range resources {
			if *resources[resourceKey].Type == "INTEGER" {
				// CPU, MEMORY
				h.consumeIntType(resourceKey, resources)
			} else if *resources[resourceKey].Type == "STRINGSET" {
				// PORTS_TCP, PORTS_UDP, GPU
				h.consumeStringSetType(resourceKey, resources)
			}
		}

		// Set consumed status
		h.taskConsumed[taskArn] = true
		logger.Info("Resources successfully consumed, continue to task creation", logger.Fields{"taskArn": taskArn})
		return true, nil
	}
	logger.Info("Resources not consumed, enough resources not available", logger.Fields{
		"taskArn":   taskArn,
		"resources": failedResourceKeys,
	})
	return false, nil
}

// Functions checkConsumableIntType and checkConsumableStringSetType to be called
// only after checking for resource map health
func (h *HostResourceManager) checkConsumableIntType(resourceName string, resources map[string]types.Resource) bool {
	resourceConsumableStatus := h.initialHostResource[resourceName].IntegerValue >= h.consumedResource[resourceName].IntegerValue+resources[resourceName].IntegerValue
	return resourceConsumableStatus
}

func (h *HostResourceManager) checkConsumableStringSetType(resourceName string, resources map[string]types.Resource) bool {
	resourceSlice := resources[resourceName].StringSetValue

	// (optimization) Get a resource specific map to ease look up
	resourceMap := make(map[string]struct{}, len(resourceSlice))
	for _, v := range resourceSlice {
		resourceMap[v] = struct{}{}
	}

	// Check intersection of resource StringSetValue is empty with consumedResource
	for _, obj1 := range h.consumedResource[resourceName].StringSetValue {
		_, ok := resourceMap[obj1]
		if ok {
			// If resource is already reserved by some other task, this 'resources' object can not be consumed
			return false
		}
	}
	return true
}

// Checks all resources exists and their values are not nil
func (h *HostResourceManager) checkResourcesHealth(resources map[string]types.Resource) error {
	for resourceKey, resourceVal := range resources {
		_, ok := h.initialHostResource[resourceKey]
		if !ok {
			logger.Error(fmt.Sprintf("resource %s not found in host resources", resourceKey))
			return &InvalidHostResource{resourceKey}
		}

		// CPU, MEMORY are INTEGER;
		// PORTS_TCP, PORTS_UDP, GPU are STRINGSET
		// Check if either of these data types exist
		if resourceVal.Type == nil || !(*resourceVal.Type == "INTEGER" || *resourceVal.Type == "STRINGSET") {
			logger.Error(fmt.Sprintf("type not assigned for resource %s", resourceKey))
			return fmt.Errorf("invalid resource type for %s", resourceKey)
		}

		// Verify resource comes from an existing pool of values - for valid gpu ids
		if *resourceVal.Type == "STRINGSET" && resourceKey == GPU {
			if *resourceVal.Type != "STRINGSET" {
				return fmt.Errorf("resource gpu must be STRINGSET type")
			}

			hostGpuMap := make(map[string]struct{}, len(h.initialHostResource[GPU].StringSetValue))
			for _, v := range h.initialHostResource[GPU].StringSetValue {
				hostGpuMap[v] = struct{}{}
			}
			for _, obj1 := range resourceVal.StringSetValue {
				_, ok := hostGpuMap[obj1]
				if !ok {
					return fmt.Errorf("task gpu %s not found in host gpus", obj1)
				}
			}
		}
	}
	return nil
}

// Helper function for consume to check if resources are consumable with the current account
// we have for the host resources. Should not call host resource manager lock in this func return values
// This function returns a bool (indicating whether ALL requested resources are consumable), a list of non-consumable
// resource keys, and error, if any.
func (h *HostResourceManager) consumable(resources map[string]types.Resource) (bool, []string, error) {
	err := h.checkResourcesHealth(resources)
	if err != nil {
		return false, nil, err
	}

	var resourcesNotConsumable []string
	for resourceKey := range resources {
		if *resources[resourceKey].Type == "INTEGER" {
			consumable := h.checkConsumableIntType(resourceKey, resources)
			if !consumable {
				resourcesNotConsumable = append(resourcesNotConsumable, resourceKey)
			}
		}

		if *resources[resourceKey].Type == "STRINGSET" {
			consumable := h.checkConsumableStringSetType(resourceKey, resources)
			if !consumable {
				resourcesNotConsumable = append(resourcesNotConsumable, resourceKey)
			}
		}
	}

	if resourcesNotConsumable != nil {
		return false, resourcesNotConsumable, nil
	} else {
		return true, nil, nil
	}
}

// Utility function to manage release of ports
// s2 is contiguous sub slice of s1, each is unique (ports)
// returns a slice after removing s2 from s1, if found
func removeSubSlice(s1 []string, s2 []string) []string {
	begin := 0
	end := len(s1) - 1
	if len(s2) == 0 {
		return s1
	}
	for ; begin < len(s1); begin++ {
		if s1[begin] == s2[0] {
			break
		}
	}
	// no intersection found
	if begin == len(s1) {
		return s1
	}

	end = begin + len(s2)
	newSlice := append(s1[:begin], s1[end:]...)
	return newSlice
}

func (h *HostResourceManager) releaseIntType(resourceType string, resources map[string]types.Resource) {
	consumedResource := h.consumedResource[resourceType]
	consumedResource.IntegerValue -= resources[resourceType].IntegerValue
	h.consumedResource[resourceType] = consumedResource
}

func (h *HostResourceManager) releaseStringSetType(resourceType string, resources map[string]types.Resource) {
	newSlice := removeSubSlice(h.consumedResource[resourceType].StringSetValue, resources[resourceType].StringSetValue)
	consumedResource := h.consumedResource[resourceType]
	consumedResource.StringSetValue = newSlice
	h.consumedResource[resourceType] = consumedResource
}

// Returns error if task resource map has error, else releases resources
// Task resource map should never have errors as it is made by task ToHostResources method
// In cases releases fails due to errors, those resources will be failed to be released
// by HostResourceManager
func (h *HostResourceManager) release(taskArn string, resources map[string]types.Resource) error {
	h.hostResourceManagerRWLock.Lock()
	defer h.hostResourceManagerRWLock.Unlock()
	defer h.logResources("Consumed resources after task release call", taskArn)

	if h.taskConsumed[taskArn] {
		err := h.checkResourcesHealth(resources)
		if err != nil {
			return err
		}

		for resourceKey := range resources {
			if *resources[resourceKey].Type == "INTEGER" {
				h.releaseIntType(resourceKey, resources)
			}
			if *resources[resourceKey].Type == "STRINGSET" {
				h.releaseStringSetType(resourceKey, resources)
			}
		}

		// Set consumed status
		delete(h.taskConsumed, taskArn)
	}
	return nil
}

// NewHostResourceManager initialize host resource manager with available host resource values
func NewHostResourceManager(resourceMap map[string]types.Resource) HostResourceManager {
	// for resources in resourceMap, some are "available resources" like CPU, mem, while
	// some others are "reserved/consumed resources" like ports
	consumedResourceMap := make(map[string]types.Resource)
	taskConsumed := make(map[string]bool)
	// assigns CPU, MEMORY, PORTS_TCP, PORTS_UDP from host
	// CPU
	CPUs := int32(0)
	consumedResourceMap[CPU] = types.Resource{
		Name:         utils.Strptr(CPU),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: CPUs,
	}
	// MEMORY
	memory := int32(0)
	consumedResourceMap[MEMORY] = types.Resource{
		Name:         utils.Strptr(MEMORY),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: memory,
	}
	// PORTS_TCP
	// Copying ports from host resources as consumed ports for initializing
	portsTcp := []string{}
	if resourceMap != nil {
		if _, ok := resourceMap[PORTSTCP]; ok {
			portsTcp = resourceMap[PORTSTCP].StringSetValue
		}
	}
	consumedResourceMap[PORTSTCP] = types.Resource{
		Name:           utils.Strptr(PORTSTCP),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: portsTcp,
	}

	// PORTS_UDP
	portsUdp := []string{}
	if resourceMap != nil {
		if _, ok := resourceMap[PORTSUDP]; ok {
			portsUdp = resourceMap[PORTSUDP].StringSetValue
		}
	}
	consumedResourceMap[PORTSUDP] = types.Resource{
		Name:           utils.Strptr(PORTSUDP),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: portsUdp,
	}

	// GPUs
	gpuIDs := []string{}
	consumedResourceMap[GPU] = types.Resource{
		Name:           utils.Strptr(GPU),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: gpuIDs,
	}

	logger.Info("Initializing host resource manager, initialHostResource", logger.Fields{"initialHostResource": resourceMap})
	logger.Info("Initializing host resource manager, consumed resource", logger.Fields{"consumedResource": consumedResourceMap})
	return HostResourceManager{
		initialHostResource: resourceMap,
		consumedResource:    consumedResourceMap,
		taskConsumed:        taskConsumed,
	}
}
