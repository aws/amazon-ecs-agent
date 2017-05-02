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
	"net"
	"testing"
	"time"

	"github.com/deniswernert/udev"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"

	eniStateManager "github.com/aws/amazon-ecs-agent/agent/eni/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/eni/statemanager/mocks"

	"github.com/aws/amazon-ecs-agent/agent/eni/netlinkwrapper/mocks"
	"github.com/aws/amazon-ecs-agent/agent/eni/udevwrapper/mocks"
)

const (
	randomDevice     = "eth1"
	randomMAC        = "00:0a:95:9d:68:16"
	randomDevPath    = " ../../devices/pci0000:00/0000:00:03.0/net/eth1"
	incorrectDevPath = "../../devices/totally/wrong/net/path"
)

// TestEmptyWatcherStruct checks initialization of a new watcher
func TestEmptyWatcherStruct(t *testing.T) {
	ctx := context.Background()
	stateManager := eniStateManager.New()
	watcher := _new(ctx, nil, nil, stateManager)
	enis := watcher.getAllENIs()
	watcher.Stop()
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

// TestWatcherInit checks the sanity of watcher initialization
func TestWatcherInit(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	pm, _ := net.ParseMAC(randomMAC)
	stateManager := eniStateManager.New()

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)

	// Init() uses netlink.LinkList() to build initial state
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

// TestInitWithNetlinkError checks the netlink linklist error path
func TestInitWithNetlinkError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	mockNetlink.EXPECT().LinkList().Return([]netlink.Link{},
		errors.New("Dummy Netlink LinkList error"))
	stateManager := eniStateManager.New()

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)
	err := watcher.Init()
	watcher.Stop()
	assert.Error(t, err)
}

// TestWatcherInitWithEmptyList checks sanity of watcher upon empty list
func TestWatcherInitWithEmptyList(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	stateManager := eniStateManager.New()

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)

	// Init() uses netlink.LinkList() to build initial state
	gomock.InOrder(
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{}, nil),
	)

	err := watcher.Init()
	watcher.Stop()
	assert.Error(t, err)
}

// TestReconcileENIs tests the reconciliation code path
func TestReconcileENIs(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	pm, _ := net.ParseMAC(randomMAC)
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	stateManager := eniStateManager.New()

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

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)
	watcher.reconcileOnce()
	enis := watcher.getAllENIs()
	watcher.Stop()
	assert.Len(t, enis, 1)
	assert.True(t, watcher.IsMACAddressPresent(randomMAC))
}

// TestReconcileENIsWithNetlinkErr tests reconciliation with netlink error
func TestReconcileENIsWithNetlinkErr(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	mockNetlink.EXPECT().LinkList().Return([]netlink.Link{},
		errors.New("Dummy Netlink LinkList error"))
	stateManager := eniStateManager.New()

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)
	watcher.reconcileOnce()
	enis := watcher.getAllENIs()
	watcher.Stop()
	assert.Empty(t, enis)
}

// TestReconcileENIsWithEmptyList checks sanity on empty list from Netlink
func TestReconcileENIsWithEmptyList(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	stateManager := eniStateManager.New()

	gomock.InOrder(
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{}, nil),
	)

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)
	watcher.reconcileOnce()
	enis := watcher.getAllENIs()
	watcher.Stop()
	assert.Empty(t, enis)
	assert.False(t, watcher.IsMACAddressPresent(randomMAC))
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

	ctx := context.TODO()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	mockUdev := mock_udevwrapper.NewMockUdev(mockCtrl)
	pm, _ := net.ParseMAC(randomMAC)
	mockStateManager := mock_statemanager.NewMockStateManager(mockCtrl)
	done := make(chan bool)

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
		mockStateManager.EXPECT().AddDeviceWithMACAddress(randomDevice, randomMAC).Do(
			func(randomDevice, randomMAC string) {
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

// TestUdevSubsystemFilter checks the subsystem filter in the event handler
func TestUdevSubsystemFilter(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	// Setup Mock Udev
	mockUdev := mock_udevwrapper.NewMockUdev(mockCtrl)
	stateManager := eniStateManager.New()

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
	stateManager := eniStateManager.New()

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
	stateManager := eniStateManager.New()

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

// TestUdevRemoveEvent removes a device based on udev event
func TestUdevRemoveEvent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	done := make(chan bool)

	// Setup Mock Udev
	mockUdev := mock_udevwrapper.NewMockUdev(mockCtrl)

	// Setup State Change
	mockStateManager := mock_statemanager.NewMockStateManager(mockCtrl)

	// Create Watcher
	watcher := _new(ctx, nil, mockUdev, mockStateManager)

	gomock.InOrder(
		mockUdev.EXPECT().Monitor(watcher.events).Return(
			nil,
		),
		mockStateManager.EXPECT().RemoveDevice(randomDevice).Do(
			func(randomDevice string) {
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

// TestUdevContext checks the watcher context in the eventHandler
func TestUdevContext(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	mockUdev := mock_udevwrapper.NewMockUdev(mockCtrl)
	pm, _ := net.ParseMAC(randomMAC)
	done := make(chan bool)

	mockStateManager := mock_statemanager.NewMockStateManager(mockCtrl)

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
		mockStateManager.EXPECT().AddDeviceWithMACAddress(randomDevice, randomMAC).Do(
			func(randomDevice, randomMAC string) {
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

// TestPerformPeriodicReconciliation checks the reconciliation context
func TestPerformPeriodicReconciliationContext(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	stateManager := eniStateManager.New()

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

	ctx, cancel := context.WithCancel(context.Background())
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	pm, _ := net.ParseMAC(randomMAC)
	mockStateManager := mock_statemanager.NewMockStateManager(mockCtrl)
	done := make(chan bool)

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, mockStateManager)

	gomock.InOrder(
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{
			&netlink.Device{
				LinkAttrs: netlink.LinkAttrs{
					HardwareAddr: pm,
					Name:         randomDevice,
				},
			},
		}, nil),
		mockStateManager.EXPECT().Reconcile(gomock.Any()).Return(),
		mockNetlink.EXPECT().LinkList().Do(func() {
			done <- true
		}),
	)

	reconInterval := time.Microsecond * 1
	go watcher.performPeriodicReconciliation(ctx, reconInterval)

	invokeStatus := <-done

	// Stop Watcher
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
	stateManager := eniStateManager.New()

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
