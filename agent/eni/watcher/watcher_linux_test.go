//go:build linux && unit
// +build linux,unit

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

package watcher

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/deniswernert/udev"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statechange"

	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/eni/netlinkwrapper"
	mock_netlinkwrapper "github.com/aws/amazon-ecs-agent/agent/eni/netlinkwrapper/mocks"
	"github.com/aws/amazon-ecs-agent/agent/eni/udevwrapper"
	mock_udevwrapper "github.com/aws/amazon-ecs-agent/agent/eni/udevwrapper/mocks"
)

const (
	randomDevice     = "eth1"
	randomDevPath    = " ../../devices/pci0000:00/0000:00:03.0/net/eth1"
	incorrectDevPath = "../../devices/totally/wrong/net/path"
)

// newTestWatcher creates an ENIWatcher for testing
func newTestWatcher(ctx context.Context,
	primaryMAC string,
	nlWrap netlinkwrapper.NetLink,
	udevWrap udevwrapper.Udev,
	state dockerstate.TaskEngineState,
	stateChangeEvents chan<- statechange.Event) *ENIWatcher {

	derivedContext, cancel := context.WithCancel(ctx)
	return &ENIWatcher{
		ctx:            derivedContext,
		cancel:         cancel,
		agentState:     state,
		eniChangeEvent: stateChangeEvents,
		primaryMAC:     primaryMAC,
		netlinkClient:  nlWrap,
		udevMonitor:    udevWrap,
		events:         make(chan *udev.UEvent),
	}
}

// TestWatcherInit checks the sanity of watcher initialization
func TestWatcherInit(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	parsedMAC, err := net.ParseMAC(randomMAC)
	assert.NoError(t, err)

	primaryMACAddr, err := net.ParseMAC("00:0a:95:9d:68:61")
	assert.NoError(t, err)

	taskEngineState := dockerstate.NewTaskEngineState()
	taskEngineState.AddENIAttachment(&apieni.ENIAttachment{
		MACAddress:       randomMAC,
		AttachStatusSent: false,
		ExpiresAt:        time.Unix(time.Now().Unix()+10, 0),
	})
	eventChannel := make(chan statechange.Event)

	// Create Watcher
	watcher := newTestWatcher(ctx, primaryMAC, mockNetlink, nil, taskEngineState, eventChannel)

	// Init() uses netlink.LinkList() to build initial state
	mockNetlink.EXPECT().LinkList().Return([]netlink.Link{
		&netlink.Device{
			LinkAttrs: netlink.LinkAttrs{
				HardwareAddr: parsedMAC,
				Name:         randomDevice,
			},
		},
		&netlink.Device{
			LinkAttrs: netlink.LinkAttrs{
				HardwareAddr: primaryMACAddr,
				Name:         "lo",
				EncapType:    "loopback",
			},
		},
	}, nil)

	waitForEvents := sync.WaitGroup{}
	waitForEvents.Add(1)
	var event statechange.Event
	go func() {
		event = <-eventChannel
		assert.NotNil(t, event.(api.TaskStateChange).Attachment)
		assert.Equal(t, randomMAC, event.(api.TaskStateChange).Attachment.MACAddress)
		waitForEvents.Done()
	}()
	watcher.Init()

	waitForEvents.Wait()

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
	eventChannel := make(chan statechange.Event)

	// Create Watcher
	watcher := newTestWatcher(ctx, primaryMAC, mockNetlink, nil, taskEngineState, eventChannel)
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
	eventChannel := make(chan statechange.Event)

	// Create Watcher
	watcher := newTestWatcher(ctx, primaryMAC, mockNetlink, nil, taskEngineState, eventChannel)

	// Init() uses netlink.LinkList() to build initial state
	mockNetlink.EXPECT().LinkList().Return([]netlink.Link{}, nil)

	err := watcher.Init()
	assert.NoError(t, err)
}

