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

package netlinkWrapper

import "github.com/vishvananda/netlink"

// NetLink Wrapper methods used from the vishvananda/netlink package
type NetLink interface {
	LinkByName(name string) (netlink.Link, error)
	LinkList() ([]netlink.Link, error)
}

type NetLinkClient struct {
}

// NewNetLink creates a new NetLink object
func NewNetLink() NetLink {
	return NetLinkClient{}
}

func (NetLinkClient) LinkByName(name string) (netlink.Link, error) {
	return netlink.LinkByName(name)
}

func (NetLinkClient) LinkList() ([]netlink.Link, error) {
	return netlink.LinkList()
}
