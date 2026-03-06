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

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"

	"github.com/deniswernert/udev"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/eni/networkutils"
	"github.com/aws/amazon-ecs-agent/agent/eni/udevwrapper"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper"
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
	logger.Info("ENI Watcher: initialized", logger.Fields{
		"primaryMAC": primaryMAC,
	})
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
		logger.Info("ENI Watcher: reconciliation found no network interfaces", logger.Fields{
			"primaryMAC": eniWatcher.primaryMAC,
		})
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
					// "already sent" is expected for trunk ENI - the attachment was acknowledged previously
					if strings.Contains(err.Error(), eniStatusSentMsg) {
						logger.Info("ENI Watcher: ENI attachment status already sent (expected for trunk ENI)", logger.Fields{
							"primaryMAC": eniWatcher.primaryMAC,
							"macAddress": macAddress,
						})
						return
					}
					logger.Error("ENI Watcher: unable to send state change", logger.Fields{
						"primaryMAC": eniWatcher.primaryMAC,
						"macAddress": macAddress,
						field.Error:  err,
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
				logger.Debug("ENI Watcher: unable to send state change (unmanaged ENI)", logger.Fields{
					"primaryMAC": eniWatcher.primaryMAC,
					field.Error:  err,
				})
			} else {
				logger.Warn("ENI Watcher: unable to send state change", logger.Fields{
					"primaryMAC": eniWatcher.primaryMAC,
					field.Error:  err,
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
	logger.Info("ENI Watcher: udev event handler started, listening for network device events", logger.Fields{
		"primaryMAC": eniWatcher.primaryMAC,
	})
	for {
		select {
		case event := <-eniWatcher.events:
			subsystem, ok := event.Env[udevSubsystem]
			if !ok || subsystem != udevNetSubsystem {
				// Log non-net subsystem events at Debug level (wakeup, pci, queues, etc.)
				logger.Debug("ENI Watcher: received udev event (non-net subsystem, ignoring)", logger.Fields{
					"primaryMAC": eniWatcher.primaryMAC,
					"action":     event.Env[udevEventAction],
					"subsystem":  subsystem,
					"interface":  event.Env[udevInterface],
					"devpath":    event.Env[udevDevPath],
				})
				continue
			}

			// Log net subsystem events at Info level
			logger.Info("ENI Watcher: received udev event", logger.Fields{
				"primaryMAC": eniWatcher.primaryMAC,
				"action":     event.Env[udevEventAction],
				"subsystem":  event.Env[udevSubsystem],
				"interface":  event.Env[udevInterface],
				"devpath":    event.Env[udevDevPath],
			})
			if event.Env[udevEventAction] != udevAddEvent {
				logger.Debug("ENI Watcher: ignoring non-add event", logger.Fields{
					"primaryMAC": eniWatcher.primaryMAC,
					"action":     event.Env[udevEventAction],
					"interface":  event.Env[udevInterface],
				})
				continue
			}
			if !networkutils.IsValidNetworkDevice(event.Env[udevDevPath]) {
				logger.Debug("ENI Watcher: ignoring event for invalid network device", logger.Fields{
					"primaryMAC": eniWatcher.primaryMAC,
					"devpath":    event.Env[udevDevPath],
					"interface":  event.Env[udevInterface],
				})
				continue
			}
			netInterface := event.Env[udevInterface]
			// GetMACAddres and sendENIStateChangeWithRetries can block the execution
			// of this method for a few seconds in the worst-case scenario.
			// Execute these within a go-routine
			primaryMAC := eniWatcher.primaryMAC
			go func(ctx context.Context, dev string, timeout time.Duration) {
				logger.Debug("ENI Watcher: processing add event for interface", logger.Fields{
					"primaryMAC": primaryMAC,
					"interface":  dev,
				})
				macAddress, err := networkutils.GetMACAddress(eniWatcher.ctx, macAddressRetryTimeout,
					dev, eniWatcher.netlinkClient)
				if err != nil {
					logger.Warn("ENI Watcher: error obtaining MAC address for interface", logger.Fields{
						"primaryMAC": primaryMAC,
						"interface":  dev,
						field.Error:  err,
					})
					return
				}

				logger.Info("ENI Watcher: obtained MAC address for interface, sending state change", logger.Fields{
					"primaryMAC": primaryMAC,
					"interface":  dev,
					"macAddress": macAddress,
				})
				if err := eniWatcher.sendENIStateChangeWithRetries(ctx, macAddress, timeout); err != nil {
					logger.Warn("ENI Watcher: unable to send state change", logger.Fields{
						"primaryMAC": primaryMAC,
						"interface":  dev,
						"macAddress": macAddress,
						field.Error:  err,
					})
				} else {
					logger.Info("ENI Watcher: successfully processed ENI attachment", logger.Fields{
						"primaryMAC": primaryMAC,
						"interface":  dev,
						"macAddress": macAddress,
					})
				}
			}(eniWatcher.ctx, netInterface, sendENIStateChangeRetryTimeout)
		case <-eniWatcher.ctx.Done():
			logger.Info("ENI Watcher: stopping udev event handler", logger.Fields{
				"primaryMAC": eniWatcher.primaryMAC,
			})
			// Send the shutdown signal and close the connection
			shutdown <- true
			if err := eniWatcher.udevMonitor.Close(); err != nil {
				logger.Warn("ENI Watcher: unable to close udev monitoring socket", logger.Fields{
					"primaryMAC": eniWatcher.primaryMAC,
					field.Error:  err,
				})
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
