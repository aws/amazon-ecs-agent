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

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/ec2/http/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	testRoleName = "test-role"
	mac          = "01:23:45:67:89:ab"
	vpcID        = "vpc-1234"
	subnetID     = "subnet-1234"
	iidRegion    = "us-east-1"
)

var testInstanceIdentityDoc = `{
  "privateIp" : "172.1.1.1",
  "devpayProductCodes" : null,
  "availabilityZone" : "us-east-1a",
  "version" : "2010-08-31",
  "region" : "us-east-1",
  "accountId" : "012345678901",
  "instanceId" : "i-01234567",
  "billingProducts" : [ "bp-01234567" ],
  "imageId" : "ami-12345678",
  "instanceType" : "t2.micro",
  "kernelId" : null,
  "ramdiskId" : null,
  "pendingTime" : "2015-06-04T22:16:06Z",
  "architecture" : "x86_64"
}`

func testSuccessResponse(s string) (*http.Response, error) {
	return &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Proto:      "HTTP/1.0",
		Body:       ioutil.NopCloser(bytes.NewReader([]byte(s))),
	}, nil
}

func testErrorResponse() (*http.Response, error) {
	return &http.Response{
		Status:     "500 Broken",
		StatusCode: 500,
		Proto:      "HTTP/1.0",
	}, nil
}

func TestGetInstanceIdentityDoc(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_http.NewMockClient(ctrl)
	testClient := NewEC2MetadataClient(mockGetter)

	mockGetter.EXPECT().Get(EC2MetadataServiceURL + InstanceIdentityDocumentResource).Return(testSuccessResponse(testInstanceIdentityDoc))

	doc, err := testClient.InstanceIdentityDocument()
	assert.NoError(t, err)
	assert.Equal(t, iidRegion, doc.Region)
}

func TestRetriesOnError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_http.NewMockClient(ctrl)
	testClient := NewEC2MetadataClient(mockGetter)

	gomock.InOrder(
		mockGetter.EXPECT().Get(EC2MetadataServiceURL+InstanceIdentityDocumentResource).Return(nil, errors.New("Something broke")),
		mockGetter.EXPECT().Get(EC2MetadataServiceURL+InstanceIdentityDocumentResource).Return(testErrorResponse()),
		mockGetter.EXPECT().Get(EC2MetadataServiceURL+InstanceIdentityDocumentResource).Return(testSuccessResponse(testInstanceIdentityDoc)),
	)

	doc, err := testClient.InstanceIdentityDocument()
	assert.NoError(t, err)
	assert.Equal(t, iidRegion, doc.Region)
}

func TestErrorPropogatesUp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_http.NewMockClient(ctrl)
	testClient := NewEC2MetadataClient(mockGetter)

	mockGetter.EXPECT().Get(EC2MetadataServiceURL+InstanceIdentityDocumentResource).Return(nil, errors.New("Something broke")).MinTimes(1)

	_, err := testClient.InstanceIdentityDocument()
	assert.Error(t, err)
}

func TestReadResourceStringReadResourceError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_http.NewMockClient(ctrl)
	testClient := &ec2MetadataClient{httpClient: mockGetter}

	path := "/some/path"
	resourceName := "some resource"

	mockGetter.EXPECT().Get(EC2MetadataServiceURL+path).Return(
		nil, errors.New("Something broke")).MinTimes(1)

	_, err := testClient.readResourceString(path, resourceName)
	assert.Error(t, err)
}

func TestReadResourceStringReadResourceSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_http.NewMockClient(ctrl)
	testClient := &ec2MetadataClient{httpClient: mockGetter}

	path := "/some/path"
	resourceName := "some resource"
	response := "some response"
	mockGetter.EXPECT().Get(EC2MetadataServiceURL + path).Return(testSuccessResponse(response))

	clientResponse, err := testClient.readResourceString(path, resourceName)
	assert.NoError(t, err)
	assert.Equal(t, response, clientResponse)
}

func TestPrimaryMAC(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_http.NewMockClient(ctrl)
	testClient := NewEC2MetadataClient(mockGetter)

	mockGetter.EXPECT().Get(EC2MetadataServiceURL + macResource).Return(testSuccessResponse(mac))
	macResponse, err := testClient.PrimaryENIMAC()
	assert.NoError(t, err)
	assert.Equal(t, mac, macResponse)
}

func TestVPCID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_http.NewMockClient(ctrl)
	testClient := NewEC2MetadataClient(mockGetter)

	mockGetter.EXPECT().Get(EC2MetadataServiceURL +
		fmt.Sprintf(vpcIDResourceFormat, mac)).Return(testSuccessResponse(vpcID))
	vpcIDResponse, err := testClient.VPCID(mac)
	assert.NoError(t, err)
	assert.Equal(t, vpcID, vpcIDResponse)
}

func TestSubnetID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_http.NewMockClient(ctrl)
	testClient := NewEC2MetadataClient(mockGetter)

	mockGetter.EXPECT().Get(EC2MetadataServiceURL +
		fmt.Sprintf(subnetIDResourceFormat, mac)).Return(testSuccessResponse(subnetID))
	subnetIDResponse, err := testClient.SubnetID(mac)
	assert.NoError(t, err)
	assert.Equal(t, subnetID, subnetIDResponse)
}
