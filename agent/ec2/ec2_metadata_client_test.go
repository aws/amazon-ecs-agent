// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package ec2_test

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/ec2/mocks"
	"github.com/golang/mock/gomock"
)

func ignoreError(v interface{}, _ error) interface{} {
	return v
}

const (
	testRoleName = "test-role"
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

	mockGetter := mock_ec2.NewMockHTTPClient(ctrl)
	testClient := ec2.NewEC2MetadataClient(mockGetter)

	mockGetter.EXPECT().Get(ec2.EC2MetadataServiceURL + ec2.InstanceIdentityDocumentResource).Return(testSuccessResponse(testInstanceIdentityDoc))

	doc, err := testClient.InstanceIdentityDocument()
	if err != nil {
		t.Fatal("Expected to be able to get doc")
	}
	if doc.Region != "us-east-1" {
		t.Error("Wrong region; expected us-east-1 but got " + doc.Region)
	}
}

func TestRetriesOnError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHTTPClient(ctrl)
	testClient := ec2.NewEC2MetadataClient(mockGetter)

	gomock.InOrder(
		mockGetter.EXPECT().Get(ec2.EC2MetadataServiceURL+ec2.InstanceIdentityDocumentResource).Return(nil, errors.New("Something broke")),
		mockGetter.EXPECT().Get(ec2.EC2MetadataServiceURL+ec2.InstanceIdentityDocumentResource).Return(testErrorResponse()),
		mockGetter.EXPECT().Get(ec2.EC2MetadataServiceURL+ec2.InstanceIdentityDocumentResource).Return(testSuccessResponse(testInstanceIdentityDoc)),
	)

	doc, err := testClient.InstanceIdentityDocument()
	if err != nil {
		t.Fatal("Expected to be able to get doc")
	}
	if doc.Region != "us-east-1" {
		t.Error("Wrong region; expected us-east-1 but got " + doc.Region)
	}
}

func TestErrorPropogatesUp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHTTPClient(ctrl)
	testClient := ec2.NewEC2MetadataClient(mockGetter)

	mockGetter.EXPECT().Get(ec2.EC2MetadataServiceURL+ec2.InstanceIdentityDocumentResource).Return(nil, errors.New("Something broke")).AnyTimes()

	_, err := testClient.InstanceIdentityDocument()
	if err == nil {
		t.Fatal("Expected error to result")
	}
}
