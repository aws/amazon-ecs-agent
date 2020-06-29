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
	"C"
	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
	"math"
	"syscall"
	"time"
	"unsafe"
)

const (
	// After registering a callback with NotifyIPInterfaceChange API, we get an initial notification to confirm that the callback is registered
	// All other notifications sent on the channel would be positive except this one
	InitialNotification = math.MinInt16
	// This is the timeout for the initial notification. If notification is not received within this timeout then we will assume error
	InitialNotificationTimeout = 3 * time.Second
)

var (
	moduleiphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")

	procNotifyIPInterfaceChange = moduleiphlpapi.NewProc("NotifyIpInterfaceChange")
	proccancelMibChangeNotify2  = moduleiphlpapi.NewProc("CancelMibChangeNotify2")

	//Added these to allow unit testing for this module. We will swap these System calls with dummy calls in unit tests
	funcNotifyIPInterfaceChange = procNotifyIPInterfaceChange.Call
	funcCancelMibChangeNotify2  = proccancelMibChangeNotify2.Call

	// The notification channel is used for communication between the notification callback from Windows APIs and the consumer
	notificationChannel chan int
)

// Interface for using the monitor
// We will mock this interface while unit testing other packages
type InterfaceMonitor interface {
	StartMonitor(notification chan int) error
	Close() error
}

type monitoringInterface struct {
	notificationHandle windows.Handle
}

//Returns a new monitoringInterface which conforms to InterfaceMonitor interface
func NewMonitor() (monitor *monitoringInterface) {
	monitor = &monitoringInterface{}

	notificationHandle := windows.Handle(0)
	monitor.notificationHandle = notificationHandle

	return
}

// This method is used to start the monitoring of new interfaces attached to the instance.
// We register a notification callback with Windows API for any changes in instance interfaces (NotifyIPInterfaceChange).
// Initial notification received within timeout is considered as a confirmation of successful registration
// notificationChannel is used for any communication between the callback and the consumer
func (monitor *monitoringInterface) StartMonitor(notification chan int) error {
	notificationChannel = notification

	log.Info("Starting the monitor")
	_, _, _ = funcNotifyIPInterfaceChange(syscall.AF_UNSPEC,
		syscall.NewCallback(callback),
		uintptr(unsafe.Pointer(&(monitor))),
		1,
		uintptr(unsafe.Pointer(&(monitor.notificationHandle))))

	// Initial notification is sent immediately once the callback is registered.
	// If the same is not received within a defined timeout then we assume an error and close the monitor
initialNotificationLoop:
	for {
		select {
		case event := <-notificationChannel:
			if event == InitialNotification {
				break initialNotificationLoop
			}
		case <-time.After(InitialNotificationTimeout):
			log.Errorf("IPHelper : Initial notification not received within timeout. Closing the Monitor.")
			monitor.Close()
			return errors.Errorf("IPHelper : Initial notification not received.")
		}
	}

	return nil
}

// This is the callback function invoked by NotifyIPInterfaceChange Windows API
// The api returns the information about changed interface as a MIB_IPInterface_Row struct
// The type of event can be inferred using the notificationType. Since we are concerned about addition of new ENIs,
// therefore we will pass information to notification channel when the event is of interface addition
func callback(callerContext, row, notificationType uintptr) uintptr {

	// Notification type 1 means addition of a device. InterfaceIndex would always be a positive value
	if notificationType == 1 {
		rowdata := *(*MibIPInterfaceRow)(unsafe.Pointer(row))
		log.Debugf("Interface added with index : %v", rowdata.InterfaceIndex)
		notificationChannel <- int(rowdata.InterfaceIndex)
	} else if notificationType == 3 {
		// Initial notification is sent immediately after registration.
		// Only after sending this, any other types of notification can be sent
		// Sending this notification on separate goroutine inorder to avoid deadlock with calling function
		go func() {
			time.Sleep(10 * time.Millisecond)
			log.Info("Initial notification received!")
			// In order to confirm the registration of callback, we will send a negative number on the notification channel
			notificationChannel <- InitialNotification
		}()
	}

	return 0
}

// This method can be called to cancel the notification callback.
// Alternatively the notification callback would be cancelled when application exits
func (monitor *monitoringInterface) Close() (err error) {
	log.Info("IPHelper : Cancelling the interface change notification callback")
	res := monitor.cancelNotifications()
	if res != 0 {
		return errors.Errorf("IPHelper : Error caused while deregistering from windows notification : %v", res)
	}
	return nil
}

// This method is used to cancel the notification callback for any changes in the interfaces attached to the system
func (monitor *monitoringInterface) cancelNotifications() int32 {
	res, _, _ := funcCancelMibChangeNotify2(uintptr(monitor.notificationHandle))
	result := int32(res)
	return result
}
