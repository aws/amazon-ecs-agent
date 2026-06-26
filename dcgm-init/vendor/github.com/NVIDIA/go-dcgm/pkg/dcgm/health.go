/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package dcgm

/*
#include "dcgm_agent.h"
#include "dcgm_structs.h"
*/
import "C"

import (
	"fmt"
	"math/rand"
	"unsafe"
)

// SystemWatch represents a health watch system and its status
type SystemWatch struct {
	// Type identifies the type of health watch system
	Type string
	// Status indicates the current health status
	Status string
	// Error contains any error message if status is not healthy
	Error string
}

// DeviceHealth represents the health status of a GPU device
type DeviceHealth struct {
	// GPU is the ID of the GPU device
	GPU uint
	// Status indicates the overall health status of the GPU
	Status string
	// Watches contains the status of individual health watch systems
	Watches []SystemWatch
}

// HealthSet enables the DCGM health check system for the given systems.
// It configures which health watch systems should be monitored for the specified group.
func HealthSet(groupID GroupHandle, systems HealthSystem) (err error) {
	result := C.dcgmHealthSet(handle.handle, groupID.handle, C.dcgmHealthSystems_t(systems))
	if err := errorString(result); err != nil {
		return fmt.Errorf("error setting health watches: %w", err)
	}
	return nil
}

// HealthGet retrieves the current state of the DCGM health check system.
// It returns which health watch systems are currently enabled for the specified group.
func HealthGet(groupID GroupHandle) (HealthSystem, error) {
	var systems C.dcgmHealthSystems_t

	result := C.dcgmHealthGet(handle.handle, groupID.handle, (*C.dcgmHealthSystems_t)(unsafe.Pointer(&systems)))
	if err := errorString(result); err != nil {
		return HealthSystem(0), err
	}
	return HealthSystem(systems), nil
}

// DiagErrorDetail contains detailed information about a health check error
type DiagErrorDetail struct {
	// Message contains a human-readable description of the error
	Message string
	// Code identifies the specific type of error
	Code HealthCheckErrorCode
}

// Incident represents a health check incident that occurred
type Incident struct {
	// System identifies which health watch system detected the incident
	System HealthSystem
	// Health indicates the severity of the incident
	Health HealthResult
	// Error contains detailed information about the incident
	Error DiagErrorDetail
	// EntityInfo identifies the GPU or component where the incident occurred
	EntityInfo GroupEntityPair
}

// HealthResponse contains the results of a health check operation
type HealthResponse struct {
	// OverallHealth indicates the aggregate health status across all watches
	OverallHealth HealthResult
	// Incidents contains details about any health issues detected
	Incidents []Incident
}

// HealthCheck checks the configured watches for any errors/failures/warnings that have occurred
// since the last time this check was invoked. On the first call, stateful information
// about all of the enabled watches within a group is created but no error results are
// provided. On subsequent calls, any error information will be returned.
func HealthCheck(groupID GroupHandle) (HealthResponse, error) {
	var healthResults C.dcgmHealthResponse_v5
	healthResults.version = makeVersion5(unsafe.Sizeof(healthResults))

	result := C.dcgmHealthCheck(handle.handle, groupID.handle, (*C.dcgmHealthResponse_t)(unsafe.Pointer(&healthResults)))

	if err := errorString(result); err != nil {
		return HealthResponse{}, &Error{msg: C.GoString(C.errorString(result)), Code: result}
	}

	response := HealthResponse{
		OverallHealth: HealthResult(healthResults.overallHealth),
	}

	// number of watches that encountered error/warning
	incidents := uint(healthResults.incidentCount)

	response.Incidents = make([]Incident, incidents)

	for i := uint(0); i < incidents; i++ {
		response.Incidents[i] = Incident{
			System: HealthSystem(healthResults.incidents[i].system),
			Health: HealthResult(healthResults.incidents[i].health),
			Error: DiagErrorDetail{
				Message: *stringPtr(&healthResults.incidents[i].error.msg[0]),
				Code:    HealthCheckErrorCode(healthResults.incidents[i].error.code),
			},
			EntityInfo: GroupEntityPair{
				EntityGroupId: Field_Entity_Group(healthResults.incidents[i].entityInfo.entityGroupId),
				EntityId:      uint(healthResults.incidents[i].entityInfo.entityId),
			},
		}
	}

	return response, nil
}

func healthCheckByGpuId(gpuID uint) (deviceHealth DeviceHealth, err error) {
	name := fmt.Sprintf("health%d", rand.Uint64())
	groupID, err := CreateGroup(name)
	if err != nil {
		return
	}

	err = AddToGroup(groupID, gpuID)
	if err != nil {
		return
	}

	err = HealthSet(groupID, DCGM_HEALTH_WATCH_ALL)
	if err != nil {
		return
	}

	result, err := HealthCheck(groupID)
	if err != nil {
		return
	}

	status := healthStatus(result.OverallHealth)

	// number of watches that encountered error/warning
	incidents := len(result.Incidents)
	watches := make([]SystemWatch, incidents)

	for j := 0; j < incidents; j++ {
		watches[j] = SystemWatch{
			Type:   systemWatch(result.Incidents[j].System),
			Status: healthStatus(result.Incidents[j].Health),
			Error:  result.Incidents[j].Error.Message,
		}
	}

	deviceHealth = DeviceHealth{
		GPU:     gpuID,
		Status:  status,
		Watches: watches,
	}
	_ = DestroyGroup(groupID)
	return
}

func healthStatus(status HealthResult) string {
	switch status {
	case 0:
		return "Healthy"
	case 10:
		return "Warning"
	case 20:
		return "Failure"
	}
	return "N/A"
}

func systemWatch(watch HealthSystem) string {
	switch watch {
	case 1:
		return "PCIe watches"
	case 2:
		return "NVLINK watches"
	case 4:
		return "Power Managemnt unit watches"
	case 8:
		return "Microcontroller unit watches"
	case 16:
		return "Memory watches"
	case 32:
		return "Streaming Multiprocessor watches"
	case 64:
		return "Inforom watches"
	case 128:
		return "Temperature watches"
	case 256:
		return "Power watches"
	case 512:
		return "Driver-related watches"
	}
	return "N/A"
}
