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
	"errors"
	"net"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	mock_gonetwrapper "github.com/aws/amazon-ecs-agent/agent/eni/gonetwrapper/mocks"
	"github.com/aws/amazon-ecs-agent/agent/eni/networkutils"
	"github.com/stretchr/testify/require"

	"testing"

	mock_iphelperwrapper "github.com/aws/amazon-ecs-agent/agent/eni/iphelperwrapper/mocks"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	// MAC Address of the primary interface
	primaryMAC = "02:22:ea:8c:80:ae"

	interfaceIndex1 = 9
	macAddress1     = "02:22:ea:8c:81:dc"
	interfacename1  = "Ethernet 9"

	interfaceIndex2 = 13
	macAddress2     = "02:b5:1d:b3:08:92"
	interfacename2  = "Ethernet 13"

	virtualInerfaceIndex = 35
	virtualMacAddress    = "00:15:5d:cb:fa:26"
	virtualInterfacename = "vEthernet (nat)"
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

// NewWindowsWatcher test case. All works well here and we get a working ENI watcher
func TestNewWindowsWatcherWithoutError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	eventChannel := make(chan statechange.Event)
	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(
		nil)

	_, err := NewWindowsWatcher(ctx, primaryMAC, nil, eventChannel, mockiphelper)

	assert.Nil(t, err)
}

