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

package csiclient

const gibToBytes = 1024 * 1024 * 1024

// dummyCSIClient can be used to test the behaviour of csi client.
type dummyCSIClient struct {
}

func (c *dummyCSIClient) GetVolumeMetrics(volumeId string, hostMountPath string) (*Metrics, error) {
	return &Metrics{
		Used:     15 * gibToBytes,
		Capacity: 20 * gibToBytes,
	}, nil
}

func NewDummyCSIClient() CSIClient {
	return &dummyCSIClient{}
}
