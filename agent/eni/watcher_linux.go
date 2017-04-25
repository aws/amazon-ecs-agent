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
	"time"

	log "github.com/cihub/seelog"
	"github.com/deniswernert/udev"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"

	"github.com/aws/amazon-ecs-agent/agent/eni/netlinkwrapper"
	eniUtils "github.com/aws/amazon-ecs-agent/agent/eni/networkutils"
	"github.com/aws/amazon-ecs-agent/agent/eni/udevwrapper"
)

// Watcher exposes a method to check if MACAddress is present in observed state
type Watcher interface {
	// IsMACAddressPresent returns a boolean to determine if MACAddress is present in observed state
	IsMACAddressPresent(mac string) bool
}

// UdevWatcher maintains the state of attached ENIs
// to the instance. It also has supporting elements to
// maintain consistency and update intervals
type UdevWatcher struct {
	ctx                  context.Context
	cancel               context.CancelFunc
	updateIntervalTicker *time.Ticker
	netlinkClient        netlinkwrapper.NetLink
	udevMonitor          udevwrapper.Udev
	events               chan *udev.UEvent
	state                StateManagerInterface
}

// New is used to return an instance of the UdevWatcher struct
func New(ctx context.Context, udevwrap udevwrapper.Udev) *UdevWatcher {
	return _new(ctx, netlinkwrapper.NetLinkClient{}, udevwrap, NewStateManager())
}

// _new is used to nest the return of the UdevWatcher struct
func _new(ctx context.Context, nlWrap netlinkwrapper.NetLink, udevWrap udevwrapper.Udev, eniState StateManagerInterface) *UdevWatcher {
	derivedContext, cancel := context.WithCancel(ctx)
	return &UdevWatcher{
		ctx:           derivedContext,
		cancel:        cancel,
		netlinkClient: nlWrap,
		udevMonitor:   udevWrap,
		events:        make(chan *udev.UEvent),
		state:         eniState,
	}
}

// Init initializes a new ENI Watcher
func (udevWatcher *UdevWatcher) Init() error {
	links, err := udevWatcher.netlinkClient.LinkList()
	if err != nil {
		return errors.Wrapf(err, "udev watcher init: error retrieving network interfaces")
	}

	udevWatcher.state.Lock()
	for _, link := range links {
		deviceName := link.Attrs().Name
		macAddress := link.Attrs().HardwareAddr.String()
		if macAddress != "" {
			udevWatcher.addDeviceWithMACAddressUnsafe(deviceName, macAddress)
		}
	}
	udevWatcher.state.Unlock()
	return nil
}

// Start periodically updates the state of ENIs connected to the system
func (udevWatcher *UdevWatcher) Start() {
	// UDev Event Handler
	go udevWatcher.eventHandler(udevWatcher.ctx)
	udevWatcher.performPeriodicReconciliation(udevWatcher.ctx, defaultReconciliationInterval)
}

// Stop is used to invoke the cancellation routine
func (udevWatcher *UdevWatcher) Stop() {
	udevWatcher.cancel()
}

// performPeriodicReconciliation is used to periodically invoke the
// reconciliation process based on a ticker
func (udevWatcher *UdevWatcher) performPeriodicReconciliation(ctx context.Context, updateInterval time.Duration) {
	udevWatcher.updateIntervalTicker = time.NewTicker(updateInterval)
	for {
		select {
		case <-udevWatcher.updateIntervalTicker.C:
			udevWatcher.reconcileOnce()
		case <-ctx.Done():
			udevWatcher.updateIntervalTicker.Stop()
			return
		}
	}
}

// reconcileOnce is used to reconcile the state of ENIs attached to the instance
func (udevWatcher *UdevWatcher) reconcileOnce() {
	log.Debugf("Udev watcher reconciliation: begin")
	links, err := udevWatcher.netlinkClient.LinkList()
	if err != nil {
		log.Warnf("Udev watcher reconciliation: error retrieving network interfaces: %v", err)
		return
	}

	// Return on empty list
	if len(links) == 0 {
		log.Info("Udev watcher reconciliation: no network interfaces discovered for reconciliation")
		return
	}

	currentState := udevWatcher.buildState(links)

	// NOTE: For correct semantics, this entire function needs to be locked.
	// As we postulate the netlinkClient.LinkList() call to be expensive, we allow
	// the race here. The state would be corrected during the next reconciliation loop.

	udevWatcher.state.Lock()
	defer udevWatcher.state.Unlock()

	// Remove non-existent interfaces first
	enis := udevWatcher.state.GetAll()
	for managedMACAddress, managedDeviceName := range enis {
		if currentDeviceName, ok := currentState[managedMACAddress]; !ok || managedDeviceName != currentDeviceName {
			udevWatcher.state.RemoveDeviceWithMACAddressUnsafe(managedMACAddress)
		}
	}

	// Add new interfaces next
	for mac, dev := range currentState {
		if _, ok := enis[mac]; !ok && mac != "" {
			udevWatcher.state.AddDeviceWithMACAddressUnsafe(dev, mac)
		}
	}
	log.Debugf("Udev watcher reconciliation: end")
}

