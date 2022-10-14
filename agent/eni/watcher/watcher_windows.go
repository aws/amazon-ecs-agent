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

// ENIWatcher maintains the state of attached ENIs to the instance. It also has supporting elements to
// maintain consistency and update intervals
// Platform specific fields are included at the end
type ENIWatcher struct {
	ctx                  context.Context
	cancel               context.CancelFunc
	updateIntervalTicker *time.Ticker
	agentState           dockerstate.TaskEngineState
	eniChangeEvent       chan<- statechange.Event
	primaryMAC           string
	interfaceMonitor     iphelperwrapper.InterfaceMonitor
	notifications        chan int
	netutils             networkutils.NetworkUtils
}

// NewWindowsWatcher is used to return an instance of the ENIWatcher
// This method starts the interface monitor as well.
// If the interface monitor cannot be started then we return an error
// newWatcher is used to nest the return of the ENIWatcher struct
func newWatcher(ctx context.Context,
	primaryMAC string,
	state dockerstate.TaskEngineState,
	stateChangeEvents chan<- statechange.Event) (*ENIWatcher, error) {

	eniMonitor := iphelperwrapper.NewMonitor()
	notificationChannel := make(chan int)
	err := eniMonitor.Start(notificationChannel)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to start eni watcher")
	}
	log.Info("windows eni watcher has been initialized")

	derivedContext, cancel := context.WithCancel(ctx)
	return &ENIWatcher{
		ctx:              derivedContext,
		cancel:           cancel,
		agentState:       state,
		eniChangeEvent:   stateChangeEvents,
		primaryMAC:       primaryMAC,
		interfaceMonitor: eniMonitor,
		notifications:    notificationChannel,
		netutils:         networkutils.New(),
	}, nil
}

// reconcileOnce is used to reconcile the state of the ENIs attached to the instance
// We get a map of all the interfaces and then reconcile the state of each interface
func (eniWatcher *ENIWatcher) reconcileOnce(withRetry bool) error {
	state, err := eniWatcher.getAllInterfaces()
	if err != nil {
		return err
	} else if state == nil || len(state) == 0 {
		return nil
	}

	for mac := range state {
		if withRetry {
			go func(ctx context.Context, macAddress string, timeout time.Duration) {
				if err := eniWatcher.sendENIStateChangeWithRetries(ctx, macAddress, timeout); err != nil {
					log.Infof("windows eni watcher event-handler: unable to send state change: %v", err)
				}
			}(eniWatcher.ctx, mac, sendENIStateChangeRetryTimeout)
			continue
		}
		if err := eniWatcher.sendENIStateChange(mac); err != nil {
			// This is not an error condition as the eni status message has already been sent
			if strings.Contains(err.Error(), eniStatusSentMsg) {
				continue
			} else if _, ok := err.(*unmanagedENIError); ok {
				log.Debugf("windows eni watcher reconciliation: unable to send state change %v", err)
			} else {
				log.Warnf("windows eni watcher reconciliation: unable to send state change %v", err)
			}
		}
	}
	return nil
}

// eventHandler is used to continuously monitor the notification channel for any notification
// If any new notification is received then we will retrieve the MAC address of that interface and send the ENI state change event
// If the context is cancelled then we will stop the monitor
func (eniWatcher *ENIWatcher) eventHandler() {
	for {
		select {
		case event := <-eniWatcher.notifications:
			if event == iphelperwrapper.InitialNotification {
				continue
			}
			// If any notification is received then we handle it in a separate goroutine
			// as handling the notification in same routine can lead to blocking of subsequent notifications
			go func(ctx context.Context, timeout time.Duration) {
				mac, err := eniWatcher.netutils.GetInterfaceMACByIndex(event, ctx, timeout)
				if err != nil {
					log.Debugf("windows eni watcher event-handler: cannot retrieve mac address for the interface with index %v", event)
					return
				}

				if err := eniWatcher.sendENIStateChangeWithRetries(ctx, mac, timeout); err != nil {
					log.Warnf("windows eni watcher event-handler: unable to send eni state change %v", err)
					return
				}

			}(eniWatcher.ctx, sendENIStateChangeRetryTimeout)
		case <-eniWatcher.ctx.Done():
			if err := eniWatcher.interfaceMonitor.Close(); err != nil {
				log.Errorf("windows eni watcher event-handler: cannot close the windows interface monitor %v", err)
			}
			return
		}
	}
}

// getAllInterfaces is used to obtain a list of all available valid interfaces
func (eniWatcher *ENIWatcher) getAllInterfaces() (state map[string]int, err error) {
	interfaces, err := eniWatcher.netutils.GetAllNetworkInterfaces()
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving available interfaces")
	}

	if len(interfaces) == 0 {
		return nil, nil
	}

	state = make(map[string]int)

	for _, iface := range interfaces {
		mac := iface.HardwareAddr.String()
		// We are considering only those adapters which have non-empty non-primary MAC address.
		// Also, We are considering only those adapters which are not virtual and are not connected to any vSwitch
		// This is because only such interfaces would be new and need to be reconciled.
		if mac != "" && mac != eniWatcher.primaryMAC && !strings.HasPrefix(iface.Name, defaultVirtualAdapterPrefix) {
			state[mac] = iface.Index
		}
	}
	return state, nil
}
