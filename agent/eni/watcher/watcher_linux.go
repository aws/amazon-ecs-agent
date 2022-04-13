//go:build linux
// +build linux

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
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/logger/field"

	log "github.com/cihub/seelog"
	"github.com/deniswernert/udev"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/eni/netlinkwrapper"
	"github.com/aws/amazon-ecs-agent/agent/eni/networkutils"
	"github.com/aws/amazon-ecs-agent/agent/eni/udevwrapper"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
)

const (
	// linkTypeDevice defines the string that's expected to be the output of
	// netlink.Link.Type() method for netlink.Device type
	linkTypeDevice = "device"

	// encapTypeLoopback defines the string that's set for the link.Attrs.EncapType
	// field for localhost devices. The EncapType field defines the link
	// encapsulation method. For localhost, it's set to "loopback"
	encapTypeLoopback = "loopback"
)

// ENIWatcher maintains the state of attached ENIs
// to the instance. It also has supporting elements to
// maintain consistency and update intervals
type ENIWatcher struct {
	ctx                  context.Context
	cancel               context.CancelFunc
	updateIntervalTicker *time.Ticker
	agentState           dockerstate.TaskEngineState
	eniChangeEvent       chan<- statechange.Event
	primaryMAC           string
	netlinkClient        netlinkwrapper.NetLink
	udevMonitor          udevwrapper.Udev
	events               chan *udev.UEvent
}

// newWatcher is used to nest the return of the ENIWatcher struct
func newWatcher(ctx context.Context,
	primaryMAC string,
	state dockerstate.TaskEngineState,
	stateChangeEvents chan<- statechange.Event) (*ENIWatcher, error) {

	udevWrap, err := udevwrapper.New()
	if err != nil {
		return nil, errors.Wrapf(err, "unable to create udev monitor")
	}

	nlWrap := netlinkwrapper.New()

	derivedContext, cancel := context.WithCancel(ctx)
	log.Info("eni watcher has been initialized")
	return &ENIWatcher{
		ctx:            derivedContext,
		cancel:         cancel,
		agentState:     state,
		eniChangeEvent: stateChangeEvents,
		primaryMAC:     primaryMAC,
		netlinkClient:  nlWrap,
		udevMonitor:    udevWrap,
		events:         make(chan *udev.UEvent),
	}, nil
}

// reconcileOnce is used to reconcile the state of ENIs attached to the instance
func (eniWatcher *ENIWatcher) reconcileOnce(withRetry bool) error {
	links, err := eniWatcher.netlinkClient.LinkList()
	if err != nil {
		return errors.Wrapf(err, "eni watcher: unable to retrieve network interfaces")
	}

	// Return on empty list
	if len(links) == 0 {
		log.Info("ENI watcher reconciliation: no network interfaces discovered for reconciliation")
		return nil
	}

	currentState := eniWatcher.buildState(links)

	// NOTE: For correct semantics, this entire function needs to be locked.
	// As we postulate the netlinkClient.LinkList() call to be expensive, we allow
	// the race here. The state would be corrected during the next reconciliation loop.

	// Add new interfaces next
	for mac := range currentState {
		if withRetry {
			go func(ctx context.Context, macAddress string, timeout time.Duration) {
				if err := eniWatcher.sendENIStateChangeWithRetries(ctx, macAddress, timeout); err != nil {
					logger.Error("Unable to send state ENI change", logger.Fields{
						field.Error: err,
					})
				}
			}(eniWatcher.ctx, mac, sendENIStateChangeRetryTimeout)
			continue
		}
		if err := eniWatcher.sendENIStateChange(mac); err != nil {
			// skip logging status sent error as it's redundant and doesn't really indicate a problem
			if strings.Contains(err.Error(), eniStatusSentMsg) {
				continue
			} else if _, ok := err.(*unmanagedENIError); ok {
				logger.Debug("Unable to send state ENI change", logger.Fields{
					field.Error: err,
				})
			} else {
				logger.Warn("Unable to send state ENI change", logger.Fields{
					field.Error: err,
				})
			}
		}
	}
	return nil
}

// buildState is used to build a state of the system for reconciliation
func (eniWatcher *ENIWatcher) buildState(links []netlink.Link) map[string]string {
	state := make(map[string]string)
	for _, link := range links {
		if link.Type() != linkTypeDevice {
			// We only care about netlink.Device types. These are created
			// by udev like 'lo' and 'eth0'. Ignore other link types
			continue
		}
		if link.Attrs().EncapType == encapTypeLoopback {
			// Ignore localhost
			continue
		}
		macAddress := link.Attrs().HardwareAddr.String()
		if macAddress != "" && macAddress != eniWatcher.primaryMAC {
			state[macAddress] = link.Attrs().Name
		}
	}
	return state
}

// eventHandler is used to manage udev net subsystem events to add/remove interfaces
func (eniWatcher *ENIWatcher) eventHandler() {
	// The shutdown channel will be used to terminate the watch for udev events
	shutdown := eniWatcher.udevMonitor.Monitor(eniWatcher.events)
	for {
		select {
		case event := <-eniWatcher.events:
			subsystem, ok := event.Env[udevSubsystem]
			if !ok || subsystem != udevNetSubsystem {
				continue
			}
			if event.Env[udevEventAction] != udevAddEvent {
				continue
			}
			if !networkutils.IsValidNetworkDevice(event.Env[udevDevPath]) {
				log.Debugf("ENI watcher event handler: ignoring event for invalid network device: %s", event.String())
				continue
			}
			netInterface := event.Env[udevInterface]
			// GetMACAddres and sendENIStateChangeWithRetries can block the execution
			// of this method for a few seconds in the worst-case scenario.
			// Execute these within a go-routine
			go func(ctx context.Context, dev string, timeout time.Duration) {
				log.Debugf("ENI watcher event-handler: add interface: %s", dev)
				macAddress, err := networkutils.GetMACAddress(eniWatcher.ctx, macAddressRetryTimeout,
					dev, eniWatcher.netlinkClient)
				if err != nil {
					log.Warnf("ENI watcher event-handler: error obtaining MACAddress for interface %s", dev)
					return
				}

				if err := eniWatcher.sendENIStateChangeWithRetries(ctx, macAddress, timeout); err != nil {
					log.Warnf("ENI watcher event-handler: unable to send state change: %v", err)
				}
			}(eniWatcher.ctx, netInterface, sendENIStateChangeRetryTimeout)
		case <-eniWatcher.ctx.Done():
			log.Info("Stopping udev event handler")
			// Send the shutdown signal and close the connection
			shutdown <- true
			if err := eniWatcher.udevMonitor.Close(); err != nil {
				log.Warnf("Unable to close the udev monitoring socket: %v", err)
			}
			return
		}
	}
}

// InjectFields is used to inject mock services.
func (eniWatcher *ENIWatcher) InjectFields(udevMonitor udevwrapper.Udev) {
	if udevMonitor != nil {
		eniWatcher.udevMonitor = udevMonitor
	}
}