// TestReconcileENIs tests the reconciliation code path
func TestReconcileENIs(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	taskEngineState := dockerstate.NewTaskEngineState()
	eventChannel := make(chan statechange.Event)

	taskEngineState.AddENIAttachment(getMockAttachment())

	mockNetlink.EXPECT().LinkList().Return(getMockNetLinkResponse(t), nil)

	var event statechange.Event
	done := make(chan struct{})
	go func() {
		event = <-eventChannel
		done <- struct{}{}
	}()

	// Create Watcher
	watcher := newTestWatcher(ctx, primaryMAC, mockNetlink, nil, taskEngineState, eventChannel)
	require.NoError(t, watcher.reconcileOnce(false))

	<-done
	assert.NotNil(t, event.(api.TaskStateChange).Attachment)
	assert.Equal(t, randomMAC, event.(api.TaskStateChange).Attachment.MACAddress)

	select {
	case <-eventChannel:
		t.Errorf("Expect no more state change event")
	default:
	}
}

func TestReconcileENIsWithRetry(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	ctx := context.Background()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	mockState := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	eventChannel := make(chan statechange.Event)

	gomock.InOrder(
		mockNetlink.EXPECT().LinkList().Return(getMockNetLinkResponse(t), nil),
		// Let the first one fail to check that we retry on failure.
		mockState.EXPECT().ENIByMac(gomock.Any()).Return(nil, false),
		mockState.EXPECT().ENIByMac(gomock.Any()).Return(getMockAttachment(), true),
	)

	var event statechange.Event
	done := make(chan struct{})
	go func() {
		event = <-eventChannel
		done <- struct{}{}
	}()

	// Create Watcher
	watcher := newTestWatcher(ctx, primaryMAC, mockNetlink, nil, mockState, eventChannel)
	require.NoError(t, watcher.reconcileOnce(true))

	<-done
	require.NotNil(t, event.(api.TaskStateChange).Attachment)
	assert.Equal(t, randomMAC, event.(api.TaskStateChange).Attachment.MACAddress)

	select {
	case <-eventChannel:
		t.Errorf("Expect no more state change event")
	default:
	}
}

func getMockAttachment() *apieni.ENIAttachment {
	return &apieni.ENIAttachment{
		MACAddress:       randomMAC,
		AttachStatusSent: false,
		ExpiresAt:        time.Unix(time.Now().Unix()+10, 0),
	}
}

