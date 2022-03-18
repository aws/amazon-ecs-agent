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

package networkutils

import (
	"context"
	"net"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	"github.com/aws/amazon-ecs-agent/agent/eni/netwrapper"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// NetworkUtils is the interface used for accessing network related functionality on Windows.
// The methods declared in this package may or may not add any additional logic over the actual networking api calls.
type NetworkUtils interface {
	GetInterfaceMACByIndex(int, context.Context, time.Duration) (string, error)
	GetAllNetworkInterfaces() ([]net.Interface, error)
	GetDNSServerAddressList(macAddress string) ([]string, error)
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

var funcGetAdapterAddresses = getAdapterAddresses

// New creates a new network utils.
func New() NetworkUtils {
	return &networkUtils{
		netWrapper: netwrapper.New(),
	}
}

// GetInterfaceMACByIndex is used for obtaining the MAC address of an interface with a given interface index.
// We internally call net.InterfaceByIndex for this purpose.
func (utils *networkUtils) GetInterfaceMACByIndex(index int, ctx context.Context,
	timeout time.Duration) (mac string, err error) {

	utils.interfaceIndex = index
	utils.timeout = timeout
	utils.ctx = ctx

	return utils.retrieveMAC()
}

// retrieveMAC is used to retrieve MAC address using retry with backoff.
// We use retry logic in order to account for any delay in MAC Address becoming available after the interface addition notification is received.
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

// GetAllNetworkInterfaces returns all the network interfaces.
func (utils *networkUtils) GetAllNetworkInterfaces() ([]net.Interface, error) {
	return utils.netWrapper.GetAllNetworkInterfaces()
}

// SetNetWrapper is used to inject netWrapper instance. This will be handy while testing to inject mocks.
func (utils *networkUtils) SetNetWrapper(netWrapper netwrapper.NetWrapper) {
	utils.netWrapper = netWrapper
}

// GetDNSServerAddressList returns the DNS server addresses of the queried interface.
func (utils *networkUtils) GetDNSServerAddressList(macAddress string) ([]string, error) {
	addresses, err := funcGetAdapterAddresses()
	if err != nil {
		return nil, err
	}

	var firstDnsNode *windows.IpAdapterDnsServerAdapter
	// Find the adapter which has the same mac as queried.
	for _, adapterAddr := range addresses {
		if strings.EqualFold(utils.parseMACAddress(adapterAddr).String(), macAddress) {
			firstDnsNode = adapterAddr.FirstDnsServerAddress
			break
		}
	}

	dnsServerAddressList := make([]string, 0)
	for firstDnsNode != nil {
		dnsServerAddressList = append(dnsServerAddressList, utils.parseSocketAddress(firstDnsNode.Address))
		firstDnsNode = firstDnsNode.Next
	}

	return dnsServerAddressList, nil
}

// parseMACAddress parses the physical address of windows.IpAdapterAddresses into net.HardwareAddr.
func (utils *networkUtils) parseMACAddress(adapterAddress *windows.IpAdapterAddresses) net.HardwareAddr {
	hardwareAddr := make(net.HardwareAddr, adapterAddress.PhysicalAddressLength)
	if adapterAddress.PhysicalAddressLength > 0 {
		copy(hardwareAddr, adapterAddress.PhysicalAddress[:])
		return hardwareAddr
	}
	return hardwareAddr
}

// parseSocketAddress parses the SocketAddress into its string representation.
// This method needs to be deprecated in favour of IP() method of SocketAdress introduced in Go 1.13+.
// The method details have been taken from https://github.com/golang/sys/blob/release-branch.go1.13/windows/types_windows.go
func (utils *networkUtils) parseSocketAddress(addr windows.SocketAddress) string {
	var ipAddr string
	if uintptr(addr.SockaddrLength) >= unsafe.Sizeof(syscall.RawSockaddrInet4{}) && addr.Sockaddr.Addr.Family == syscall.AF_INET {
		ip := net.IP((*syscall.RawSockaddrInet4)(unsafe.Pointer(addr.Sockaddr)).Addr[:])
		ipAddr = ip.String()
	}
	return ipAddr
}

// getAdapterAddresses returns a list of IP adapter and address
// structures. The structure contains an IP adapter and flattened
// multiple IP addresses including unicast, anycast and multicast
// addresses.
// This method is copied as it is from the official golang source code.
// https://golang.org/src/net/interface_windows.go
func getAdapterAddresses() ([]*windows.IpAdapterAddresses, error) {
	var b []byte
	l := uint32(15000) // recommended initial size
	for {
		b = make([]byte, l)
		err := windows.GetAdaptersAddresses(syscall.AF_UNSPEC, windows.GAA_FLAG_INCLUDE_PREFIX, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&b[0])), &l)
		if err == nil {
			if l == 0 {
				return nil, nil
			}
			break
		}
		if err.(syscall.Errno) != syscall.ERROR_BUFFER_OVERFLOW {
			return nil, os.NewSyscallError("getadaptersaddresses", err)
		}
		if l <= uint32(len(b)) {
			return nil, os.NewSyscallError("getadaptersaddresses", err)
		}
	}
	var aas []*windows.IpAdapterAddresses
	for aa := (*windows.IpAdapterAddresses)(unsafe.Pointer(&b[0])); aa != nil; aa = aa.Next {
		aas = append(aas, aa)
	}
	return aas, nil
}
