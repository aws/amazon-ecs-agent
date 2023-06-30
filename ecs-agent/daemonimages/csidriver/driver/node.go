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
	"github.com/container-storage-interface/spec/lib/go/csi"
)

// nodeService represents the node service of CSI driver.
type nodeService struct {
	mounter Mounter
	// UnimplementedNodeServer implements all interfaces with empty implementation. As one mini version of csi driver,
	// we only need to override the necessary interfaces.
	csi.UnimplementedNodeServer
}

func newNodeService() nodeService {
	nodeMounter, err := newNodeMounter()
	if err != nil {
		panic(err)
	}

	return nodeService{mounter: nodeMounter}
}
