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
package ec2util

import (
	"errors"

	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
)

// Fake EC2 metadata client that can be used for tests.
// All implemented methods return an appropriate happy response
// for the most typical Agent configuration.
type FakeEC2MetadataClient struct{}

func (FakeEC2MetadataClient) DefaultCredentials() (*ec2.RoleCredentials, error) {
	return nil, errors.New("not implemented")
}

func (FakeEC2MetadataClient) InstanceIdentityDocument() (imds.InstanceIdentityDocument, error) {
	return imds.InstanceIdentityDocument{}, errors.New("not implemented")
}

func (FakeEC2MetadataClient) PrimaryENIMAC() (string, error) {
	return "mac", nil
}

func (FakeEC2MetadataClient) AllENIMacs() (string, error) {
	return "", errors.New("not implemented")
}

func (FakeEC2MetadataClient) VPCID(mac string) (string, error) {
	return "", errors.New("not implemented")
}

func (FakeEC2MetadataClient) SubnetID(mac string) (string, error) {
	return "", errors.New("not implemented")
}

func (FakeEC2MetadataClient) InstanceID() (string, error) {
	return "", errors.New("not implemented")
}

func (FakeEC2MetadataClient) GetMetadata(path string) (string, error) {
	return "", errors.New("not implemented")
}

func (FakeEC2MetadataClient) GetDynamicData(path string) (string, error) {
	return "", errors.New("not implemented")
}

func (FakeEC2MetadataClient) GetUserData() (string, error) {
	return "", errors.New("not implemented")
}

func (FakeEC2MetadataClient) Region() (string, error) {
	return "", errors.New("not implemented")
}

func (FakeEC2MetadataClient) AvailabilityZoneID() (string, error) {
	return "", errors.New("not implemented")
}

func (FakeEC2MetadataClient) PrivateIPv4Address() (string, error) {
	return "", errors.New("not implemented")
}

func (FakeEC2MetadataClient) PublicIPv4Address() (string, error) {
	return "", errors.New("not implemented")
}

func (FakeEC2MetadataClient) IPv6Address() (string, error) {
	return "", errors.New("not implemented")
}

func (FakeEC2MetadataClient) SpotInstanceAction() (string, error) {
	return "", errors.New("not implemented")
}

func (FakeEC2MetadataClient) OutpostARN() (string, error) {
	return "", errors.New("not implemented")
}

func (FakeEC2MetadataClient) TargetLifecycleState() (string, error) {
	return "", errors.New("not implemented")
}
