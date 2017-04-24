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
	"sync"
	"time"

	log "github.com/cihub/seelog"
	"github.com/deniswernert/udev"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"

	"github.com/aws/amazon-ecs-agent/agent/eni/netlinkWrapper"
	"github.com/aws/amazon-ecs-agent/agent/eni/udevWrapper"
	eniUtils "github.com/aws/amazon-ecs-agent/agent/eni/utils"
)

const (
	udevSubsystem                 = "SUBSYSTEM"
	udevNetSubsystem              = "net"
	udevPCISubsystem              = "pci"
	udevEventAction               = "ACTION"
	udevAddEvent                  = "add"
	udevRemoveEvent               = "remove"
	udevDevPath                   = "DEVPATH"
	udevInterface                 = "INTERFACE"
	defaultReconciliationInterval = time.Second * 30
	mapCapacityHint               = 10
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
	updateLock           sync.RWMutex
	updateIntervalTicker *time.Ticker
	enis                 map[string]string // MAC => Device-Name
	netlinkClient        netlinkWrapper.NetLink
	udevMonitor          udevWrapper.Udev
	events               chan *udev.UEvent
}

// New is used to return an instance of the UdevWatcher struct
func New(ctx context.Context, nlWrap netlinkWrapper.NetLink, udevWrap udevWrapper.Udev) *UdevWatcher {
	derivedContext, cancel := context.WithCancel(ctx)
	return &UdevWatcher{
		ctx:           derivedContext,
		cancel:        cancel,
		enis:          make(map[string]string, mapCapacityHint),
		netlinkClient: nlWrap,
		udevMonitor:   udevWrap,
		events:        make(chan *udev.UEvent),
	}
}

// Init initializes a new ENI Watcher
func (udevWatcher *UdevWatcher) Init() error {
	links, err := udevWatcher.netlinkClient.LinkList()
	if err != nil {
		return errors.Wrapf(err, "init udevWatcher: error retrieving network interfaces")
	}

	udevWatcher.updateLock.Lock()
	for _, link := range links {
		deviceName := link.Attrs().Name
		macAddress := link.Attrs().HardwareAddr.String()
		if macAddress != "" {
			udevWatcher.addDeviceWithMACAddressUnsafe(deviceName, macAddress)
		}
	}
	udevWatcher.updateLock.Unlock()

	// UDev Event Handler
	go udevWatcher.eventHandler(udevWatcher.ctx)
	return nil
}

// Start periodically updates the state of ENIs connected to the system
func (udevWatcher *UdevWatcher) Start() {
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
	udevWatcher.updateLock.Lock()
	defer udevWatcher.updateLock.Unlock()

	// Remove non-existent interfaces first
	for managedMACAddress, managedDeviceName := range udevWatcher.enis {
		if currentDeviceName, ok := currentState[managedMACAddress]; !ok || managedDeviceName != currentDeviceName {
			udevWatcher.removeDeviceWithMACAddressUnsafe(managedMACAddress)
		}
	}

	// Add new interfaces next
	for mac, dev := range currentState {
		if _, ok := udevWatcher.enis[mac]; !ok && mac != "" {
			udevWatcher.addDeviceWithMACAddressUnsafe(dev, mac)
		}
	}
}

// getAllENIs is used to retrieve the state observed by the Watcher
func (udevWatcher *UdevWatcher) getAllENIs() map[string]string {
	udevWatcher.updateLock.RLock()
	defer udevWatcher.updateLock.RUnlock()
	return udevWatcher.enis
}

// IsMACAddressPresent checks if the MACAddress belongs to the maintained state
func (udevWatcher *UdevWatcher) IsMACAddressPresent(macAddress string) bool {
	enis := udevWatcher.getAllENIs()
	_, ok := enis[macAddress]
	return ok
}

// addDeviceWithMACAddressUnsafe adds new devices upon initialization
// NOTE: Expects lock to be held prior to update for correct semantics
func (udevWatcher *UdevWatcher) addDeviceWithMACAddressUnsafe(deviceName, macAddress string) {
	log.Debugf("Udev watcher: adding device %s with MAC %s", deviceName, macAddress)
	// Update State
	udevWatcher.enis[macAddress] = deviceName
}

// removeDeviceWithMACAddressUnsafe is used to remove new devices from maintained state
// NOTE: Expects lock to be held prior to update for correct semantics
func (udevWatcher *UdevWatcher) removeDeviceWithMACAddressUnsafe(mac string) {
	log.Debugf("Udev watcher: removing device with MACAddress: %s", mac)

	if _, ok := udevWatcher.enis[mac]; !ok {
		log.Warnf("Udev watcher: device with MACAddress: %s missing from managed state", mac)
	}

	delete(udevWatcher.enis, mac)
}

// removeDeviceUnsafe is used to remove new devices from uDev events
// NOTE: removeDeviceUnsafe expects lock to be held prior to update for correct semantics
func (udevWatcher *UdevWatcher) removeDeviceUnsafe(deviceName string) {
	log.Debugf("Udev watcher: removing device: %s", deviceName)

	for mac, dev := range udevWatcher.enis {
		if dev == deviceName {
			udevWatcher.removeDeviceWithMACAddressUnsafe(mac)
			return
		}
	}
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
				macAddress, err := eniUtils.GetMACAddress(netInterface, udevWatcher.netlinkClient)
				if err != nil {
					log.Warnf("Udev watcher event-handler: error obtaining MACAddress for interface %s", netInterface)
					continue
				}
				udevWatcher.updateLock.Lock()
				udevWatcher.addDeviceWithMACAddressUnsafe(netInterface, macAddress)
				udevWatcher.updateLock.Unlock()
			case udevRemoveEvent:
				netInterface := event.Env[udevInterface]
				udevWatcher.updateLock.Lock()
				udevWatcher.removeDeviceUnsafe(netInterface)
				udevWatcher.updateLock.Unlock()
			}
		case <-ctx.Done():
			return
		}
	}
}
