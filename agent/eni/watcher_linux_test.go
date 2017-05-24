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

	"github.com/deniswernert/udev"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
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

// TestWatcherInit checks the sanity of watcher initialization
func TestWatcherInit(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	pm, _ := net.ParseMAC(randomMAC)

	taskEngineState := dockerstate.NewTaskEngineState()
	taskEngineState.AddENIAttachment(&api.ENIAttachment{
		MacAddress:       randomMAC,
		AttachStatusSent: false,
	})
	eventChannel := make(chan api.TaskStateChange)
	stateManager := eniStateManager.New(taskEngineState, eventChannel)

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

	var event api.TaskStateChange
	go func() { event = <-eventChannel }()
	watcher.Init()

	assert.NotNil(t, event.Attachments)
	assert.Equal(t, randomMAC, event.Attachments.MacAddress)

	select {
	case <-eventChannel:
		t.Errorf("Expect no more state change event")
	default:
	}
}

// TestInitWithNetlinkError checks the netlink linklist error path
func TestInitWithNetlinkError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	mockNetlink.EXPECT().LinkList().Return([]netlink.Link{},
		errors.New("Dummy Netlink LinkList error"))

	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan api.TaskStateChange)
	stateManager := eniStateManager.New(taskEngineState, eventChannel)

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)
	err := watcher.Init()
	assert.Error(t, err)
}

// TestWatcherInitWithEmptyList checks sanity of watcher upon empty list
func TestWatcherInitWithEmptyList(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan api.TaskStateChange)
	stateManager := eniStateManager.New(taskEngineState, eventChannel)

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)

	// Init() uses netlink.LinkList() to build initial state
	gomock.InOrder(
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{}, nil),
	)

	err := watcher.Init()
	assert.Error(t, err)
}

// TestReconcileENIs tests the reconciliation code path
func TestReconcileENIs(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	pm, _ := net.ParseMAC(randomMAC)
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)

	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan api.TaskStateChange)
	stateManager := eniStateManager.New(taskEngineState, eventChannel)

	taskEngineState.AddENIAttachment(&api.ENIAttachment{
		MacAddress:       randomMAC,
		AttachStatusSent: false,
	})

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

	var event api.TaskStateChange
	done := make(chan struct{})
	go func() {
		event = <-eventChannel
		done <- struct{}{}
	}()

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)
	watcher.reconcileOnce()

	<-done
	assert.NotNil(t, event.Attachments)
	assert.Equal(t, randomMAC, event.Attachments.MacAddress)

	select {
	case <-eventChannel:
		t.Errorf("Expect no more state change event")
	default:
	}
}

// TestReconcileENIsWithNetlinkErr tests reconciliation with netlink error
func TestReconcileENIsWithNetlinkErr(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	mockNetlink.EXPECT().LinkList().Return([]netlink.Link{},
		errors.New("Dummy Netlink LinkList error"))

	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan api.TaskStateChange)
	stateManager := eniStateManager.New(taskEngineState, eventChannel)

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)
	watcher.reconcileOnce()

	select {
	case <-eventChannel:
		t.Errorf("Expect no more state change event")
	default:
	}
}

// TestReconcileENIsWithEmptyList checks sanity on empty list from Netlink
func TestReconcileENIsWithEmptyList(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)

	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan api.TaskStateChange)
	stateManager := eniStateManager.New(taskEngineState, eventChannel)

	gomock.InOrder(
		mockNetlink.EXPECT().LinkList().Return([]netlink.Link{}, nil),
	)

	// Create Watcher
	watcher := _new(ctx, mockNetlink, nil, stateManager)
	watcher.reconcileOnce()
	watcher.Stop()

	select {
	case <-eventChannel:
		t.Errorf("Expect no more state change event")
	default:
	}
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
		mockStateManager.EXPECT().HandleENIEvent(randomMAC).Do(
			func(mac string) {
				assert.Equal(t, randomMAC, mac)
				done <- true
			}),
	)

	// Spin off event handler
	go watcher.eventHandler(ctx)

	// Send event to channel
	event := getUdevEventDummy(udevAddEvent, udevNetSubsystem, randomDevPath)
	watcher.events <- &event

	watcher.Stop()
	assert.True(t, <-done)
}

// TestUdevSubsystemFilter checks the subsystem filter in the event handler
func TestUdevSubsystemFilter(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	// Setup Mock Udev
	mockUdev := mock_udevwrapper.NewMockUdev(mockCtrl)
	mockStateManager := mock_statemanager.NewMockStateManager(mockCtrl)

	// Create Watcher
	watcher := _new(ctx, nil, mockUdev, mockStateManager)

	mockUdev.EXPECT().Monitor(watcher.events).Return(
		nil,
	)

	// Spin off event handler
	go watcher.eventHandler(ctx)

	// Send event to channel
	event := getUdevEventDummy(udevAddEvent, udevPCISubsystem, randomDevPath)
	watcher.events <- &event
	watcher.Stop()

	// This event shouldn't trigger the statemanager to handle HandleENIEvent
}

// TestUdevAddEventWithInvalidInterface attempts to add a device without
// a well defined interface
func TestUdevAddEventWithInvalidInterface(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()

	// Setup Mock Udev
	mockUdev := mock_udevwrapper.NewMockUdev(mockCtrl)
	mockStateManager := mock_statemanager.NewMockStateManager(mockCtrl)
	// Create Watcher
	watcher := _new(ctx, nil, mockUdev, mockStateManager)

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
	watcher.Stop()
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

	mockStateManager := mock_statemanager.NewMockStateManager(mockCtrl)
	watcher := _new(ctx, mockNetlink, mockUdev, mockStateManager)

	invoked := make(chan struct{})
	gomock.InOrder(
		mockUdev.EXPECT().Monitor(watcher.events).Return(
			nil,
		),
		mockNetlink.EXPECT().LinkByName(randomDevice).Do(func(device string) {
			invoked <- struct{}{}
		}).Return(
			&netlink.Device{},
			errors.New("Dummy Netlink LinkByName error")),
	)

	// Spin off event handler
	go watcher.eventHandler(ctx)

	// Send event to channel
	event := getUdevEventDummy(udevAddEvent, udevNetSubsystem, randomDevPath)
	watcher.events <- &event
	<-invoked
	watcher.Stop()
}
