// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package ec2

import "errors"

type blackHoleMetadataClient struct{}

func NewBlackholeEC2MetadataClient() EC2MetadataClient {
	return blackHoleMetadataClient{}
}

func (blackHoleMetadataClient) InstanceIdentityDocument() (*InstanceIdentityDocument, error) {
	return nil, errors.New("blackholed")
}

func (blackHoleMetadataClient) PrimaryENIMAC() (string, error) {
	return "", errors.New("blackholed")
}

func (blackHoleMetadataClient) VPCID(mac string) (string, error) {
	return "", errors.New("blackholed")
}

func (blackHoleMetadataClient) SubnetID(mac string) (string, error) {
	return "", errors.New("blackholed")
}

func (blackholeMetadataClientImpl) GetMetadata(path string) (string, error) {
	return "", errors.New("blackholed")
}

func (blackholeMetadataClientImpl) GetDynamicData(path string) (string, error) {
	return "", errors.New("blackholed")
}
