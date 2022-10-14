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

import "golang.org/x/sys/windows"

const (
	ifMaxStringSize        = 256
	ifMaxPhysAddressLength = 32
)

// MibIfRow2 structure stores information about a particular interface.
// https://learn.microsoft.com/en-us/windows/win32/api/netioapi/ns-netioapi-mib_if_row2
type MibIfRow2 struct {
	InterfaceLUID               uint64
	interfaceIndex              uint32
	interfaceGUID               windows.GUID
	alias                       [ifMaxStringSize + 1]uint16
	description                 [ifMaxStringSize + 1]uint16
	physicalAddressLength       uint32
	physicalAddress             [ifMaxPhysAddressLength]byte
	permanentPhysicalAddress    [ifMaxPhysAddressLength]byte
	mtu                         uint32
	ifType                      uint32
	tunnelType                  uint32
	mediaType                   uint32
	physicalMediumType          uint32
	accessType                  uint32
	directionType               uint32
	interfaceAndOperStatusFlags uint8
	operStatus                  uint32
	adminStatus                 uint32
	mediaConnectState           uint32
	networkGUID                 windows.GUID
	connectionType              uint32
	transmitLinkSpeed           uint64
	receiveLinkSpeed            uint64
	InOctets                    uint64
	InUcastPkts                 uint64
	InNUcastPkts                uint64
	InDiscards                  uint64
	InErrors                    uint64
	inUnknownProtos             uint64
	inUcastOctets               uint64
	inMulticastOctets           uint64
	inBroadcastOctets           uint64
	OutOctets                   uint64
	OutUcastPkts                uint64
	OutNUcastPkts               uint64
	OutDiscards                 uint64
	OutErrors                   uint64
	outUcastOctets              uint64
	outMulticastOctets          uint64
	outBroadcastOctets          uint64
	outQLen                     uint64
}
