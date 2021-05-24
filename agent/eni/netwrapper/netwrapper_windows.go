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

package netwrapper

import (
	"net"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NetWrapper interface is created to abstract Golang's net package from its usage in the watcher
// Also, this enables us to mock this interface for unit tests
type NetWrapper interface {
	FindInterfaceByIndex(int) (*net.Interface, error)
	GetAllNetworkInterfaces() ([]net.Interface, error)
	GetAdapterAddresses() ([]*windows.IpAdapterAddresses, error)
}

type utils struct{}

// New returns a wrapper over Golang's net package
func New() NetWrapper {
	return &utils{}
}

func (utils *utils) FindInterfaceByIndex(index int) (*net.Interface, error) {
	return net.InterfaceByIndex(index)
}

func (utils *utils) GetAllNetworkInterfaces() ([]net.Interface, error) {
	return net.Interfaces()
}

// GetAdapterAddresses returns a list of IP adapter and address
// structures. The structure contains an IP adapter and flattened
// multiple IP addresses including unicast, anycast and multicast
// addresses.
// This method is copied as it is from the official golang source code.
// https://golang.org/src/net/interface_windows.go
func (utils *utils) GetAdapterAddresses() ([]*windows.IpAdapterAddresses, error) {
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