// NewWindowsWatcher test case. An error occurs while starting the monitor in this case. Therefore we get an error
func TestNewWindowsWatcherWithError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	eventChannel := make(chan statechange.Event)
	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(
		errors.New("Error while starting monitor"))

	_, err := NewWindowsWatcher(ctx, primaryMAC, nil, eventChannel, mockiphelper)

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

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	mockStateManager.EXPECT().ENIByMac(macAddress1).
		Return(&apieni.ENIAttachment{
			MACAddress:       macAddress1,
			AttachStatusSent: false,
			ExpiresAt:        time.Now().Add(time.Second * 5),
		}, true)
	mockStateManager.EXPECT().ENIByMac(macAddress2).
		Return(&apieni.ENIAttachment{
			MACAddress:       macAddress2,
			AttachStatusSent: true,
		}, true)
	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(nil)
	mockgonetutils := mock_gonetwrapper.NewMockGolangNetUtils(mockCtrl)
	mockgonetutils.EXPECT().GetAllNetworkInterfaces().Return(constructInterfaceList(), nil)

	watcher, _ := NewWindowsWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)
	netutils := networkutils.GetNetworkUtils()
	netutils.SetGoNetUtils(mockgonetutils)
	watcher.SetNetworkUtils(netutils)

	waitForEvents := sync.WaitGroup{}
	waitForEvents.Add(1)

	go func() {
		event := <-eventChannel
		assert.NotNil(t, event.(api.TaskStateChange).Attachment)
		assert.Equal(t, macAddress1, event.(api.TaskStateChange).Attachment.MACAddress)
		waitForEvents.Done()
	}()

	err := watcher.Init()
	waitForEvents.Wait()

	assert.Nil(t, err)

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
	mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(nil)
	mockgonetutils := mock_gonetwrapper.NewMockGolangNetUtils(mockCtrl)
	mockgonetutils.EXPECT().GetAllNetworkInterfaces().Return(nil, errors.New("Error while retrieving interfaces"))

	watcher, _ := NewWindowsWatcher(ctx, primaryMAC, nil, eventChannel, mockiphelper)
	netutils := networkutils.GetNetworkUtils()
	netutils.SetGoNetUtils(mockgonetutils)
	watcher.SetNetworkUtils(netutils)

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
	mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(nil)
	mockgonetutils := mock_gonetwrapper.NewMockGolangNetUtils(mockCtrl)
	mockgonetutils.EXPECT().GetAllNetworkInterfaces().Return(make([]net.Interface, 0), nil)

	watcher, _ := NewWindowsWatcher(ctx, primaryMAC, nil, eventChannel, mockiphelper)
	netutils := networkutils.GetNetworkUtils()
	netutils.SetGoNetUtils(mockgonetutils)
	watcher.SetNetworkUtils(netutils)

	err := watcher.Init()
	assert.Nil(t, err)
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
	mockgonetutils := mock_gonetwrapper.NewMockGolangNetUtils(mockCtrl)
	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)

	mac1, _ := net.ParseMAC(macAddress1)
	gomock.InOrder(
		mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(nil),
		mockgonetutils.EXPECT().FindInterfaceByIndex(interfaceIndex1).Return(&net.Interface{
			Index:        interfaceIndex1,
			Name:         interfacename1,
			HardwareAddr: mac1,
		}, nil),
		mockStateManager.EXPECT().ENIByMac(macAddress1).
			Return(&apieni.ENIAttachment{
				MACAddress: macAddress1,
				ExpiresAt:  time.Now().Add(time.Second * 5),
			}, true),
	)

	watcher, _ := NewWindowsWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)
	netutils := networkutils.GetNetworkUtils()
	netutils.SetGoNetUtils(mockgonetutils)
	watcher.SetNetworkUtils(netutils)

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
	mockgonetutils := mock_gonetwrapper.NewMockGolangNetUtils(mockCtrl)

	gomock.InOrder(
		mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(nil),
		mockgonetutils.EXPECT().FindInterfaceByIndex(gomock.Any()).Return(nil, errors.New("Error while retrieving details")),
	)

	watcher, _ := NewWindowsWatcher(ctx, primaryMAC, nil, eventChannel, mockiphelper)
	netutils := networkutils.GetNetworkUtils()
	netutils.SetGoNetUtils(mockgonetutils)
	watcher.SetNetworkUtils(netutils)

	go watcher.eventHandler()
	watcher.notifications <- interfaceIndex1

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
	mockgonetutils := mock_gonetwrapper.NewMockGolangNetUtils(mockCtrl)
	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)

	mac1, _ := net.ParseMAC(macAddress1)
	gomock.InOrder(
		mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(nil),
		mockgonetutils.EXPECT().FindInterfaceByIndex(interfaceIndex1).Return(&net.Interface{
			Index:        interfaceIndex1,
			Name:         interfacename1,
			HardwareAddr: mac1,
		}, nil),
		mockStateManager.EXPECT().ENIByMac(macAddress1).
			Return(&apieni.ENIAttachment{
				MACAddress:       macAddress1,
				AttachStatusSent: true,
			}, true),
	)

	watcher, _ := NewWindowsWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)
	netutils := networkutils.GetNetworkUtils()
	netutils.SetGoNetUtils(mockgonetutils)
	watcher.SetNetworkUtils(netutils)

	go watcher.eventHandler()
	watcher.notifications <- interfaceIndex1

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
	mockgonetutils := mock_gonetwrapper.NewMockGolangNetUtils(mockCtrl)
	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)

	mac1, _ := net.ParseMAC(macAddress1)
	gomock.InOrder(
		mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(nil),
		mockgonetutils.EXPECT().FindInterfaceByIndex(interfaceIndex1).Return(&net.Interface{
			Index:        interfaceIndex1,
			Name:         interfacename1,
			HardwareAddr: mac1,
		}, nil),
		mockStateManager.EXPECT().ENIByMac(macAddress1).
			Return(nil, false),
	)

	watcher, _ := NewWindowsWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)
	netutils := networkutils.GetNetworkUtils()
	netutils.SetGoNetUtils(mockgonetutils)
	watcher.SetNetworkUtils(netutils)

	go watcher.eventHandler()
	watcher.notifications <- interfaceIndex1

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
	mockgonetutils := mock_gonetwrapper.NewMockGolangNetUtils(mockCtrl)
	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)

	mac1, _ := net.ParseMAC(macAddress1)
	gomock.InOrder(
		mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(nil),
		mockgonetutils.EXPECT().FindInterfaceByIndex(interfaceIndex1).Return(&net.Interface{
			Index:        interfaceIndex1,
			Name:         interfacename1,
			HardwareAddr: mac1,
		}, nil),
		mockStateManager.EXPECT().ENIByMac(macAddress1).
			Return(&apieni.ENIAttachment{
				MACAddress: macAddress1,
				ExpiresAt:  time.Now().Add(-1 * time.Second),
			}, true),
		mockStateManager.EXPECT().RemoveENIAttachment(macAddress1),
	)

	watcher, _ := NewWindowsWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)
	netutils := networkutils.GetNetworkUtils()
	netutils.SetGoNetUtils(mockgonetutils)
	watcher.SetNetworkUtils(netutils)

	go watcher.eventHandler()
	watcher.notifications <- interfaceIndex1

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

