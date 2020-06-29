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

package iphelperwrapper

import (
	"testing"
	"time"
	"unsafe"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	// Initial delay after which Initial notification is sent
	InitialNotificationDelay = time.Millisecond
	// Time delay between two successive notifications
	TimeBetweenNotifications = 100 * time.Millisecond
)

var (
	// Dummy function which is representative of system call (NotifyIPInterfaceChange)
	dummyFuncNotifyIPInterfaceChange = func(...uintptr) (uintptr, uintptr, error) {
		return uintptr(0), uintptr(0), nil
	}

	// Dummy function which is representative of system call (CancelMibChangeNotify2)
	dummyFuncCancelMibChangeNotify2 = func(...uintptr) (uintptr, uintptr, error) {
		return uintptr(0), uintptr(0), nil
	}

	// Dummy information for callback
	notificationInterfaceAddition = MibIPInterfaceRow{
		InterfaceIndex: 9,
	}

	// Dummy information for callback
	notificationInterfaceDeletion = MibIPInterfaceRow{
		InterfaceIndex: 9,
	}
)

// Test for StartMonitor. This is a success test case in which initial notification is sent immediately
func TestStartMonitorInitialNotificationImmediately(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Overriding the windows API call with a dummy call
	funcNotifyIPInterfaceChange = dummyFuncNotifyIPInterfaceChange
	funcCancelMibChangeNotify2 = dummyFuncCancelMibChangeNotify2
	notificationChannel := make(chan int)
	infMonitor := NewMonitor()

	//Send initial notification immediately
	go func() {
		time.Sleep(InitialNotificationDelay)
		callback(uintptr(0), uintptr(0), uintptr(3))
	}()

	err := infMonitor.StartMonitor(notificationChannel)

	assert.Nil(t, err)
}

// Test for StartMonitor. This is a success test case.
// In this test case, initial notification is sent immediately followed by interface addition and deletion callback.
func TestStartMonitorSuccessAddInterfaces(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Overriding the windows API call with a dummy call
	funcNotifyIPInterfaceChange = dummyFuncNotifyIPInterfaceChange
	funcCancelMibChangeNotify2 = dummyFuncCancelMibChangeNotify2
	notificationChannel := make(chan int)
	infMonitor := NewMonitor()

	//Send initial notification immediately
	go func() {
		time.Sleep(InitialNotificationDelay)
		callback(uintptr(0), uintptr(0), uintptr(3))
		time.Sleep(TimeBetweenNotifications)
		callback(uintptr(0), uintptr(unsafe.Pointer(&notificationInterfaceAddition)), uintptr(1))
		time.Sleep(TimeBetweenNotifications)
		callback(uintptr(0), uintptr(unsafe.Pointer(&notificationInterfaceAddition)), uintptr(1))
	}()

	var events = make([]int, 0)
	var expectedEvents = []int{9, 9}

	err := infMonitor.StartMonitor(notificationChannel)

eventloop:
	for {
		select {
		case event := <-notificationChannel:
			events = append(events, event)
		case <-time.After(TimeBetweenNotifications * 3):
			break eventloop
		}
	}
	assert.Nil(t, err)
	assert.Equal(t, expectedEvents, events)

}

// Test for StartMonitor. In this test case, only delete interface callbacks are received.
// No message should be sent on notification channel
func TestStartMonitorSuccessDeleteInterfaces(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Overriding the windows API call with a dummy call
	funcNotifyIPInterfaceChange = dummyFuncNotifyIPInterfaceChange
	funcCancelMibChangeNotify2 = dummyFuncCancelMibChangeNotify2
	notificationChannel := make(chan int)
	infMonitor := NewMonitor()

	//Send initial notification immediately
	go func() {
		time.Sleep(InitialNotificationDelay)
		callback(uintptr(0), uintptr(0), uintptr(3))
		time.Sleep(TimeBetweenNotifications)
		callback(uintptr(0), uintptr(unsafe.Pointer(&notificationInterfaceDeletion)), uintptr(2))
		time.Sleep(TimeBetweenNotifications)
		callback(uintptr(0), uintptr(unsafe.Pointer(&notificationInterfaceDeletion)), uintptr(2))
	}()

	var events = make([]int, 0)
	var expectedEvents = make([]int, 0)

	err := infMonitor.StartMonitor(notificationChannel)

eventloop:
	for {
		select {
		case event := <-notificationChannel:
			events = append(events, event)
		case <-time.After(TimeBetweenNotifications * 3):
			break eventloop
		}
	}
	assert.Nil(t, err)
	assert.Equal(t, expectedEvents, events)
}

