// +build !linux

// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

// Package netlinkWrapper is a culmination of interfaces, structs and methods
// that are placeholders for build purposes on unsupported platforms

package netlinkwrapper

import (
	"fmt"
	"runtime"
)

// LinkUnsupported is a placeholder for build purposes on unsupported platforms
type LinkUnsupported struct{}

// NetLinkClient is a placeholder for build purposes on unsupported platforms
type NetLinkClient struct{}

// NetLink interface for build purposes on unsupported platforms
type NetLink interface {
	LinkByName(name string) (LinkUnsupported, error)
	LinkList() ([]LinkUnsupported, error)
}

// New creates a new NetLink object
func New() NetLink {
	return NetLinkClient{}
}

// LinkByName is a placeholder for build purposes on unsupported platforms
func (NetLinkClient) LinkByName(name string) (LinkUnsupported, error) {
	return LinkUnsupported{}, fmt.Errorf("netlink wrapper: unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
}

// LinkList is a placeholder for build purposes on unsupported platforms
func (NetLinkClient) LinkList() ([]LinkUnsupported, error) {
	return []LinkUnsupported{}, fmt.Errorf("netlink wrapper: unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
}
