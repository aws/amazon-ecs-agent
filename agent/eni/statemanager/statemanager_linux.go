// +build linux

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

package statemanager

import (
	"sync"

	log "github.com/cihub/seelog"
	"github.com/vishvananda/netlink"
)

const mapCapacityHint = 10 // mapCapacityHint is a nominal hint for map capacity

// stateChangeUnsafe exposes methods that are unsafe to use
// without grabbing locks in the correct order
type stateChangeUnsafe interface {
	AddDeviceWithMACAddressUnsafe(deviceName, macAddress string)
	RemoveDeviceWithMACAddressUnsafe(mac string)
	RemoveDeviceUnsafe(deviceName string)
}

// StateManager interface incorporates stateChangeUnsafe and adds
// additional methods to lock, unlock and retrieve all state
type StateManager interface {
	stateChangeUnsafe
	GetAll() map[string]string
	Init(state []netlink.Link)
	Reconcile(currentState map[string]string)
	AddDeviceWithMACAddress(deviceName, macAddress string)
	RemoveDeviceWithMACAddress(mac string)
	RemoveDevice(deviceName string)
	IsMACAddressPresent(macAddress string) bool
}

// stateManager holds the state of ENIs connected to the instance
type stateManager struct {
	updateLock sync.RWMutex
	enis       map[string]string // enis is a map of MAC => Device-Name
}

// New returns a new StateManager
func New() StateManager {
	return &stateManager{
		enis: make(map[string]string, mapCapacityHint),
	}
}

// Init populates the initial state of the map
func (statemanager *stateManager) Init(state []netlink.Link) {
	statemanager.updateLock.Lock()
	defer statemanager.updateLock.Unlock()
	for _, link := range state {
		deviceName := link.Attrs().Name
		macAddress := link.Attrs().HardwareAddr.String()
		if macAddress != "" {
			statemanager.AddDeviceWithMACAddressUnsafe(deviceName, macAddress)
		}
	}
}

// Reconcile performs a 2 phase reconciliation of managed state
func (statemanager *stateManager) Reconcile(currentState map[string]string) {
	statemanager.updateLock.Lock()
	defer statemanager.updateLock.Unlock()

	// Remove non-existent interfaces first
	for managedMACAddress, managedDeviceName := range statemanager.enis {
		if currentDeviceName, ok := currentState[managedMACAddress]; !ok || managedDeviceName != currentDeviceName {
			statemanager.RemoveDeviceWithMACAddressUnsafe(managedMACAddress)
		}
	}

	// Add new interfaces next
	for mac, dev := range currentState {
		if _, ok := statemanager.enis[mac]; !ok && mac != "" {
			statemanager.AddDeviceWithMACAddressUnsafe(dev, mac)
		}
	}
}

// GetAll is used to retrieve the state observed by the StateManager
func (statemanager *stateManager) GetAll() map[string]string {
	return statemanager.enis
}

// AddDeviceWithMACAddressUnsafe adds new devices upon initialization
// NOTE: Expects lock to be held prior to update for correct semantics
func (statemanager *stateManager) AddDeviceWithMACAddressUnsafe(deviceName, macAddress string) {
	log.Debugf("ENI state manager: adding device %s with MAC %s (unsafe)", deviceName, macAddress)
	// Update State
	statemanager.enis[macAddress] = deviceName
}

// RemoveDeviceWithMACAddressUnsafe is used to remove new devices from maintained state
// NOTE: Expects lock to be held prior to update for correct semantics
func (statemanager *stateManager) RemoveDeviceWithMACAddressUnsafe(mac string) {
	log.Debugf("ENI state manager: removing device with MACAddress: %s (unsafe)", mac)

	enis := statemanager.GetAll()
	if _, ok := enis[mac]; !ok {
		log.Warnf("ENI state manager: device with MACAddress: %s missing from managed state", mac)
		return
	}

	delete(enis, mac)
}

// RemoveDeviceUnsafe is used to remove new devices from uDev events
// NOTE: removeDeviceUnsafe expects lock to be held prior to update for correct semantics
func (statemanager *stateManager) RemoveDeviceUnsafe(deviceName string) {
	log.Debugf("ENI state manager: removing device: %s (unsafe)", deviceName)

	enis := statemanager.GetAll()
	for mac, dev := range enis {
		if dev == deviceName {
			statemanager.RemoveDeviceWithMACAddressUnsafe(mac)
			return
		}
	}
	log.Debug("ENI state manager: no device was removed from the map")
}

// AddDeviceWithMACAddress adds new devices upon initialization
func (statemanager *stateManager) AddDeviceWithMACAddress(deviceName, macAddress string) {
	log.Debugf("ENI state manager: adding device %s with MAC %s", deviceName, macAddress)

	statemanager.updateLock.Lock()
	defer statemanager.updateLock.Unlock()

	statemanager.AddDeviceWithMACAddressUnsafe(deviceName, macAddress)
}

// RemoveDeviceWithMACAddress is used to remove new devices from maintained state
func (statemanager *stateManager) RemoveDeviceWithMACAddress(mac string) {
	log.Debugf("ENI state manager: removing device with MACAddress: %s", mac)

	statemanager.updateLock.Lock()
	defer statemanager.updateLock.Unlock()

	statemanager.RemoveDeviceWithMACAddressUnsafe(mac)
}

// RemoveDevice is used to remove new devices from uDev events
func (statemanager *stateManager) RemoveDevice(deviceName string) {
	log.Debugf("ENI state manager: removing device: %s", deviceName)

	statemanager.updateLock.Lock()
	defer statemanager.updateLock.Unlock()

	statemanager.RemoveDeviceUnsafe(deviceName)
}

func (statemanager *stateManager) IsMACAddressPresent(macAddress string) bool {
	log.Debugf("ENI state manager: checking state for MACAddress: %s", macAddress)

	statemanager.updateLock.RLock()
	defer statemanager.updateLock.RUnlock()

	_, ok := statemanager.enis[macAddress]
	return ok
}
