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

	"github.com/aws/amazon-ecs-agent/agent/eni/netwrapper"
	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

//go:generate mockgen -destination=mocks/$GOFILE -copyright_file=../../../scripts/copyright_file github.com/aws/amazon-ecs-agent/agent/eni/networkutils NetworkUtils

// NetworkUtils is the interface used for accessing network related functionality on Windows.
// The methods declared in this package may or may not add any additional logic over the actual networking api calls.
type NetworkUtils interface {
	// GetInterfaceMACByIndex returns the MAC address of the device with given interface index.
	// We will retry with the given timeout and context before erroring out.
	GetInterfaceMACByIndex(int, context.Context, time.Duration) (string, error)
	// GetAllNetworkInterfaces returns all the network interfaces in the host namespace.
	GetAllNetworkInterfaces() ([]net.Interface, error)
	// GetDNSServerAddressList returns the DNS Server list associated to the interface with
	// the given MAC address.
	GetDNSServerAddressList(macAddress string) ([]string, error)
	// ConvertInterfaceAliasToLUID converts an interface alias to it's LUID.
	ConvertInterfaceAliasToLUID(interfaceAlias string) (uint64, error)
	// GetMIBIfEntryFromLUID returns the MIB_IF_ROW2 for the interface with the given LUID.
	GetMIBIfEntryFromLUID(ifaceLUID uint64) (*MibIfRow2, error)
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
	// funcConvertInterfaceAliasToLuid is the system call to ConvertInterfaceAliasToLuid Win32 API.
	funcConvertInterfaceAliasToLuid func(a ...uintptr) (r1 uintptr, r2 uintptr, lastErr error)
	// funcGetIfEntry2Ex is the system call to GetIfEntry2Ex Win32 API.
	funcGetIfEntry2Ex func(a ...uintptr) (r1 uintptr, r2 uintptr, lastErr error)
}

var funcGetAdapterAddresses = getAdapterAddresses

// New creates a new network utils.
func New() NetworkUtils {
	// We would be using GetIfTable2Ex, GetIfEntry2Ex, and FreeMibTable Win32 APIs from IP Helper.
	moduleIPHelper := windows.NewLazySystemDLL("iphlpapi.dll")
	procConvertInterfaceAliasToLuid := moduleIPHelper.NewProc("ConvertInterfaceAliasToLuid")
	procGetIfEntry2Ex := moduleIPHelper.NewProc("GetIfEntry2Ex")

	return &networkUtils{
		netWrapper:                      netwrapper.New(),
		funcConvertInterfaceAliasToLuid: procConvertInterfaceAliasToLuid.Call,
		funcGetIfEntry2Ex:               procGetIfEntry2Ex.Call,
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
		dnsServerAddressList = append(dnsServerAddressList, firstDnsNode.Address.IP().String())
		firstDnsNode = firstDnsNode.Next
	}

	return dnsServerAddressList, nil
}

// ConvertInterfaceAliasToLUID returns the LUID of the interface with given interface alias.
// Internally, it would invoke ConvertInterfaceAliasToLuid Win32 API to perform the conversion.
func (utils *networkUtils) ConvertInterfaceAliasToLUID(interfaceAlias string) (uint64, error) {
	var luid uint64
	alias := windows.StringToUTF16Ptr(interfaceAlias)

	// ConvertInterfaceAliasToLuid function converts alias into LUID.
	// https://learn.microsoft.com/en-us/windows/win32/api/netioapi/nf-netioapi-convertinterfacealiastoluid
	retVal, _, _ := utils.funcConvertInterfaceAliasToLuid(uintptr(unsafe.Pointer(alias)), uintptr(unsafe.Pointer(&luid)))
	if retVal != 0 {
		return 0, errors.Errorf("error occured while calling ConvertInterfaceAliasToLuid: %s", syscall.Errno(retVal))
	}

	return luid, nil
}

// GetMIBIfEntryFromLUID returns the MIB_IF_ROW2 object for the interface with given LUID.
// Internally, this would invoke GetIfEntry2Ex Win32 API to retrieve the specific row.
func (utils *networkUtils) GetMIBIfEntryFromLUID(ifaceLUID uint64) (*MibIfRow2, error) {
	row := &MibIfRow2{
		InterfaceLUID: ifaceLUID,
	}

	// GetIfEntry2Ex function retrieves the MIB-II interface for the given interface index..
	// https://learn.microsoft.com/en-us/windows/win32/api/netioapi/nf-netioapi-getifentry2ex
	retVal, _, _ := utils.funcGetIfEntry2Ex(uintptr(0), uintptr(unsafe.Pointer(row)))
	if retVal != 0 {
		return nil, errors.Errorf("error occured while calling GetIfEntry2Ex: %s", syscall.Errno(retVal))
	}

	return row, nil
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
