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

//go:build windows
// +build windows

package ecscni

import (
	"runtime"

	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/uuid"

	"github.com/Microsoft/hcsshim/hcn"
	"github.com/pkg/errors"
)

// NetNSUtil provides some basic methods for performing network namespace related operations.
type NetNSUtil interface {
	// NewNetNS creates a new network namespace in the system.
	NewNetNS(netNSID string) error
	// DelNetNS deletes the network namespace from the system.
	DelNetNS(netNSID string) error
	// NewNetNSID generates a HCN Namespace ID.
	NewNetNSID() string
	// NSExists checks if the given namespace exists or not.
	NSExists(netNSID string) (bool, error)
}

type nsUtil struct{}

// NewNetNSUtil creates a new instance of NetNSUtil.
func NewNetNSUtil() NetNSUtil {
	return &nsUtil{}
}

// NewNetNS creates a new network namespace with the given namespace id.
func (*nsUtil) NewNetNS(netNSID string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hcnNetNs := &hcn.HostComputeNamespace{
		Id:            netNSID,
		SchemaVersion: hcn.V2SchemaVersion(),
	}

	_, err := hcnNetNs.Create()
	if err != nil {
		return errors.Wrapf(err, "unable to create hcn namespace")
	}

	return nil
}

// DelNetNS removes the network namespace with the given namespace id.
func (*nsUtil) DelNetNS(netNSID string) error {
	hcnNetNs, err := hcn.GetNamespaceByID(netNSID)
	if err != nil {
		// The possible reasons for HCN not able to find the specified network namespace can be-
		// 1. Duplicate deletion calls are invoked for the same namespace.
		// 2. DelNetNS is invoked before NewNetNS.
		// 3. Error from HCN while trying to find the namespace.
		return errors.Wrapf(err, "unable to find hcn namespace")
	}

	err = hcnNetNs.Delete()
	if err != nil {
		return errors.Wrapf(err, "unable to delete hcn namespace")
	}

	return nil
}

// NewNetNSID generates a HCN Namespace ID.
func (*nsUtil) NewNetNSID() string {
	return uuid.GenerateWithPrefix("", "")
}

// NSExists checks if any namespace exists with the given namespace id.
func (*nsUtil) NSExists(netNSID string) (bool, error) {
	_, err := hcn.GetNamespaceByID(netNSID)
	if err != nil {
		// If the HCN resource was not found then the namespace doesn't exist, so return false.
		// Otherwise, there was an error in HCN request, so return an error.
		if hcn.IsNotFoundError(err) {
			return false, nil
		} else {
			return false, errors.Wrap(err, "unable to get the status of ns")
		}
	}

	return true, nil
}