// These are tests for common functionality of Linux and Windows

// Test for SendENIStateChange. We send a StateChange notification for an ECS managed ENI which is yet to expire.
func TestSendENIStateChange(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	eventChannel := make(chan statechange.Event)
	ctx := context.TODO()

	gomock.InOrder(
		mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(nil),
		mockStateManager.EXPECT().ENIByMac(macAddress1).Return(&apieni.ENIAttachment{
			ExpiresAt: time.Now().Add(time.Second * 5),
		}, true),
	)

	watcher, _ := NewWindowsWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)

	go watcher.sendENIStateChange(macAddress1)

	eniChangeEvent := <-eventChannel
	taskStateChange, ok := eniChangeEvent.(api.TaskStateChange)
	require.True(t, ok)
	assert.Equal(t, apieni.ENIAttached, taskStateChange.Attachment.Status)
}

// Test for SendENIStateChange. We call the method for an Unmanaged ENI. Therefore we get an error.
func TestSendENIStateChangeUnmanaged(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	eventChannel := make(chan statechange.Event)
	ctx := context.TODO()

	gomock.InOrder(
		mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(nil),
		mockStateManager.EXPECT().ENIByMac(macAddress1).Return(nil, false),
	)

	watcher, _ := NewWindowsWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)
	assert.Error(t, watcher.sendENIStateChange(macAddress1))
}

// Test for SendENIStateChange. We call the method for an ENI whose state change message has already been sent.
// Therefore, we will not send another message
func TestSendENIStateChangeAlreadySent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	eventChannel := make(chan statechange.Event)
	ctx := context.TODO()

	gomock.InOrder(
		mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(nil),
		mockStateManager.EXPECT().ENIByMac(macAddress1).Return(&apieni.ENIAttachment{
			AttachStatusSent: true,
			ExpiresAt:        time.Now().Add(time.Second * 5),
			MACAddress:       macAddress1,
		}, true),
	)

	watcher, _ := NewWindowsWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)
	assert.Error(t, watcher.sendENIStateChange(macAddress1))
}

// Test for SendENIStateChange. We call the method for an expired ENI.
// The ENI is removed from the state and no new notification is sent
func TestSendENIStateChangeExpired(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	eventChannel := make(chan statechange.Event)
	ctx := context.TODO()

	gomock.InOrder(
		mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(nil),
		mockStateManager.EXPECT().ENIByMac(macAddress1).Return(
			&apieni.ENIAttachment{
				AttachStatusSent: false,
				ExpiresAt:        time.Now().Add(-1 * time.Second),
				MACAddress:       macAddress1,
			}, true),
		mockStateManager.EXPECT().RemoveENIAttachment(macAddress1),
	)

	watcher, _ := NewWindowsWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)
	assert.Error(t, watcher.sendENIStateChange(macAddress1))
}

