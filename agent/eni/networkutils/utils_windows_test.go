// +build windows,unit

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
	"errors"
	"net"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"

	mock_netwrapper "github.com/aws/amazon-ecs-agent/agent/eni/netwrapper/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	interfaceIndex = 9
	macAddress     = "02:22:ea:8c:81:dc"
	validDnsServer = "10.0.0.2"
)

// This is a success test. We receive the appropriate MAC address corresponding to the interface index.
func TestGetInterfaceMACByIndex(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	mocknetwrapper := mock_netwrapper.NewMockNetWrapper(mockCtrl)
	netUtils := New()
	netUtils.SetNetWrapper(mocknetwrapper)
	hardwareAddr, err := net.ParseMAC(macAddress)

	mocknetwrapper.EXPECT().FindInterfaceByIndex(interfaceIndex).Return(
		&net.Interface{
			Index:        interfaceIndex,
			HardwareAddr: hardwareAddr,
		}, nil)

	mac, err := netUtils.GetInterfaceMACByIndex(interfaceIndex, ctx, macAddressBackoffMax)

	assert.Equal(t, macAddress, mac)
	assert.NoError(t, err)
}

// In this test case, an empty MAC Address is returned everytime.
// Therefore, we will return an error
func TestGetInterfaceMACByIndexEmptyAddress(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	mocknetwrapper := mock_netwrapper.NewMockNetWrapper(mockCtrl)
	netUtils := New()
	netUtils.SetNetWrapper(mocknetwrapper)

	mocknetwrapper.EXPECT().FindInterfaceByIndex(interfaceIndex).Return(
		&net.Interface{
			Index:        interfaceIndex,
			HardwareAddr: make([]byte, 0),
		}, nil).AnyTimes()

	mac, err := netUtils.GetInterfaceMACByIndex(interfaceIndex, ctx, macAddressBackoffMax)

	assert.Error(t, err)
	assert.Empty(t, mac)
}

// In this test case, we will return empty MAC address first 3 times and then the correct address.
// We should get the appropriate MAC Address without any errors
func TestGetInterfaceMACByIndexRetries(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	mocknetwrapper := mock_netwrapper.NewMockNetWrapper(mockCtrl)
	netUtils := New()
	netUtils.SetNetWrapper(mocknetwrapper)
	hardwareAddr, err := net.ParseMAC(macAddress)
	emptyaddr := make([]byte, 0)

	gomock.InOrder(
		mocknetwrapper.EXPECT().FindInterfaceByIndex(interfaceIndex).Return(
			&net.Interface{
				Index:        interfaceIndex,
				HardwareAddr: emptyaddr,
			}, nil).Times(3),
		mocknetwrapper.EXPECT().FindInterfaceByIndex(interfaceIndex).Return(
			&net.Interface{
				Index:        interfaceIndex,
				HardwareAddr: hardwareAddr,
			}, nil),
	)

	mac, err := netUtils.GetInterfaceMACByIndex(interfaceIndex, ctx, macAddressBackoffMax*3)

	assert.NoError(t, err)
	assert.Equal(t, macAddress, mac)
}

// In this test case, the context times out before any response is received.
// Therefore, we will return an error
func TestGetInterfaceMACByIndexContextTimeout(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	mocknetwrapper := mock_netwrapper.NewMockNetWrapper(mockCtrl)
	netUtils := New()
	netUtils.SetNetWrapper(mocknetwrapper)

	mocknetwrapper.EXPECT().FindInterfaceByIndex(interfaceIndex).Return(
		&net.Interface{
			Index:        interfaceIndex,
			HardwareAddr: make([]byte, 0),
		}, nil).MinTimes(1)

	mac, err := netUtils.GetInterfaceMACByIndex(interfaceIndex, ctx, macAddressBackoffMin*2)

	assert.Error(t, err)
	assert.Empty(t, mac)
}

// In this test case, we will simulate an error connecting with the host to get the interfaces.
// Therefore, the response should be an error
func TestGetInterfaceMACByIndexWithGolangNetError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.TODO()
	mocknetwrapper := mock_netwrapper.NewMockNetWrapper(mockCtrl)
	netUtils := New()
	netUtils.SetNetWrapper(mocknetwrapper)

	mocknetwrapper.EXPECT().FindInterfaceByIndex(interfaceIndex).Return(
		nil, errors.New("unable to retrieve interface"))

	mac, err := netUtils.GetInterfaceMACByIndex(interfaceIndex, ctx, macAddressBackoffMax)

	assert.Error(t, err)
	assert.Empty(t, mac)
}

// Test for GetALlNetworkInterfaces. In this test case, all the interfaces would be returned.
func TestGetAllNetworkInterfaces(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mocknetwrapper := mock_netwrapper.NewMockNetWrapper(mockCtrl)
	netUtils := New()
	netUtils.SetNetWrapper(mocknetwrapper)

	expectedIface := make([]net.Interface, 1)

	expectedIface[0] = net.Interface{
		Index:        interfaceIndex,
		HardwareAddr: make([]byte, 0),
	}

	mocknetwrapper.EXPECT().GetAllNetworkInterfaces().Return(
		expectedIface, nil,
	)

	iface, err := netUtils.GetAllNetworkInterfaces()

	assert.NoError(t, err)
	assert.Equal(t, expectedIface, iface)
}

// Test for GetALlNetworkInterfaces. In this test case, we will simulate an error connecting to the host.
// Therefore, we will return an error
func TestGetAllNetworkInterfacesError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mocknetwrapper := mock_netwrapper.NewMockNetWrapper(mockCtrl)
	netUtils := New()
	netUtils.SetNetWrapper(mocknetwrapper)

	mocknetwrapper.EXPECT().GetAllNetworkInterfaces().Return(
		nil, errors.New("error occurred while fetching interfaces"),
	)

	inf, err := netUtils.GetAllNetworkInterfaces()

	assert.Nil(t, inf)
	assert.Error(t, err)
}

// TestGetDNSServerAddressList tests the success path of GetDNSServerAddressList.
func TestGetDNSServerAddressList(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mocknetwrapper := mock_netwrapper.NewMockNetWrapper(mockCtrl)
	netUtils := networkUtils{netWrapper: mocknetwrapper}

	funcGetAdapterAddresses = func() ([]*windows.IpAdapterAddresses, error) {
		return []*windows.IpAdapterAddresses{
			{
				PhysicalAddressLength: 6,
				PhysicalAddress:       [8]byte{2, 34, 234, 140, 129, 220, 0, 0},
				FirstDnsServerAddress: &windows.IpAdapterDnsServerAdapter{
					Address: windows.SocketAddress{
						Sockaddr: &syscall.RawSockaddrAny{
							Addr: syscall.RawSockaddr{
								Family: syscall.AF_INET,
								Data:   [14]int8{0, 0, 10, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0},
							},
						},
						SockaddrLength: 16,
					},
				},
			},
		}, nil
	}

	dnsServerList, err := netUtils.GetDNSServerAddressList(macAddress)
	assert.NoError(t, err)
	assert.Len(t, dnsServerList, 1)
	assert.EqualValues(t, dnsServerList[0], validDnsServer)
}
