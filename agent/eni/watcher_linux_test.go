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

package eni

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/deniswernert/udev"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"

	"github.com/aws/amazon-ecs-agent/agent/eni/mocks"
	"github.com/aws/amazon-ecs-agent/agent/eni/netlinkwrapper/mocks"
	"github.com/aws/amazon-ecs-agent/agent/eni/udevwrapper/mocks"
)

const (
	randomDevice     = "eth1"
	randomMAC        = "00:0a:95:9d:68:16"
	randomDevPath    = " ../../devices/pci0000:00/0000:00:03.0/net/eth1"
	invalidMAC       = "0a:1b:3c:4d:5e:6ff"
	invalidDevice    = "xyz"
	incorrectDevPath = "../../devices/totally/wrong/net/path"
	numRetries       = 3
)

// TestEmptyWatcherStruct checks initialization of a new watcher
func TestEmptyWatcherStruct(t *testing.T) {
	ctx := context.Background()
	stateManager := NewStateManager()
	watcher := _new(ctx, nil, nil, stateManager)
	enis := watcher.getAllENIs()
	assert.Empty(t, enis)
}

// TestEmptyWatcherStruct checks initialization of a new watcher
func TestEmptyWatcherRootWrapper(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	ctx := context.Background()
	mockUdev := mock_udevwrapper.NewMockUdev(mockCtrl)
	watcher := New(ctx, mockUdev)
	enis := watcher.getAllENIs()
	watcher.Stop()
	assert.Empty(t, enis)
}

// Setup a basic watcher with a single added interface
func setupBasicWatcher(t *testing.T) *UdevWatcher {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	stateManager := NewStateManager()

	watcher := _new(ctx, nil, nil, stateManager)

	// Add valid (device, MAC)
	watcher.addDeviceWithMACAddressUnsafe(randomDevice, randomMAC)
	return watcher
}

// TestAddDeviceWithMACAddress checks adding devices to the watcher
func TestAddDeviceWithMACAddress(t *testing.T) {
	watcher := setupBasicWatcher(t)
	enis := watcher.getAllENIs()
	assert.NotEmpty(t, enis)
	assert.Len(t, enis, 1)
	assert.True(t, watcher.IsMACAddressPresent(randomMAC))
}

// TestRemoveDeviceWithMACAddress checks removing devices from the watcher
func TestRemoveDeviceWithMACAddress(t *testing.T) {
	watcher := setupBasicWatcher(t)
	enis := watcher.getAllENIs()
	assert.NotEmpty(t, enis)
	assert.Len(t, enis, 1)
	assert.True(t, watcher.IsMACAddressPresent(randomMAC))

	watcher.removeDeviceWithMACAddressUnsafe(randomMAC)
	enis = watcher.getAllENIs()
	assert.Empty(t, enis)
	assert.False(t, watcher.IsMACAddressPresent(randomMAC))
}

// TestRemoveDevice checks removing devices from watcher
func TestRemoveDevice(t *testing.T) {
	watcher := setupBasicWatcher(t)
	enis := watcher.getAllENIs()
	assert.NotEmpty(t, enis)
	assert.Len(t, enis, 1)
	assert.True(t, watcher.IsMACAddressPresent(randomMAC))

	// Remove device from Watcher
	watcher.removeDeviceUnsafe(randomDevice)
	enis = watcher.getAllENIs()
	assert.Empty(t, enis)
	assert.False(t, watcher.IsMACAddressPresent(randomMAC))
}

// TestWatcherInit checks the sanity of watcher initialization
func TestWatcherInit(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	pm, _ := net.ParseMAC(randomMAC)
	stateManager := NewStateManager()

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)

	// Init() uses netlink.LinkList() to build initial state
	// eventHandler() upon receiving an add device event uses netlink.LinkByName
	// to fetch the MACAddress
	gomock.InOrder(
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{
			&netlink.Device{
				LinkAttrs: netlink.LinkAttrs{
					HardwareAddr: pm,
					Name:         randomDevice,
				},
			},
		}, nil),
	)

	watcher.Init()

	enis := watcher.getAllENIs()

	watcher.Stop()

	assert.NotEmpty(t, enis)
	assert.Len(t, enis, 1)
	assert.True(t, watcher.IsMACAddressPresent(randomMAC))
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

