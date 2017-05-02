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
	"math/rand"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"
)

const (
	randomDevice = "eth1"
	randomMAC    = "00:0a:95:9d:68:16"
)

// TestEmptyStateManager checks the empty state manager
func TestEmptyStateManager(t *testing.T) {
	statemanager := New()
	state := statemanager.GetAll()
	assert.Empty(t, state)
}

// setupStateManager creates a StateManager with one interface
func setupStateManager(t *testing.T) StateManager {
	statemanager := New()
	statemanager.AddDeviceWithMACAddress(randomDevice, randomMAC)

	return statemanager
}

// Generate Random MAC Address
func genRandomMACAddress() string {
	validAlphabet := "0123456789ABCDEF"
	lmac := 12
	b := make([]byte, lmac)

	for i := range b {
		b[i] = validAlphabet[rand.Intn(len(validAlphabet))]
	}

	mac := string(b)
	for i := 2; i < len(mac); i += 3 {
		mac = mac[:i] + ":" + mac[i:]
	}
	return mac
}

// TestAddDeviceWithMACAddress tests adding an interface to state manager
func TestAddDeviceWithMACAddress(t *testing.T) {
	statemanager := setupStateManager(t)
	state := statemanager.GetAll()
	assert.NotEmpty(t, state)
	assert.Len(t, state, 1)
	assert.True(t, statemanager.IsMACAddressPresent(randomMAC))
}

// TestRemoveDeviceWithMACAddress removes an interface from state manager
func TestRemoveDeviceWithMACAddress(t *testing.T) {
	statemanager := setupStateManager(t)

	state := statemanager.GetAll()
	assert.NotEmpty(t, state)
	assert.Len(t, state, 1)

	statemanager.RemoveDeviceWithMACAddress(randomMAC)

	state = statemanager.GetAll()
	assert.Empty(t, state)
	assert.False(t, statemanager.IsMACAddressPresent(randomMAC))
}

// TestRemoveDeviceWithMissingMACAddress attempts to remove a non-existent entity
// from state managed by state manager
func TestRemoveDeviceWithMissingMACAddress(t *testing.T) {
	statemanager := setupStateManager(t)

	state := statemanager.GetAll()
	assert.NotEmpty(t, state)
	assert.Len(t, state, 1)

	statemanager.RemoveDeviceWithMACAddress(genRandomMACAddress())

	state = statemanager.GetAll()
	assert.Len(t, state, 1)
	assert.True(t, statemanager.IsMACAddressPresent(randomMAC))
}

// TestRemoveDevice checks interface removal based on interface name
func TestRemoveDevice(t *testing.T) {
	statemanager := setupStateManager(t)

	state := statemanager.GetAll()
	assert.NotEmpty(t, state)
	assert.Len(t, state, 1)

	statemanager.RemoveDevice(randomDevice)

	state = statemanager.GetAll()
	assert.Empty(t, state)
	assert.False(t, statemanager.IsMACAddressPresent(randomMAC))
}

// TestAddDeviceWithMACAddressUnsafe checks the unsafe interface addition to state manager
func TestAddDeviceWithMACAddressUnsafe(t *testing.T) {
	statemanager := New()
	statemanager.AddDeviceWithMACAddressUnsafe(randomDevice, randomMAC)
	state := statemanager.GetAll()
	assert.NotEmpty(t, state)
	assert.Len(t, state, 1)
	assert.True(t, statemanager.IsMACAddressPresent(randomMAC))
}

// TestRemoveDeviceWithMACAddressUnsafe checks the unsafe interface removal from state manager
func TestRemoveDeviceWithMACAddressUnsafe(t *testing.T) {
	statemanager := setupStateManager(t)

	state := statemanager.GetAll()
	assert.NotEmpty(t, state)
	assert.Len(t, state, 1)

	statemanager.RemoveDeviceWithMACAddressUnsafe(randomMAC)

	state = statemanager.GetAll()
	assert.Empty(t, state)
	assert.False(t, statemanager.IsMACAddressPresent(randomMAC))
}

// TestRemoveDeviceWithMissingMACAddressUnsafe attempts to remove a
// non-existent entity from state managed by state manager
func TestRemoveDeviceWithMissingMACAddressUnsafe(t *testing.T) {
	statemanager := setupStateManager(t)

	state := statemanager.GetAll()
	assert.NotEmpty(t, state)
	assert.Len(t, state, 1)

	statemanager.RemoveDeviceWithMACAddressUnsafe(genRandomMACAddress())

	state = statemanager.GetAll()
	assert.Len(t, state, 1)
	assert.True(t, statemanager.IsMACAddressPresent(randomMAC))
}

// TestRemoveDeviceUnsafe checks the unsafe interface removal from state manager
// based on interface name
func TestRemoveDeviceUnsafe(t *testing.T) {
	statemanager := setupStateManager(t)

	state := statemanager.GetAll()
	assert.NotEmpty(t, state)
	assert.Len(t, state, 1)

	statemanager.RemoveDeviceUnsafe(randomDevice)

	state = statemanager.GetAll()
	assert.Empty(t, state)
	assert.False(t, statemanager.IsMACAddressPresent(randomMAC))
}

// TestInit checks the Init code path of state manager
func TestInit(t *testing.T) {
	statemanager := New()
	pm, _ := net.ParseMAC(randomMAC)
	links := []netlink.Link{
		&netlink.Device{
			LinkAttrs: netlink.LinkAttrs{
				HardwareAddr: pm,
				Name:         randomDevice,
			},
		},
	}

	statemanager.Init(links)

	state := statemanager.GetAll()
	assert.NotEmpty(t, state)
	assert.Len(t, state, 1)
	assert.True(t, statemanager.IsMACAddressPresent(randomMAC))
}

// TestBasicReconcile tests the interface addition to state manager during reconciliation
func TestBasicReconcile(t *testing.T) {
	statemanager := New()

	currentState := make(map[string]string)
	currentState[randomMAC] = randomDevice

	statemanager.Reconcile(currentState)

	state := statemanager.GetAll()
	assert.NotEmpty(t, state)
	assert.Len(t, state, 1)
	assert.True(t, statemanager.IsMACAddressPresent(randomMAC))
}

// TestReconcile checks the state manager reconciliation method
func TestReconcile(t *testing.T) {
	statemanager := New()

	statemanager.AddDeviceWithMACAddress("eth0", genRandomMACAddress())
	statemanager.AddDeviceWithMACAddress(randomDevice, randomMAC)

	currentState := make(map[string]string)
	currentState[randomMAC] = randomDevice

	statemanager.Reconcile(currentState)

	state := statemanager.GetAll()
	assert.NotEmpty(t, state)
	assert.Len(t, state, 1)
	assert.True(t, statemanager.IsMACAddressPresent(randomMAC))
}
