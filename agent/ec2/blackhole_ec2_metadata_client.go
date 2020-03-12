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

package ec2

import (
	"errors"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"
)

type blackholeMetadataClient struct{}

func NewBlackholeEC2MetadataClient() EC2MetadataClient {
	return blackholeMetadataClient{}
}

func (blackholeMetadataClient) DefaultCredentials() (*RoleCredentials, error) {
	return nil, errors.New("blackholed")
}

func (blackholeMetadataClient) InstanceIdentityDocument() (ec2metadata.EC2InstanceIdentityDocument, error) {
	return ec2metadata.EC2InstanceIdentityDocument{}, errors.New("blackholed")
}

func (blackholeMetadataClient) PrimaryENIMAC() (string, error) {
	return "", errors.New("blackholed")
}

func (blackholeMetadataClient) AllENIMacs() (string, error) {
	return "", errors.New("blackholed")
}

func (blackholeMetadataClient) VPCID(mac string) (string, error) {
	return "", errors.New("blackholed")
}

func (blackholeMetadataClient) SubnetID(mac string) (string, error) {
	return "", errors.New("blackholed")
}

func (blackholeMetadataClient) InstanceID() (string, error) {
	return "", errors.New("blackholed")
}

func (blackholeMetadataClient) GetMetadata(path string) (string, error) {
	return "", errors.New("blackholed")
}

func (blackholeMetadataClient) GetDynamicData(path string) (string, error) {
	return "", errors.New("blackholed")
}

func (blackholeMetadataClient) GetUserData() (string, error) {
	return "", errors.New("blackholed")
}

func (blackholeMetadataClient) Region() (string, error) {
	return "", errors.New("blackholed")
}

func (blackholeMetadataClient) PrivateIPv4Address() (string, error) {
	return "", errors.New("blackholed")
}

func (blackholeMetadataClient) PublicIPv4Address() (string, error) {
	return "", errors.New("blackholed")
}

func (blackholeMetadataClient) SpotInstanceAction() (string, error) {
	return "", errors.New("blackholed")
}

func (blackholeMetadataClient) OutpostARN() (string, error) {
	return "", errors.New("blackholed")
}
