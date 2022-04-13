//go:build windows
// +build windows

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
	"math"
	"syscall"
	"time"
	"unsafe"

	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

const (
	// After registering a callback with NotifyIPInterfaceChange API, we get an initial notification to confirm that the callback is registered
	// All other notifications sent on the channel would be positive except this one
	InitialNotification = math.MinInt16
)

var (
	// notificationChannel is used for communication between the notification callback from Windows APIs and the consumer
	notificationChannel chan int

	// initialNotificationTimeout is the timeout for the initial notification.
	// If notification is not received within this timeout then we will assume error
	// We have made this as a var so as to override this value in tests
	initialNotificationTimeout = 4 * time.Second
)

// InterfaceMonitor is used for monitoring interface additions
// InterfaceMonitor can be mocked while unit testing other packages
type InterfaceMonitor interface {
	Start(notification chan int) error
	Close() error
}

type eniMonitor struct {
	notificationHandle          windows.Handle
	funcNotifyIPInterfaceChange func(a ...uintptr) (r1 uintptr, r2 uintptr, lastErr error)
	funcCancelMibChangeNotify2  func(a ...uintptr) (r1 uintptr, r2 uintptr, lastErr error)
}

// NewMonitor creates a new monitor to implement the interface
func NewMonitor() InterfaceMonitor {
	monitor := &eniMonitor{}

	moduleiphlpapi := windows.NewLazySystemDLL("iphlpapi.dll")
	procNotifyIPInterfaceChange := moduleiphlpapi.NewProc("NotifyIpInterfaceChange")
	procCancelMibChangeNotify2 := moduleiphlpapi.NewProc("CancelMibChangeNotify2")

	monitor.funcNotifyIPInterfaceChange = procNotifyIPInterfaceChange.Call
	monitor.funcCancelMibChangeNotify2 = procCancelMibChangeNotify2.Call

	notificationHandle := windows.Handle(0)
	monitor.notificationHandle = notificationHandle

	return monitor
}

// Start is used to start the monitoring of new interfaces attached to the instance.
// We register a notification callback with Windows API for any changes in instance interfaces (NotifyIPInterfaceChange).
// Initial notification received within timeout is considered as a confirmation of successful registration
// notificationChannel is used for any communication between the callback and the consumer
func (monitor *eniMonitor) Start(notification chan int) error {
	notificationChannel = notification

	log.Debug("IPHelper: Starting the eni monitor. Trying to register the callback from Windows API")
	retVal, _, _ := monitor.funcNotifyIPInterfaceChange(syscall.AF_UNSPEC,
		syscall.NewCallback(callback),
		uintptr(unsafe.Pointer(&(monitor))),
		1,
		uintptr(unsafe.Pointer(&(monitor.notificationHandle))))

	// If there is any error while registering callback then retVal will not be 0. Therefore, we will return with an error.
	if retVal != 0 {
		return errors.Errorf("error occured while calling Windows API: %s", syscall.Errno(retVal))
	}

	// Initial notification is sent immediately once the callback is registered.
	// If the same is not received within a defined timeout then we assume an error and close the monitor
	for {
		select {
		case event := <-notificationChannel:
			if event == InitialNotification {
				log.Debug("IPHelper: NotifyIPInterfaceChange Windows API call successful. ENI Monitor started.")
				return nil
			}
		case <-time.After(initialNotificationTimeout):
			log.Errorf("IPHelper: Initial notification not received within timeout. Closing the Monitor.")
			if retErr := monitor.Close(); retErr != nil {
				return errors.Wrapf(retErr, "initial notification not received")
			}
			return errors.Errorf("initial notification not received while starting ENI monitor")
		}
	}
}

// callback function is invoked by NotifyIPInterfaceChange Windows API
// The api returns the information about changed interface as a MIB_IPInterface_Row struct
// The type of event can be inferred using the notificationType. Since we are concerned about addition of new ENIs,
// therefore we will pass information to notification channel when the event is of interface addition i.e.1
func callback(callerContext, row, notificationType uintptr) uintptr {

	// Notification type 1 means addition of a device. InterfaceIndex would always be a positive value
	if notificationType == 1 {
		rowdata := *(*MibIPInterfaceRow)(unsafe.Pointer(row))
		log.Debugf("Interface added with index: %v", rowdata.InterfaceIndex)
		notificationChannel <- int(rowdata.InterfaceIndex)
	} else if notificationType == 3 {
		// Notification type = 3 means Initial notification. This block would be executed only once i.e. when agent starts
		// Initial notification is sent immediately after registration.
		// Only after sending this, any other types of notification can be sent
		// Reason for goroutine - callback with notification == 3 is called as part of call from Start method
		// notification channel is being listened to in Start.
		// If we send the initial notification on same routine from which Start was called then application hangs
		go func() {
			// In order to confirm the registration of callback, we will send a negative number on the notification channel
			notificationChannel <- InitialNotification
		}()
	}

	return 0
}

// Close can be called to cancel the notification callback.
// Alternatively the notification callback would be cancelled when application exits
func (monitor *eniMonitor) Close() (err error) {
	log.Info("IPHelper: Cancelling the interface change notification callback")
	return monitor.cancelNotification()
}

// cancelNotification is used to cancel the notification callback for any changes in the interfaces attached to the system
// If the call is successful then res is 0. Otherwise we will return the corresponding error.
func (monitor *eniMonitor) cancelNotification() error {
	res, _, _ := monitor.funcCancelMibChangeNotify2(uintptr(monitor.notificationHandle))

	if res != 0 {
		return errors.Errorf("error caused while deregistering from windows notification: %s", syscall.Errno(res))
	}
	return nil
}
