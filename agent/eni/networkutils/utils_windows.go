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

package networkutils

import (
	"context"
	"net"
	"time"

	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	"github.com/aws/amazon-ecs-agent/agent/eni/netwrapper"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// NetworkUtils is the interface used for accessing network utils
// The methods declared in this package may or may not add any additional logic over the actual networking api calls.
// Moreover, we will use a wrapper over Golang's net package. This is done to ensure that any future change which
// requires a package different from Golang's net, can be easily implemented by changing the underlying wrapper without
// impacting watcher
type NetworkUtils interface {
	GetInterfaceMACByIndex(int, context.Context, time.Duration) (string, error)
	GetAllNetworkInterfaces() ([]net.Interface, error)
	SetNetWrapper(netWrapper netwrapper.NetWrapper)
}

type networkUtils struct {
	// Interface Index as returned by "NotifyIPInterfaceChange" API
	interfaceIndex int
	// The retrieved macAddress is stored here
	macAddress string
	// This is the timeout after which we stop looking for MAC Address of ENI on host
	timeout time.Duration
	ctx     context.Context
	// A wrapper over Golang's net package
	netWrapper netwrapper.NetWrapper
}

// New creates a new network utils
func New() NetworkUtils {
	return &networkUtils{
		netWrapper: netwrapper.New(),
	}
}

// This method is used for obtaining the MAC address of an interface with a given interface index
// We internally call net.InterfaceByIndex for this purpose
func (utils *networkUtils) GetInterfaceMACByIndex(index int, ctx context.Context,
	timeout time.Duration) (mac string, err error) {

	utils.interfaceIndex = index
	utils.timeout = timeout
	utils.ctx = ctx

	return utils.retrieveMAC()
}

// This method is used to retrieve MAC address using retry with backoff.
// We use retry logic in order to account for any delay in MAC Address becoming available after the interface addition notification is received
func (utils *networkUtils) retrieveMAC() (string, error) {
	backoff := retry.NewExponentialBackoff(macAddressBackoffMin, macAddressBackoffMax,
		macAddressBackoffJitter, macAddressBackoffMultiple)

	ctx, cancel := context.WithTimeout(utils.ctx, utils.timeout)
	defer cancel()

	err := retry.RetryWithBackoffCtx(ctx, backoff, func() error {

		iface, err := utils.netWrapper.FindInterfaceByIndex(utils.interfaceIndex)
		if err != nil {
			seelog.Warnf("Unable to retrieve mac address for Interface Index: %v , %v", utils.interfaceIndex, err)
			return apierrors.NewRetriableError(apierrors.NewRetriable(false), err)
		}

		if iface.HardwareAddr.String() == "" {
			seelog.Debugf("Empty MAC Address for interface with index: %v", utils.interfaceIndex)
			return errors.Errorf("empty mac address for interface with index: %v", utils.interfaceIndex)
		}

		utils.macAddress = iface.HardwareAddr.String()
		return nil
	})

	if err != nil {
		return "", err
	}

	if err = ctx.Err(); err != nil {
		return "", errors.Wrapf(err, "timed out waiting for mac address for interface with Index: %v", utils.interfaceIndex)
	}

	return utils.macAddress, nil
}

// Returns all the network interfaces
func (utils *networkUtils) GetAllNetworkInterfaces() ([]net.Interface, error) {
	return utils.netWrapper.GetAllNetworkInterfaces()
}

// This method is used to inject netWrapper instance. This will be handy while testing to inject mocks.
func (utils *networkUtils) SetNetWrapper(netWrapper netwrapper.NetWrapper) {
	utils.netWrapper = netWrapper
}