// TestRemoveDeviceWithMissingState attempts to remove a non-existent device
func TestRemoveDeviceWithMissingState(t *testing.T) {
	watcher := setupBasicWatcher(t)
	watcher.removeDeviceWithMACAddressUnsafe(genRandomMACAddress())
	enis := watcher.getAllENIs()
	assert.Len(t, enis, 1)
	assert.True(t, watcher.IsMACAddressPresent(randomMAC))
}

// TestRemoveDeviceWithMissingDevice attempts to remove a non-existent device
func TestRemoveDeviceWithMissingDevice(t *testing.T) {
	watcher := setupBasicWatcher(t)
	watcher.removeDeviceUnsafe(invalidDevice)
	enis := watcher.getAllENIs()
	assert.Len(t, enis, 1)
	assert.True(t, watcher.IsMACAddressPresent(randomMAC))
}

// TestInitWithNetlinkError checks the netlink linklist error path
func TestInitWithNetlinkError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	mockNetlink.EXPECT().LinkList().Return([]netlink.Link{},
		errors.New("Dummy Netlink LinkList error"))
	stateManager := NewStateManager()

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)
	err := watcher.Init()
	assert.Error(t, err)
}

// TestReconcileENIs tests the reconciliation code path
func TestReconcileENIs(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	mockNetlink.EXPECT().LinkList().Return([]netlink.Link{}, nil)
	stateManager := NewStateManager()

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)
	watcher.reconcileOnce()
	enis := watcher.getAllENIs()
	assert.Len(t, enis, 0)
	assert.False(t, watcher.IsMACAddressPresent(randomMAC))
}

// TestReconcileENIsWithNetlinkErr tests reconciliation with netlink error
func TestReconcileENIsWithNetlinkErr(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	mockNetlink.EXPECT().LinkList().Return([]netlink.Link{},
		errors.New("Dummy Netlink LinkList error"))
	stateManager := NewStateManager()

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)
	watcher.reconcileOnce()
	enis := watcher.getAllENIs()
	assert.Empty(t, enis)
}

// TestReconcileENIsWithRemoval tests multiple reconciliation iterations
func TestReconcileENIsWithRemoval(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	pm, _ := net.ParseMAC(randomMAC)
	gomock.InOrder(
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{
			&netlink.Device{
				LinkAttrs: netlink.LinkAttrs{
					HardwareAddr: pm,
					Name:         randomDevice,
				},
			},
		}, nil),
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{
			&netlink.Device{
				LinkAttrs: netlink.LinkAttrs{
					HardwareAddr: pm,
					Name:         invalidDevice,
				},
			},
		}, nil),
	)
	stateManager := NewStateManager()

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)

	watcher.reconcileOnce()
	watcher.reconcileOnce()
	enis := watcher.getAllENIs()
	assert.Len(t, enis, 1)
	assert.True(t, watcher.IsMACAddressPresent(randomMAC))
}

// TestIsMacAddressPresentInManagedState checks if MacAddress is present in managed state
func TestIsMacAddressPresentInManagedState(t *testing.T) {
	watcher := setupBasicWatcher(t)
	macStatus := watcher.IsMACAddressPresent(randomMAC)
	assert.True(t, macStatus)
	enis := watcher.getAllENIs()
	assert.Len(t, enis, 1)
	assert.True(t, watcher.IsMACAddressPresent(randomMAC))
}

// TestIsMacAddressNotPresentInManagedState checks if MacAddress is not present in managed state
func TestIsMacAddressNotPresentInManagedState(t *testing.T) {
	watcher := setupBasicWatcher(t)
	macStatus := watcher.IsMACAddressPresent(invalidMAC)
	assert.False(t, macStatus)
	enis := watcher.getAllENIs()
	assert.Len(t, enis, 1)
	assert.True(t, watcher.IsMACAddressPresent(randomMAC))
}

// getUdevEventDummy builds a dummy udev.UEvent object
func getUdevEventDummy(action, subsystem, devpath string) udev.UEvent {
	m := make(map[string]string, 5)
	m["INTERFACE"] = "eth1"
	m["IFINDEX"] = "1"
	m["ACTION"] = action
	m["SUBSYSTEM"] = subsystem
	m["DEVPATH"] = devpath
	event := udev.UEvent{
		Env: m,
	}
	return event
}