func getMockNetLinkResponse(t *testing.T) []netlink.Link {
	parsedMAC, err := net.ParseMAC(randomMAC)
	require.NoError(t, err)

	primaryMACAddr, err := net.ParseMAC("00:0a:95:9d:68:61")
	require.NoError(t, err)

	return []netlink.Link{
		&netlink.Device{
			LinkAttrs: netlink.LinkAttrs{
				HardwareAddr: parsedMAC,
				Name:         randomDevice,
			},
		},
		&netlink.Device{
			LinkAttrs: netlink.LinkAttrs{
				HardwareAddr: primaryMACAddr,
				Name:         "lo",
				EncapType:    "loopback",
			},
		},
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
	eventChannel := make(chan statechange.Event)

	// Create Watcher
	watcher := newTestWatcher(ctx, primaryMAC, mockNetlink, nil, taskEngineState, eventChannel)
	assert.Error(t, watcher.reconcileOnce(false))

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
	eventChannel := make(chan statechange.Event)

	mockNetlink.EXPECT().LinkList().Return([]netlink.Link{}, nil)

	// Create Watcher
	watcher := newTestWatcher(ctx, primaryMAC, mockNetlink, nil, taskEngineState, eventChannel)
	assert.NoError(t, watcher.reconcileOnce(false))
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
	parsedMAC, _ := net.ParseMAC(randomMAC)
	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	eventChannel := make(chan statechange.Event)

	// Create Watcher
	watcher := newTestWatcher(ctx, primaryMAC, mockNetlink, mockUdev, mockStateManager, eventChannel)

	shutdown := make(chan bool)
	gomock.InOrder(
		mockUdev.EXPECT().Monitor(watcher.events).Return(shutdown),
		mockNetlink.EXPECT().LinkByName(randomDevice).Return(
			&netlink.Device{
				LinkAttrs: netlink.LinkAttrs{
					HardwareAddr: parsedMAC,
					Name:         randomDevice,
				},
			}, nil),
		mockStateManager.EXPECT().ENIByMac(randomMAC).Return(
			&apieni.ENIAttachment{ExpiresAt: time.Unix(time.Now().Unix()+10, 0)}, true),
	)

	// Spin off event handler
	go watcher.eventHandler()
	// Send event to channel
	event := getUdevEventDummy(udevAddEvent, udevNetSubsystem, randomDevPath)
	watcher.events <- &event

	eniChangeEvent := <-eventChannel
	taskStateChange, ok := eniChangeEvent.(api.TaskStateChange)
	require.True(t, ok)
	assert.Equal(t, apieni.ENIAttached, taskStateChange.Attachment.Status)

	var waitForClose sync.WaitGroup
	waitForClose.Add(2)
	mockUdev.EXPECT().Close().Do(func() {
		waitForClose.Done()
	}).Return(nil)
	go func() {
		<-shutdown
		waitForClose.Done()
	}()

	go watcher.Stop()
	waitForClose.Wait()
}

// TestUdevSubsystemFilter checks the subsystem filter in the event handler
func TestUdevSubsystemFilter(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	// Setup Mock Udev
	mockUdev := mock_udevwrapper.NewMockUdev(mockCtrl)

	// Create Watcher
	watcher := newTestWatcher(ctx, primaryMAC, nil, mockUdev, nil, nil)

	shutdown := make(chan bool)
	mockUdev.EXPECT().Monitor(watcher.events).Return(shutdown)

	// Spin off event handler
	go watcher.eventHandler()
	// Send event to channel
	// This event shouldn't trigger the statemanager to handle HandleENIEvent
	event := getUdevEventDummy(udevAddEvent, udevPCISubsystem, randomDevPath)
	watcher.events <- &event

	var waitForClose sync.WaitGroup
	waitForClose.Add(2)
	mockUdev.EXPECT().Close().Do(func() {
		waitForClose.Done()
	}).Return(nil)
	go func() {
		<-shutdown
		waitForClose.Done()
	}()

	go watcher.Stop()
	waitForClose.Wait()
}

// TestUdevAddEventWithInvalidInterface attempts to add a device without
// a well defined interface
func TestUdevAddEventWithInvalidInterface(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()

	// Setup Mock Udev
	mockUdev := mock_udevwrapper.NewMockUdev(mockCtrl)
	// Create Watcher
	watcher := newTestWatcher(ctx, primaryMAC, nil, mockUdev, nil, nil)

	shutdown := make(chan bool)
	mockUdev.EXPECT().Monitor(watcher.events).Return(shutdown)

	// Spin off event handler
	go watcher.eventHandler()

	// Send event to channel
	event := getUdevEventDummy(udevAddEvent, udevNetSubsystem, incorrectDevPath)
	watcher.events <- &event

	var waitForClose sync.WaitGroup
	waitForClose.Add(2)
	mockUdev.EXPECT().Close().Do(func() {
		waitForClose.Done()
	}).Return(nil)
	go func() {
		<-shutdown
		waitForClose.Done()
	}()

	go watcher.Stop()
	waitForClose.Wait()
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

	watcher := newTestWatcher(ctx, primaryMAC, mockNetlink, mockUdev, nil, nil)

	var invoked sync.WaitGroup
	invoked.Add(1)

	shutdown := make(chan bool)
	gomock.InOrder(
		mockUdev.EXPECT().Monitor(watcher.events).Return(shutdown),
		mockNetlink.EXPECT().LinkByName(randomDevice).Do(func(device string) {
			invoked.Done()
		}).Return(
			&netlink.Device{},
			errors.New("Dummy Netlink LinkByName error")),
	)

	// Spin off event handler
	go watcher.eventHandler()

	// Send event to channel
	event := getUdevEventDummy(udevAddEvent, udevNetSubsystem, randomDevPath)
	watcher.events <- &event
	invoked.Wait()

	var waitForClose sync.WaitGroup
	waitForClose.Add(2)
	mockUdev.EXPECT().Close().Do(func() {
		waitForClose.Done()
	}).Return(nil)
	go func() {
		<-shutdown
		waitForClose.Done()
	}()

	go watcher.Stop()
	waitForClose.Wait()
}
