//go:build windows && unit
// +build windows,unit

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
	"net"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/agent/eni/iphelperwrapper"
	mock_iphelperwrapper "github.com/aws/amazon-ecs-agent/agent/eni/iphelperwrapper/mocks"
	mock_networkutils "github.com/aws/amazon-ecs-agent/agent/eni/networkutils/mocks"
	"github.com/aws/amazon-ecs-agent/agent/statechange"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

const (
	interfaceIndex1 = 9
	macAddress1     = "02:22:ea:8c:81:dc"
	interfacename1  = "Ethernet 9"

	interfaceIndex2 = 13
	macAddress2     = "02:b5:1d:b3:08:92"
	interfacename2  = "Ethernet 13"

	virtualInerfaceIndex = 35
	virtualMacAddress    = "00:15:5d:cb:fa:26"
	virtualInterfacename = "vEthernet (nat)"

	// Time delay after which the watcher context is cancelled at the end of the test
	cancelContextDelay = time.Second
)

// This function returns a list of all the interfaces(2 real and one v-switch attached interface)
func constructInterfaceList() []net.Interface {
	interfaces := make([]net.Interface, 3)
	mac1, _ := net.ParseMAC(macAddress1)
	mac2, _ := net.ParseMAC(macAddress2)
	mac3, _ := net.ParseMAC(virtualMacAddress)
	interfaces[0] = net.Interface{
		Index:        interfaceIndex1,
		Name:         interfacename1,
		HardwareAddr: mac1,
	}
	interfaces[1] = net.Interface{
		Index:        interfaceIndex2,
		Name:         interfacename2,
		HardwareAddr: mac2,
	}
	interfaces[2] = net.Interface{
		Index:        virtualInerfaceIndex,
		Name:         virtualInterfacename,
		HardwareAddr: mac3,
	}
	return interfaces
}

// newTestWatcher is used to create a watcher for testing purpose
func newTestWatcher(ctx context.Context, primaryMAC string, state dockerstate.TaskEngineState,
	stateChangeEvents chan<- statechange.Event, eniMonitor iphelperwrapper.InterfaceMonitor) (*ENIWatcher, error) {

	notificationChannel := make(chan int)
	err := eniMonitor.Start(notificationChannel)
	if err != nil {
		return nil, errors.Wrapf(err, "error occurred while instantiating watcher")
	}

	derivedContext, cancel := context.WithCancel(ctx)
	return &ENIWatcher{
		ctx:              derivedContext,
		cancel:           cancel,
		agentState:       state,
		eniChangeEvent:   stateChangeEvents,
		primaryMAC:       primaryMAC,
		interfaceMonitor: eniMonitor,
		notifications:    notificationChannel,
	}, nil
}

// NewWindowsWatcher test case. All works well here and we get a working ENI watcher
func TestNewWatcherWithoutError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	eventChannel := make(chan statechange.Event)
	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	mockiphelper.EXPECT().Start(gomock.Any()).Return(
		nil)

	_, err := newTestWatcher(ctx, primaryMAC, nil, eventChannel, mockiphelper)

	assert.NoError(t, err)
}

// NewWindowsWatcher test case. An error occurs while starting the monitor in this case. Therefore we get an error
func TestNewWatcherWithError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	eventChannel := make(chan statechange.Event)
	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	mockiphelper.EXPECT().Start(gomock.Any()).Return(
		errors.New("Error while starting monitor"))

	_, err := newTestWatcher(ctx, primaryMAC, nil, eventChannel, mockiphelper)

	assert.Error(t, err)
}

// The following tests would lead to testing of ReconcileOnce, Init and getAllInterfaces methods

// Test for ReconcileOnce method. In this test case, we have two ENI's registered with StateManager with one still pending acknowledgement
// Our method should send an ENI status change for the first ENI
func TestReconcileOnce(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	eventChannel := make(chan statechange.Event)

	waitForEvents := sync.WaitGroup{}

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	mockStateManager.EXPECT().ENIByMac(macAddress1).
		Return(&apieni.ENIAttachment{
			MACAddress:       macAddress1,
			AttachStatusSent: false,
			ExpiresAt:        time.Now().Add(expirationTimeAddition),
		}, true)
	mockStateManager.EXPECT().ENIByMac(macAddress2).Do(
		func(mac string) {
			waitForEvents.Done()
		}).
		Return(&apieni.ENIAttachment{
			MACAddress:       macAddress2,
			AttachStatusSent: true,
		}, true)
	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	mockiphelper.EXPECT().Start(gomock.Any()).Return(nil)
	mockNetworkUtils := mock_networkutils.NewMockNetworkUtils(mockCtrl)
	mockNetworkUtils.EXPECT().GetAllNetworkInterfaces().Return(constructInterfaceList(), nil)

	watcher, _ := newTestWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)
	watcher.netutils = mockNetworkUtils

	waitForEvents.Add(2)

	go func() {
		event := <-eventChannel
		assert.NotNil(t, event.(api.TaskStateChange).Attachment)
		assert.Equal(t, macAddress1, event.(api.TaskStateChange).Attachment.MACAddress)
		waitForEvents.Done()
	}()

	err := watcher.Init()
	waitForEvents.Wait()

	assert.NoError(t, err)

	select {
	case <-eventChannel:
		t.Errorf("No more events expected.")
	default:
	}
}

