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

// Mounter defines an interface for many volume related options. As of now, only
// 'PathExists' is added to determine if a file path exists on the node.
type Mounter interface {
	PathExists(path string) (bool, error)
}

// NodeMounter implements Mounter.
type NodeMounter struct {
	// TODO
}

func (nm NodeMounter) PathExists(path string) (bool, error) {
	// TODO
	return false, nil
}

func newNodeMounter() (Mounter, error) {
	// TODO
	return NodeMounter{}, nil
}
