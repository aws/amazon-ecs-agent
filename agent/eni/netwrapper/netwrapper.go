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
)

// NetWrapper interface is created to abstract Golang's net package from its usage.
// This enables us to mock this interface for unit tests.
type NetWrapper interface {
	FindInterfaceByIndex(int) (*net.Interface, error)
	GetAllNetworkInterfaces() ([]net.Interface, error)
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
	return utils.getAllNetworkInterfaces()
}