// Test for SendENIStateChangeWithRetries. We send a notification after retry in this case.
func TestSendENIStateChangeWithRetries(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	eventChannel := make(chan statechange.Event)
	ctx := context.TODO()

	gomock.InOrder(
		mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(nil),
		mockStateManager.EXPECT().ENIByMac(macAddress1).Return(nil, false),
		mockStateManager.EXPECT().ENIByMac(macAddress1).Return(&apieni.ENIAttachment{
			ExpiresAt:  time.Now().Add(5 * time.Second),
			MACAddress: macAddress1,
		}, true),
	)

	watcher, _ := NewWindowsWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)

	go watcher.sendENIStateChangeWithRetries(ctx, macAddress1, sendENIStateChangeRetryTimeout)

	eniChangeEvent := <-eventChannel
	taskStateChange, ok := eniChangeEvent.(api.TaskStateChange)
	require.True(t, ok)
	assert.Equal(t, apieni.ENIAttached, taskStateChange.Attachment.Status)
}

// Test for SendENIStateChangeWithRetries. We call this method for an expired ENI.
// Therefore, we remove the ENI from state and return without any message on channel
func TestSendENIStateChangeWithRetriesDoesNotRetryExpiredENI(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	ctx := context.TODO()

	gomock.InOrder(
		mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(nil),
		// ENIByMAC returns an error for exipred ENI attachment, which should
		// mean that it doesn't get retried.
		mockStateManager.EXPECT().ENIByMac(macAddress1).Return(
			&apieni.ENIAttachment{
				AttachStatusSent: false,
				ExpiresAt:        time.Now().Add(time.Second * -1),
				MACAddress:       macAddress1,
			}, true),
		mockStateManager.EXPECT().RemoveENIAttachment(macAddress1),
	)

	watcher, _ := NewWindowsWatcher(ctx, primaryMAC, mockStateManager, nil, mockiphelper)

	assert.Error(t, watcher.sendENIStateChangeWithRetries(
		ctx, macAddress1, sendENIStateChangeRetryTimeout))
}

// TestSendENIStateChangeWithAttachmentTypeInstanceENI tests that we send the attachment state change
// of an instance level eni as an attachment state change
func TestSendENIStateChangeWithAttachmentTypeInstanceENI(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	eventChannel := make(chan statechange.Event)
	ctx := context.TODO()

	gomock.InOrder(
		mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(nil),
		mockStateManager.EXPECT().ENIByMac(macAddress1).Return(&apieni.ENIAttachment{
			AttachmentType: apieni.ENIAttachmentTypeInstanceENI,
			ExpiresAt:      time.Now().Add(5 * time.Second),
		}, true),
	)

	watcher, _ := NewWindowsWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)

	go watcher.sendENIStateChange(macAddress1)

	eniChangeEvent := <-eventChannel
	attachmentStateChange, ok := eniChangeEvent.(api.AttachmentStateChange)
	require.True(t, ok)
	assert.Equal(t, apieni.ENIAttached, attachmentStateChange.Attachment.Status)
}

// TestSendENIStateChangeWithAttachmentTypeTaskENI tests that we send the attachment state change
// of a regular eni as a task state change
func TestSendENIStateChangeWithAttachmentTypeTaskENI(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStateManager := mock_dockerstate.NewMockTaskEngineState(mockCtrl)
	mockiphelper := mock_iphelperwrapper.NewMockInterfaceMonitor(mockCtrl)
	eventChannel := make(chan statechange.Event)
	ctx := context.TODO()

	gomock.InOrder(
		mockiphelper.EXPECT().StartMonitor(gomock.Any()).Return(nil),
		mockStateManager.EXPECT().ENIByMac(macAddress1).Return(&apieni.ENIAttachment{
			AttachmentType: apieni.ENIAttachmentTypeTaskENI,
			ExpiresAt:      time.Now().Add(5 * time.Second),
		}, true),
	)

	watcher, _ := NewWindowsWatcher(ctx, primaryMAC, mockStateManager, eventChannel, mockiphelper)

	go watcher.sendENIStateChange(macAddress1)

	eniChangeEvent := <-eventChannel
	taskStateChange, ok := eniChangeEvent.(api.TaskStateChange)
	require.True(t, ok)
	assert.Equal(t, apieni.ENIAttached, taskStateChange.Attachment.Status)
}
