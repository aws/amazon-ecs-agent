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

package iphelperwrapper

import (
	"testing"
	"time"
	"unsafe"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"
)

const (
	// Time delay between two successive notifications
	TimeBetweenNotifications = 100 * time.Millisecond
)

var (
	// Dummy function which is representative of system call (NotifyIPInterfaceChange)
	dummyFuncNotifyIPInterfaceChange = func(...uintptr) (uintptr, uintptr, error) {
		callback(uintptr(0), uintptr(0), uintptr(3))
		return uintptr(0), uintptr(0), nil
	}

	// Dummy function which is representative of system call (NotifyIPInterfaceChange)
	// Initial notification is not sent in this function
	dummyFuncNotifyIPInterfaceChangeWithoutInitialNotification = func(...uintptr) (uintptr, uintptr, error) {
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

// TestStart is used to test Start method with and without error
func TestStart(t *testing.T) {
	initialNotificationTimeout = time.Second
	var tests = []struct {
		name                        string
		notifyIPInterfaceChangeFunc func(...uintptr) (uintptr, uintptr, error)
		cancelMibChangeNotify2Func  func(...uintptr) (uintptr, uintptr, error)
		error                       bool
	}{
		// Test for Start. In this test case, we receive an error registering the callback.
		// Therefore, our method should return with an error
		{
			name: "ErrorWhileCallingWindowsAPI",
			notifyIPInterfaceChangeFunc: func(...uintptr) (uintptr, uintptr, error) {
				return uintptr(87), uintptr(0), nil
			},
			cancelMibChangeNotify2Func: dummyFuncCancelMibChangeNotify2,
			error:                      true,
		},
		// Test for Start. This is a success test case in which initial notification is sent immediately
		{
			name:                        "InitialNotificationImmediately",
			notifyIPInterfaceChangeFunc: dummyFuncNotifyIPInterfaceChange,
			cancelMibChangeNotify2Func:  dummyFuncCancelMibChangeNotify2,
			error:                       false,
		},
		// Test for Start. In this test case, no initial notification is received. Therefore, we close the monitor
		{
			name:                        "NoInitialNotification",
			notifyIPInterfaceChangeFunc: dummyFuncNotifyIPInterfaceChangeWithoutInitialNotification,
			cancelMibChangeNotify2Func:  dummyFuncCancelMibChangeNotify2,
			error:                       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notificationChannel := make(chan int)
			monitor := &eniMonitor{
				notificationHandle:          windows.Handle(0),
				funcNotifyIPInterfaceChange: tt.notifyIPInterfaceChangeFunc,
				funcCancelMibChangeNotify2:  tt.cancelMibChangeNotify2Func,
			}
			err := monitor.Start(notificationChannel)

			if tt.error {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test for Start. This is a success test case.
// In this test case, initial notification is sent immediately followed by interface addition and deletion callback.
func TestStartSuccessAddInterfaces(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	notificationChannel := make(chan int)
	monitor := &eniMonitor{
		notificationHandle:          windows.Handle(0),
		funcNotifyIPInterfaceChange: dummyFuncNotifyIPInterfaceChange,
		funcCancelMibChangeNotify2:  dummyFuncCancelMibChangeNotify2,
	}

	// Send initial notification immediately
	go func() {
		time.Sleep(TimeBetweenNotifications)
		callback(uintptr(0), uintptr(unsafe.Pointer(&notificationInterfaceAddition)), uintptr(1))
		time.Sleep(TimeBetweenNotifications)
		callback(uintptr(0), uintptr(unsafe.Pointer(&notificationInterfaceAddition)), uintptr(1))
	}()

	var events = make([]int, 0)
	var expectedEvents = []int{9, 9}

	err := monitor.Start(notificationChannel)

	events = append(events, <-notificationChannel)
	events = append(events, <-notificationChannel)

	assert.NoError(t, err)
	assert.Equal(t, expectedEvents, events)

}

// Test for Start. In this test case, only delete interface callbacks are received.
// No message should be sent on notification channel
func TestStartSuccessDeleteInterfaces(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	notificationChannel := make(chan int)
	monitor := &eniMonitor{
		notificationHandle:          windows.Handle(0),
		funcNotifyIPInterfaceChange: dummyFuncNotifyIPInterfaceChange,
		funcCancelMibChangeNotify2:  dummyFuncCancelMibChangeNotify2,
	}

	// Send initial notification immediately
	go func() {
		time.Sleep(TimeBetweenNotifications)
		callback(uintptr(0), uintptr(unsafe.Pointer(&notificationInterfaceDeletion)), uintptr(2))
		time.Sleep(TimeBetweenNotifications)
		callback(uintptr(0), uintptr(unsafe.Pointer(&notificationInterfaceDeletion)), uintptr(2))
	}()

	err := monitor.Start(notificationChannel)

	assert.NoError(t, err)
}

// Test for Start. In this test case, addition and deletion callbacks are received.
// Only interface addition notifications should be sent on communication channel
func TestStartSuccessAddAndDeleteInterfaces(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	notificationChannel := make(chan int)
	monitor := &eniMonitor{
		notificationHandle:          windows.Handle(0),
		funcNotifyIPInterfaceChange: dummyFuncNotifyIPInterfaceChange,
		funcCancelMibChangeNotify2:  dummyFuncCancelMibChangeNotify2,
	}

	// Send initial notification immediately
	go func() {
		time.Sleep(TimeBetweenNotifications)
		callback(uintptr(0), uintptr(unsafe.Pointer(&notificationInterfaceDeletion)), uintptr(2))
		time.Sleep(TimeBetweenNotifications)
		callback(uintptr(0), uintptr(unsafe.Pointer(&notificationInterfaceAddition)), uintptr(1))
	}()

	var events = make([]int, 0)
	var expectedEvents = []int{9}

	err := monitor.Start(notificationChannel)

	events = append(events, <-notificationChannel)

	assert.NoError(t, err)
	assert.Equal(t, expectedEvents, events)
}

// Test for Start. In this test case, no initial notification is received. Therefore, we close the monitor
func TestStartNoInitialNotification(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	notificationChannel := make(chan int)
	monitor := &eniMonitor{
		notificationHandle:          windows.Handle(0),
		funcNotifyIPInterfaceChange: dummyFuncNotifyIPInterfaceChangeWithoutInitialNotification,
		funcCancelMibChangeNotify2:  dummyFuncCancelMibChangeNotify2,
	}

	err := monitor.Start(notificationChannel)
	// Do not send any initial notification on the notification channel.

	assert.Error(t, err)
}

// Test for Start. In this test case, no initial notification is received. rather random notifications are sent.
// Therefore, we should exit after the timeout without sending any message on notification channel
func TestStartNoInitialNotificationButRandomNotification(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	notificationChannel := make(chan int)
	monitor := &eniMonitor{
		notificationHandle:          windows.Handle(0),
		funcNotifyIPInterfaceChange: dummyFuncNotifyIPInterfaceChangeWithoutInitialNotification,
		funcCancelMibChangeNotify2:  dummyFuncCancelMibChangeNotify2,
	}

	// Send random notification after a delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		callback(uintptr(0), uintptr(unsafe.Pointer(&notificationInterfaceAddition)), uintptr(1))
	}()
	err := monitor.Start(notificationChannel)
	// DO not send any initial notification on the notification channel.

	assert.Error(t, err)
}

// Test for Start. In this test case, we will receive an initial notification after timeout.
// Therefore, we should close the monitor and discard any future callbacks.
func TestStartInitialNotificationAfterTimeout(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Overriding the windows API call with a dummy call
	funcNotifyIPInterfaceChange := func(...uintptr) (uintptr, uintptr, error) {
		// Send initial notification after timeout
		go func() {
			time.Sleep(initialNotificationTimeout * 2)
			callback(uintptr(0), uintptr(0), uintptr(3))
		}()
		return uintptr(0), uintptr(0), nil
	}

	notificationChannel := make(chan int)
	monitor := &eniMonitor{
		notificationHandle:          windows.Handle(0),
		funcNotifyIPInterfaceChange: funcNotifyIPInterfaceChange,
		funcCancelMibChangeNotify2:  dummyFuncCancelMibChangeNotify2,
	}

	err := monitor.Start(notificationChannel)
	// Do not send any initial notification on the notification channel.

	assert.Error(t, err)
}