// TestUdevAddEvent tests adding a device from an udev event
func TestUdevAddEvent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	done := make(chan bool)
	ctx := context.TODO()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	mockUdev := mock_udevwrapper.NewMockUdev(mockCtrl)
	pm, _ := net.ParseMAC(randomMAC)
	mockStateManager := mock_eni.NewMockStateManagerInterface(mockCtrl)

	// Create Watcher
	watcher := _new(ctx, mockNetlink, mockUdev, mockStateManager)

	gomock.InOrder(
		mockUdev.EXPECT().Monitor(watcher.events).Return(
			nil,
		),
		mockNetlink.EXPECT().LinkByName(randomDevice).Return(
			&netlink.Device{
				LinkAttrs: netlink.LinkAttrs{
					HardwareAddr: pm,
					Name:         randomDevice,
				},
			}, nil),
		mockStateManager.EXPECT().Lock().Return(),
		mockStateManager.EXPECT().AddDeviceWithMACAddressUnsafe(randomDevice, randomMAC).Return(),
		mockStateManager.EXPECT().Unlock().Do(
			func() {
				done <- true
			}),
	)

	// Spin off event handler
	go watcher.eventHandler(ctx)

	// Send event to channel
	event := getUdevEventDummy(udevAddEvent, udevNetSubsystem, randomDevPath)
	watcher.events <- &event

	invokeStatus := <-done

	watcher.Stop()
	assert.True(t, invokeStatus)
}

// TestUdevRemoveEvent removes a device based on udev event
func TestUdevRemoveEvent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	done := make(chan bool)

	// Setup Mock Udev
	mockUdev := mock_udevwrapper.NewMockUdev(mockCtrl)

	// Setup State Change
	mockStateManager := mock_eni.NewMockStateManagerInterface(mockCtrl)

	// Create Watcher
	watcher := _new(ctx, nil, mockUdev, mockStateManager)

	gomock.InOrder(
		mockUdev.EXPECT().Monitor(watcher.events).Return(
			nil,
		),
		mockStateManager.EXPECT().Lock().Return(),
		mockStateManager.EXPECT().RemoveDeviceUnsafe(randomDevice).Return(),
		mockStateManager.EXPECT().Unlock().Do(
			func() {
				done <- true
			}),
	)

	// Spin off event handler
	go watcher.eventHandler(ctx)

	// Remove Device
	event := getUdevEventDummy(udevRemoveEvent, udevNetSubsystem, randomDevPath)
	watcher.events <- &event

	invokeStatus := <-done

	// Stop Watcher
	watcher.Stop()

	assert.True(t, invokeStatus)
}

// TestUdevSubsystemFilter checks the subsystem filter in the event handler
func TestUdevSubsystemFilter(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	// Setup Mock Udev
	mockUdev := mock_udevwrapper.NewMockUdev(mockCtrl)
	stateManager := NewStateManager()

	// Create Watcher
	watcher := _new(ctx, nil, mockUdev, stateManager)

	mockUdev.EXPECT().Monitor(watcher.events).Return(
		nil,
	)

	// Spin off event handler
	go watcher.eventHandler(ctx)

	// Send event to channel
	event := getUdevEventDummy(udevAddEvent, udevPCISubsystem, randomDevPath)
	watcher.events <- &event

	// Fetch All ENIs
	enis := watcher.getAllENIs()

	// Stop Watcher
	watcher.Stop()

	assert.Empty(t, enis)
	assert.False(t, watcher.IsMACAddressPresent(randomMAC))
}

// TestUdevAddEventWithInvalidInterface attempts to add a device without
// a well defined interface
func TestUdevAddEventWithInvalidInterface(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()

	// Setup Mock Udev
	mockUdev := mock_udevwrapper.NewMockUdev(mockCtrl)

	// Setup State Change
	stateManager := NewStateManager()

	// Create Watcher
	watcher := _new(ctx, nil, mockUdev, stateManager)

	gomock.InOrder(
		mockUdev.EXPECT().Monitor(watcher.events).Return(
			nil,
		),
	)

	// Spin off event handler
	go watcher.eventHandler(ctx)

	// Send event to channel
	event := getUdevEventDummy(udevAddEvent, udevNetSubsystem, incorrectDevPath)
	watcher.events <- &event

	// Fetch All ENIs
	enis := watcher.getAllENIs()

	// Stop Watcher
	watcher.Stop()

	assert.Empty(t, enis)
}

