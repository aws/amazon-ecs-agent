//go:build !linux && !windows
// +build !linux,!windows

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

package netconfig

import "errors"

type NetworkConfigClient struct {
}

func NewNetworkConfigClient() *NetworkConfigClient {
	return &NetworkConfigClient{}
}

// DefaultNetInterfaceName returns the device name of the first default network interface
// available on the instance. This is only supported on linux as of now.
func DefaultNetInterfaceName(unknown interface{}) (string, error) {
	return "", errors.New("not supported on unknown platform")
}