// Test for ReconcileOnce method. In this test case, we receive an error connecting with the host to obtain network interfaces
// Our method should also return an error in this case
func TestReconcileOnceNetUtilsError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	eventChannel := make(chan statechange.Event)

	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	mockiphelper.EXPECT().Start(gomock.Any()).Return(nil)
	mockNetworkUtils := mock_networkutils.NewMockNetworkUtils(mockCtrl)
	mockNetworkUtils.EXPECT().GetAllNetworkInterfaces().Return(nil, errors.New("Error while retrieving interfaces"))

	watcher, _ := newTestWatcher(ctx, primaryMAC, nil, eventChannel, mockiphelper)
	watcher.netutils = mockNetworkUtils

	err := watcher.Init()
	assert.Error(t, err)
}

// Test for ReconcileOnce method. We receive an empty interface list in this case.
// Therefore, we return with an error as well
func TestReconcileOnceEmptyInterfaceList(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	eventChannel := make(chan statechange.Event)

	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	mockiphelper.EXPECT().Start(gomock.Any()).Return(nil)
	mockNetworkUtils := mock_networkutils.NewMockNetworkUtils(mockCtrl)
	mockNetworkUtils.EXPECT().GetAllNetworkInterfaces().Return(make([]net.Interface, 0), nil)

	watcher, _ := newTestWatcher(ctx, primaryMAC, nil, eventChannel, mockiphelper)
	watcher.netutils = mockNetworkUtils

	err := watcher.Init()
	assert.NoError(t, err)
}

// The following tests would cover eventHandler method

// Test for eventHandle method. In this test case, IPHelper initiates a callback for interface with index as interfaceIndex1.
// EventHandle will verify that this interface is an ECS managed ENI and will send a StatusChange notification for the same.
// Only one message should be sent corresponding to the added ENI
func TestEventHandlerSuccess(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	eventChannel := make(chan statechange.Event)

	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	mockNetworkUtils := mock_networkutils.NewMockNetworkUtils(mockCtrl)

	gomock.InOrder(
		mockiphelper.EXPECT().Start(gomock.Any()).Return(nil),
		mockNetworkUtils.EXPECT().GetInterfaceMACByIndex(interfaceIndex1, gomock.Any(), sendENIStateChangeRetryTimeout).Return(macAddress1, nil),
		mockStateManager.EXPECT().ENIByMac(macAddress1).
			Return(&apieni.ENIAttachment{
				MACAddress: macAddress1,
				ExpiresAt:  time.Now().Add(expirationTimeAddition),
			}, true),
	)

	watcher, _ := newTestWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)
	watcher.netutils = mockNetworkUtils

	go watcher.eventHandler()
	watcher.notifications <- interfaceIndex1
	var event statechange.Event
	select {
	case event = <-eventChannel:
		break
	case <-time.After(time.Second):
		t.FailNow()
	}
	taskStateChange, ok := event.(api.TaskStateChange)

	assert.True(t, ok)
	assert.Equal(t, apieni.ENIAttached, taskStateChange.Attachment.Status)

	var wait sync.WaitGroup
	wait.Add(1)

	mockiphelper.EXPECT().Close().DoAndReturn(
		func() error {
			wait.Done()
			return nil
		})
	go watcher.Stop()
	wait.Wait()
}

