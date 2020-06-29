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
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	"github.com/aws/amazon-ecs-agent/agent/eni/gonetwrapper"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
	"net"
	"time"
)

// Interface for NetworkUtils
// We will abstract the Golang's net package from its direct use in watcher.
type WindowsNetworkUtils interface {
	GetInterfaceMACByIndex(int, context.Context, time.Duration) (string, error)
	GetAllNetworkInterfaces() ([]net.Interface, error)
}

type networkUtils struct {
	// Interface Index as returned by "NotifyIPInterfaceChange" API
	interfaceIndex int
	// The retrieved macAddress is stored here
	macAddress string
	// This is the timeout after which we stop looking for MAC Address of ENI on host
	timeout    time.Duration
	ctx        context.Context
	// A wrapper over Golang's net package
	gonetutils gonetwrapper.GolangNetUtils
}

// Returns an instance of networkUtils which can be used to access this package's functionality
func GetNetworkUtils() *networkUtils {
	return &networkUtils{
		gonetutils: gonetwrapper.NewGoNetUtilWrapper(),
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

		inf, err := utils.gonetutils.FindInterfaceByIndex(utils.interfaceIndex)
		if err != nil {
			seelog.Warnf("Unable to retrieve MAC Address for Interface Index : %v , %v", utils.interfaceIndex, err)
			return apierrors.NewRetriableError(apierrors.NewRetriable(false), err)
		}

		if inf.HardwareAddr.String() == "" {
			seelog.Debugf("Empty MAC Address for interface with index : %v", utils.interfaceIndex)
			return errors.Errorf("Empty MAC Address for interface with index : %v", utils.interfaceIndex)
		}

		utils.macAddress = inf.HardwareAddr.String()
		return nil
	})

	if err != nil {
		return "", err
	}

	if err = ctx.Err(); err != nil {
		return "", errors.Wrapf(err, "Timed out waiting for MAC Address for interface with Index : %v", utils.interfaceIndex)
	}

	return utils.macAddress, nil
}

// Returns all the network interfaces
func (utils *networkUtils) GetAllNetworkInterfaces() ([]net.Interface, error){
	return utils.gonetutils.GetAllNetworkInterfaces()
}

// This method is used to inject GolangNetUtils instance. This will be handy while testing to inject mocks.
func (utils *networkUtils) SetGoNetUtils(gutils gonetwrapper.GolangNetUtils){
	utils.gonetutils = gutils
}
