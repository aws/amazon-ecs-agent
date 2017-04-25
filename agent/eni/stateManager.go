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

package eni

import (
	"sync"

	log "github.com/cihub/seelog"
)

// stateChangeUnsafe exposes methods that are unsafe to use
// without grabbing locks in the correct order
type stateChangeUnsafe interface {
	AddDeviceWithMACAddressUnsafe(deviceName, macAddress string)
	RemoveDeviceWithMACAddressUnsafe(mac string)
	RemoveDeviceUnsafe(deviceName string)
}

type locks interface {
	Lock()
	Unlock()
}

type StateManagerInterface interface {
	GetAll() map[string]string
	stateChangeUnsafe
	locks
}

// stateManager holds the state of ENIs connected to the instance
type stateManager struct {
	updateLock sync.RWMutex
	enis       map[string]string // MAC => Device-Name
}

// NewStateManager returns a new ENIState object
func NewStateManager() StateManagerInterface {
	return &stateManager{
		enis: make(map[string]string, mapCapacityHint),
	}
}

// addDeviceWithMACAddressUnsafe adds new devices upon initialization
// NOTE: Expects lock to be held prior to update for correct semantics
func (e *stateManager) AddDeviceWithMACAddressUnsafe(deviceName, macAddress string) {
	log.Debugf("ENI state manager: adding device %s with MAC %s", deviceName, macAddress)
	// Update State
	e.enis[macAddress] = deviceName
}

// removeDeviceWithMACAddressUnsafe is used to remove new devices from maintained state
// NOTE: Expects lock to be held prior to update for correct semantics
func (e *stateManager) RemoveDeviceWithMACAddressUnsafe(mac string) {
	log.Debugf("ENI state manager: removing device with MACAddress: %s", mac)

	if _, ok := e.enis[mac]; !ok {
		log.Warnf("ENI state manager: device with MACAddress: %s missing from managed state", mac)
		return
	}

	delete(e.enis, mac)
}

// removeDeviceUnsafe is used to remove new devices from uDev events
// NOTE: removeDeviceUnsafe expects lock to be held prior to update for correct semantics
func (e *stateManager) RemoveDeviceUnsafe(deviceName string) {
	log.Debugf("ENI state manager: removing device: %s", deviceName)

	for mac, dev := range e.enis {
		if dev == deviceName {
			e.RemoveDeviceWithMACAddressUnsafe(mac)
			return
		}
	}
}

// getAllENIs is used to retrieve the state observed by the Watcher
func (e *stateManager) GetAll() map[string]string {
	return e.enis
}

// Lock grabs the RW lock for the protected state
func (e *stateManager) Lock() {
	e.updateLock.Lock()
	return
}

// Unlock releases the lock protecting the state
func (e *stateManager) Unlock() {
	e.updateLock.Unlock()
	return
}