// getAllENIs is used to retrieve the state observed by the Watcher
func (udevWatcher *UdevWatcher) getAllENIs() map[string]string {
	return udevWatcher.state.GetAll()
}

// IsMACAddressPresent checks if the MACAddress belongs to the maintained state
func (udevWatcher *UdevWatcher) IsMACAddressPresent(macAddress string) bool {
	udevWatcher.state.Lock()
	defer udevWatcher.state.Unlock()
	enis := udevWatcher.getAllENIs()
	_, ok := enis[macAddress]
	return ok
}

// addDeviceWithMACAddressUnsafe adds new devices upon initialization
// NOTE: Expects lock to be held prior to update for correct semantics
func (udevWatcher *UdevWatcher) addDeviceWithMACAddressUnsafe(deviceName, macAddress string) {
	log.Debugf("Udev watcher: adding device %s with MAC %s", deviceName, macAddress)
	udevWatcher.state.AddDeviceWithMACAddressUnsafe(deviceName, macAddress)
}

// removeDeviceWithMACAddressUnsafe is used to remove new devices from maintained state
// NOTE: Expects lock to be held prior to update for correct semantics
func (udevWatcher *UdevWatcher) removeDeviceWithMACAddressUnsafe(mac string) {
	log.Debugf("Udev watcher: removing device with MACAddress: %s", mac)
	udevWatcher.state.RemoveDeviceWithMACAddressUnsafe(mac)
}

// removeDeviceUnsafe is used to remove new devices from uDev events
// NOTE: removeDeviceUnsafe expects lock to be held prior to update for correct semantics
func (udevWatcher *UdevWatcher) removeDeviceUnsafe(deviceName string) {
	log.Debugf("Udev watcher: removing device: %s", deviceName)
	udevWatcher.state.RemoveDeviceUnsafe(deviceName)
}

// buildState is used to build a state of the system for reconciliation
func (udevWatcher *UdevWatcher) buildState(links []netlink.Link) map[string]string {
	state := make(map[string]string, mapCapacityHint)

	for _, link := range links {
		macAddress := link.Attrs().HardwareAddr.String()
		if macAddress != "" {
			state[macAddress] = link.Attrs().Name
		}
	}
	return state
}

// eventHandler is used to manage udev net subsystem events to add/remove interfaces
func (udevWatcher *UdevWatcher) eventHandler(ctx context.Context) {
	udevWatcher.udevMonitor.Monitor(udevWatcher.events)
	for {
		select {
		case event := <-udevWatcher.events:
			subsystem, ok := event.Env[udevSubsystem]
			if !ok || subsystem != udevNetSubsystem {
				continue
			}
			switch event.Env[udevEventAction] {
			case udevAddEvent:
				if !eniUtils.IsValidNetworkDevice(event.Env[udevDevPath]) {
					continue
				}
				netInterface := event.Env[udevInterface]
				log.Debugf("Udev watcher event-handler: add interface: %s", netInterface)
				macAddress, err := eniUtils.GetMACAddress(netInterface, udevWatcher.netlinkClient)
				if err != nil {
					log.Warnf("Udev watcher event-handler: error obtaining MACAddress for interface %s", netInterface)
					continue
				}
				udevWatcher.state.Lock()
				udevWatcher.state.AddDeviceWithMACAddressUnsafe(netInterface, macAddress)
				udevWatcher.state.Unlock()
			case udevRemoveEvent:
				netInterface := event.Env[udevInterface]
				log.Debugf("Udev watcher event-handler: remove interface: %s", netInterface)
				udevWatcher.state.Lock()
				udevWatcher.state.RemoveDeviceUnsafe(netInterface)
				udevWatcher.state.Unlock()
			}
		case <-ctx.Done():
			return
		}
	}
}