// Test for eventHandle method. In this method, we get an error while connecting to host for finding MAC of the added interface.
func TestEventHandlerGetInterfaceByMACError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	eventChannel := make(chan statechange.Event)

	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	mockNetworkUtils := mock_networkutils.NewMockNetworkUtils(mockCtrl)

	gomock.InOrder(
		mockiphelper.EXPECT().Start(gomock.Any()).Return(nil),
		mockNetworkUtils.EXPECT().GetInterfaceMACByIndex(interfaceIndex1, gomock.Any(), sendENIStateChangeRetryTimeout).Return(
			"", errors.New("Error while retrieving details")),
	)

	watcher, _ := newTestWatcher(ctx, primaryMAC, nil, eventChannel, mockiphelper)
	watcher.netutils = mockNetworkUtils

	go watcher.eventHandler()
	watcher.notifications <- interfaceIndex1

	time.Sleep(cancelContextDelay)
	var wait sync.WaitGroup
	wait.Add(1)

	mockiphelper.EXPECT().Close().DoAndReturn(
		func() error {
			wait.Done()
			return nil
		})
	go watcher.Stop()
	wait.Wait()
}

// Test for eventHandle method. In this case, we will receive notification about an ENI whose attachment status has already been sent.
// We will not resend the same
func TestEventHandlerENIStatusAlreadySent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	eventChannel := make(chan statechange.Event)

	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	mockNetworkUtils := mock_networkutils.NewMockNetworkUtils(mockCtrl)

	gomock.InOrder(
		mockiphelper.EXPECT().Start(gomock.Any()).Return(nil),
		mockNetworkUtils.EXPECT().GetInterfaceMACByIndex(interfaceIndex1, gomock.Any(), sendENIStateChangeRetryTimeout).Return(macAddress1, nil),
		mockStateManager.EXPECT().ENIByMac(macAddress1).
			Return(&apieni.ENIAttachment{
				MACAddress:       macAddress1,
				AttachStatusSent: true,
			}, true).AnyTimes(),
	)

	watcher, _ := newTestWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)
	watcher.netutils = mockNetworkUtils

	go watcher.eventHandler()
	watcher.notifications <- interfaceIndex1

	time.Sleep(cancelContextDelay)
	var wait sync.WaitGroup
	wait.Add(1)

	mockiphelper.EXPECT().Close().DoAndReturn(
		func() error {
			wait.Done()
			return nil
		})
	go watcher.Stop()
	wait.Wait()
}

// Test for eventHandle method. In this test case, we will receive notification about an ENI which is not registered with ECS
// Therefore, we will raise UnmanagedENIError
func TestEventHandlerUnmanagedENI(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	eventChannel := make(chan statechange.Event)

	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	mockNetworkUtils := mock_networkutils.NewMockNetworkUtils(mockCtrl)

	gomock.InOrder(
		mockiphelper.EXPECT().Start(gomock.Any()).Return(nil),
		mockNetworkUtils.EXPECT().GetInterfaceMACByIndex(interfaceIndex1, gomock.Any(), sendENIStateChangeRetryTimeout).Return(macAddress1, nil),
		mockStateManager.EXPECT().ENIByMac(macAddress1).
			Return(nil, false).AnyTimes(),
	)

	watcher, _ := newTestWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)
	watcher.netutils = mockNetworkUtils

	go watcher.eventHandler()
	watcher.notifications <- interfaceIndex1

	time.Sleep(cancelContextDelay)
	var wait sync.WaitGroup
	wait.Add(1)

	mockiphelper.EXPECT().Close().DoAndReturn(
		func() error {
			wait.Done()
			return nil
		})
	go watcher.Stop()
	wait.Wait()
}

// Test for eventHandle method. In this test case, we receive a notification about an ENI whose timer has already expired.
// Therefore, that ENI will be removed from State
func TestEventHandlerExpiredENI(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	eventChannel := make(chan statechange.Event)

	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	mockNetworkUtils := mock_networkutils.NewMockNetworkUtils(mockCtrl)

	gomock.InOrder(
		mockiphelper.EXPECT().Start(gomock.Any()).Return(nil),
		mockNetworkUtils.EXPECT().GetInterfaceMACByIndex(interfaceIndex1, gomock.Any(), sendENIStateChangeRetryTimeout).Return(macAddress1, nil),
		mockStateManager.EXPECT().ENIByMac(macAddress1).
			Return(&apieni.ENIAttachment{
				MACAddress: macAddress1,
				ExpiresAt:  time.Now().Add(expirationTimeSubtraction),
			}, true),
		mockStateManager.EXPECT().RemoveENIAttachment(macAddress1),
	)

	watcher, _ := newTestWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)
	watcher.netutils = mockNetworkUtils

	go watcher.eventHandler()
	watcher.notifications <- interfaceIndex1

	time.Sleep(cancelContextDelay)
	var wait sync.WaitGroup
	wait.Add(1)

	mockiphelper.EXPECT().Close().DoAndReturn(
		func() error {
			wait.Done()
			return nil
		})
	go watcher.Stop()
	wait.Wait()
}
