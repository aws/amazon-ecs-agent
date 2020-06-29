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

package watcher

import (
	"context"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/eni/iphelperwrapper"
	"github.com/aws/amazon-ecs-agent/agent/eni/networkutils"

	log "github.com/cihub/seelog"
	"github.com/pkg/errors"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
)

const (
	defaultVirtualAdapterPrefix = "vEthernet"
)

// UdevWatcher maintains the state of attached ENIs to the instance. It also has supporting elements to
// maintain consistency and update intervals
// Platform specific fields are included at the end
type UdevWatcher struct {
	ctx                  context.Context
	cancel               context.CancelFunc
	updateIntervalTicker *time.Ticker
	agentState           dockerstate.TaskEngineState
	eniChangeEvent       chan<- statechange.Event
	primaryMAC           string
	interfaceMonitor     iphelperwrapper.InterfaceMonitor
	notifications        chan int
	netutils             networkutils.WindowsNetworkUtils
}

// NewWindowsWatcher is used to return an instance of the UdevWatcher
// This method starts the interface monitor as well.
// If the interface monitor cannot be started then we return an error
func NewWindowsWatcher(ctx context.Context, primaryMAC string, state dockerstate.TaskEngineState,
	stateChangeEvents chan<- statechange.Event, infMonitor iphelperwrapper.InterfaceMonitor) (*UdevWatcher, error) {

	derivedContext, cancel := context.WithCancel(ctx)

	notificationChannel := make(chan int)
	err := infMonitor.StartMonitor(notificationChannel)
	if err != nil {
		return nil, errors.Wrapf(err, "Error occurred while instantiating watcher")
	}

	return &UdevWatcher{
		ctx:              derivedContext,
		cancel:           cancel,
		agentState:       state,
		eniChangeEvent:   stateChangeEvents,
		primaryMAC:       primaryMAC,
		interfaceMonitor: infMonitor,
		notifications:    notificationChannel,
		netutils:         networkutils.GetNetworkUtils(),
	}, nil
}

// This method is used to reconcile the state of the ENIs attached to the instance
// We get a map of all the interfaces and then reconcile the state of each interface
func (udevWatcher *UdevWatcher) reconcileOnce(withRetry bool) error {
	state, err := udevWatcher.getAllInterfaces()
	if err != nil {
		return err
	} else if state == nil || len(state) == 0 {
		return nil
	}

	for mac := range state {
		if withRetry {
			go func(ctx context.Context, macAddress string, timeout time.Duration) {
				if err := udevWatcher.sendENIStateChangeWithRetries(ctx, macAddress, timeout); err != nil {
					log.Infof("Udev watcher event-handler: unable to send state change: %v", err)
				}
			}(udevWatcher.ctx, mac, sendENIStateChangeRetryTimeout)
			continue
		}
		if err := udevWatcher.sendENIStateChange(mac); err != nil {
			// This is not an error condition as the eni status message has already been sent
			if strings.Contains(err.Error(), eniStatusSentMsg) {
				continue
			} else if _, ok := err.(*unmanagedENIError); ok {
				log.Debugf("Udev watcher reconciliation: unable to send state change: %v", err)
			} else {
				log.Warnf("Udev watcher reconciliation: unable to send state change: %v", err)
			}
		}
	}
	return nil
}

// This method is used to continuously monitor the notification channel for any notification
// If any new notification is received then we will retrieve the MAC address of that interface and send the ENI state change event
// If the context is cancelled then we will stop the monitor
func (udevWatcher *UdevWatcher) eventHandler() {
	for {
		select {
		case event := <-udevWatcher.notifications:
			if event == iphelperwrapper.InitialNotification {
				continue
			}
			go func(ctx context.Context, timeout time.Duration) {
				mac, err := udevWatcher.netutils.GetInterfaceMACByIndex(event, ctx, timeout)
				if err != nil {
					log.Debugf("udevWatcher : event-handler : Cannot retrieve MAC Address for the interface with index : %v", event)
					return
				}

				if err := udevWatcher.sendENIStateChangeWithRetries(ctx, mac, timeout); err != nil {
					log.Warnf("udevWatcher : event-handler : Unable to send ENI State Change : %v", err)
					return
				}

			}(udevWatcher.ctx, sendENIStateChangeRetryTimeout)
		case <-udevWatcher.ctx.Done():
			if err := udevWatcher.interfaceMonitor.Close(); err != nil {
				log.Errorf("udevWatcher : event-handler : Cannot close the Windows interface monitor : %v", err)
			}
			return
		}
	}
}

// This method is used to obtain a list of all available valid interfaces
func (udevWatcher *UdevWatcher) getAllInterfaces() (state map[string]int, err error) {
	interfaces, err := udevWatcher.netutils.GetAllNetworkInterfaces()
	if err != nil {
		return nil, errors.Wrap(err, "Error occured while retrieving details of available interfaces.")
	}

	if len(interfaces) == 0 {
		return nil, nil
	}

	state = make(map[string]int)

	for _, inf := range interfaces {
		mac := inf.HardwareAddr.String()
		//We are considering only those adapters which have non-empty non-primary MAC address.
		//Also, We are considering only those adapters which are not virtual and are not connected to any vSwitch
		//This is because only such interfaces would be new and need to be reconciled.
		if mac != "" && mac != udevWatcher.primaryMAC && !strings.HasPrefix(inf.Name, defaultVirtualAdapterPrefix) {
			state[mac] = inf.Index
		}
	}
	return state, nil
}

// This method is used for injecting NetworkUtils instance in udevWatcher
// This will be handy while testing to inject mock objects
func (udevWatcher *UdevWatcher) SetNetworkUtils(utils networkutils.WindowsNetworkUtils) {
	udevWatcher.netutils = utils
}