// Test for StartMonitor. In this test case, addition and deletion callbacks are received.
// Only interface addition notifications should be sent on communication channel
func TestStartMonitorSuccessAddAndDeleteInterfaces(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Overriding the windows API call with a dummy call
	funcNotifyIPInterfaceChange = dummyFuncNotifyIPInterfaceChange
	funcCancelMibChangeNotify2 = dummyFuncCancelMibChangeNotify2
	notificationChannel := make(chan int)
	infMonitor := NewMonitor()

	//Send initial notification immediately
	go func() {
		time.Sleep(InitialNotificationDelay)
		callback(uintptr(0), uintptr(0), uintptr(3))
		time.Sleep(TimeBetweenNotifications)
		callback(uintptr(0), uintptr(unsafe.Pointer(&notificationInterfaceDeletion)), uintptr(2))
		time.Sleep(TimeBetweenNotifications)
		callback(uintptr(0), uintptr(unsafe.Pointer(&notificationInterfaceAddition)), uintptr(1))
	}()

	var events = make([]int, 0)
	var expectedEvents = []int{9}

	err := infMonitor.StartMonitor(notificationChannel)

eventloop:
	for {
		select {
		case event := <-notificationChannel:
			events = append(events, event)
		case <-time.After(TimeBetweenNotifications * 3):
			break eventloop
		}
	}
	assert.Nil(t, err)
	assert.Equal(t, expectedEvents, events)
}

// Test for StartMonitor. In this test case, no initial notification is received. Therefore, we close the monitor
func TestStartMonitorNoInitialNotification(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Overriding the windows API call with a dummy call
	funcNotifyIPInterfaceChange = dummyFuncNotifyIPInterfaceChange
	funcCancelMibChangeNotify2 = dummyFuncCancelMibChangeNotify2
	notificationChannel := make(chan int)
	infMonitor := NewMonitor()

	err := infMonitor.StartMonitor(notificationChannel)
	// DO not send any initial notification on the notification channel.

	assert.Error(t, err)
}

// Test for StartMonitor. In this test case, no initial notification is received. rather random notifications are sent.
// Therefore, we should exit after the timeout without sending any message on notification channel
func TestStartMonitorNoInitialNotificationButRandomNotification(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Overriding the windows API call with a dummy call
	funcNotifyIPInterfaceChange = dummyFuncNotifyIPInterfaceChange
	funcCancelMibChangeNotify2 = dummyFuncCancelMibChangeNotify2
	notificationChannel := make(chan int)
	infMonitor := NewMonitor()
	//Send random notification after a delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		callback(uintptr(0), uintptr(unsafe.Pointer(&notificationInterfaceAddition)), uintptr(1))
	}()
	err := infMonitor.StartMonitor(notificationChannel)
	// DO not send any initial notification on the notification channel.

	assert.Error(t, err)
}

// Test for StartMonitor. In this test case, we will receive an initial notification after timeout.
// Therefore, we should close the monitor and discard any future callbacks.
func TestStartMonitorInitialNotificationAfterTimeout(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Overriding the windows API call with a dummy call
	funcNotifyIPInterfaceChange = dummyFuncNotifyIPInterfaceChange
	funcCancelMibChangeNotify2 = dummyFuncCancelMibChangeNotify2
	notificationChannel := make(chan int)
	infMonitor := NewMonitor()
	//Send initial notification after timeout
	go func() {
		time.Sleep(InitialNotificationTimeout)
		callback(uintptr(0), uintptr(0), uintptr(3))
	}()
	err := infMonitor.StartMonitor(notificationChannel)
	// DO not send any initial notification on the notification channel.

	assert.Error(t, err)
}
