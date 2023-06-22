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

package driver

import (
	mountutils "k8s.io/mount-utils"

	"github.com/aws/amazon-ecs-agent/ecs-agent/daemon_images/csi-driver/mounter"
)

// Mounter is the interface implemented by NodeMounter.
type Mounter interface {
	PathExists(path string) (bool, error)
}

// NodeMounter implements Mounter.
type NodeMounter struct {
	*mountutils.SafeFormatAndMount
}

func newNodeMounter() (Mounter, error) {
	// mounter.NewSafeMounter returns a SafeFormatAndMount
	safeMounter, err := mounter.NewSafeMounter()
	if err != nil {
		return nil, err
	}
	return &NodeMounter{safeMounter}, nil
}