// TestUdevAddEventWithoutMACAdress attempts to add a device without
// a MACAddress based on an udev event
func TestUdevAddEventWithoutMACAdress(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	// Setup Mock Netlink
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	// Setup Mock Udev
	mockUdev := mock_udevwrapper.NewMockUdev(mockCtrl)

	// Create Watcher
	stateManager := NewStateManager()

	// Create Watcher
	watcher := _new(ctx, mockNetlink, mockUdev, stateManager)

	gomock.InOrder(
		mockUdev.EXPECT().Monitor(watcher.events).Return(
			nil,
		),
		mockNetlink.EXPECT().LinkByName(randomDevice).Return(
			&netlink.Device{},
			errors.New("Dummy Netlink LinkByName error")),
	)

	// Spin off event handler
	go watcher.eventHandler(ctx)

	// Send event to channel
	event := getUdevEventDummy(udevAddEvent, udevNetSubsystem, randomDevPath)
	watcher.events <- &event

	// Fetch All ENIs
	enis := watcher.getAllENIs()

	// Stop Watcher
	watcher.Stop()

	assert.Empty(t, enis)
	assert.False(t, watcher.IsMACAddressPresent(randomMAC))
}

// TestUdevContext checks the watcher context in the eventHandler
func TestUdevContext(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	mockUdev := mock_udevwrapper.NewMockUdev(mockCtrl)
	pm, _ := net.ParseMAC(randomMAC)
	done := make(chan bool)

	mockStateManager := mock_eni.NewMockStateManagerInterface(mockCtrl)

	// Create Watcher
	watcher := _new(ctx, mockNetlink, mockUdev, mockStateManager)

	gomock.InOrder(
		mockUdev.EXPECT().Monitor(watcher.events).Return(
			nil,
		),
		mockNetlink.EXPECT().LinkByName(randomDevice).Return(
			&netlink.Device{
				LinkAttrs: netlink.LinkAttrs{
					HardwareAddr: pm,
					Name:         randomDevice,
				},
			}, nil),
		mockStateManager.EXPECT().Lock().Return(),
		mockStateManager.EXPECT().AddDeviceWithMACAddressUnsafe(randomDevice, randomMAC).Return(),
		mockStateManager.EXPECT().Unlock().Do(
			func() {
				done <- true
			}),
	)

	// Spin off event handler
	go watcher.eventHandler(ctx)

	// Send event to channel
	event := getUdevEventDummy(udevAddEvent, udevNetSubsystem, randomDevPath)
	watcher.events <- &event

	invokeStatus := <-done

	// Send cancellation
	cancel()

	assert.True(t, invokeStatus)
}

// TestStartStop checks the Start and Stop methods of the watcher
func TestStartStop(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	mockUdev := mock_udevwrapper.NewMockUdev(mockCtrl)
	pm, _ := net.ParseMAC(randomMAC)
	stateManager := NewStateManager()

	// Create Watcher
	watcher := _new(ctx, mockNetlink, mockUdev, stateManager)

	gomock.InOrder(
		mockUdev.EXPECT().Monitor(watcher.events).Return(
			nil,
		).AnyTimes(),
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{
			&netlink.Device{
				LinkAttrs: netlink.LinkAttrs{
					HardwareAddr: pm,
					Name:         randomDevice,
				},
			},
		}, nil).AnyTimes(),
	)
	go watcher.Start()

	enis := watcher.getAllENIs()

	watcher.Stop()

	assert.Empty(t, enis)
	assert.False(t, watcher.IsMACAddressPresent(randomMAC))
}

// TestPerformPeriodicReconciliation checks the reconciliation context
func TestPerformPeriodicReconciliationContext(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	stateManager := NewStateManager()

	// Create Watcher
	watcher := _new(ctx, nil, nil, stateManager)

	go watcher.performPeriodicReconciliation(ctx, defaultReconciliationInterval)

	enis := watcher.getAllENIs()

	cancel()

	assert.Empty(t, enis)
	assert.False(t, watcher.IsMACAddressPresent(randomMAC))
}

// TestPerformPeriodicReconciliation checks the reconciliation ticker
func TestPerformPeriodicReconciliationTicker(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	done := make(chan bool)

	ctx := context.TODO()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	pm, _ := net.ParseMAC(randomMAC)
	stateManager := NewStateManager()

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)

	gomock.InOrder(
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{
			&netlink.Device{
				LinkAttrs: netlink.LinkAttrs{
					HardwareAddr: pm,
					Name:         randomDevice,
				},
			},
		}, nil),
		mockNetlink.EXPECT().LinkList().Do(
			func() {
				done <- true
			}),
	)

	reconInterval := time.Microsecond * 1
	go watcher.performPeriodicReconciliation(ctx, reconInterval)

	invokeStatus := <-done

	// Stop Watcher
	watcher.Stop()

	assert.True(t, invokeStatus)
}
