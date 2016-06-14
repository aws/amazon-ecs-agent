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
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/ec2/mocks"
	"github.com/golang/mock/gomock"
)

func makeTestRoleCredentials() ec2.RoleCredentials {
	return ec2.RoleCredentials{
		Code:            "Success",
		LastUpdated:     time.Now(),
		Type:            "AWS-HMAC",
		AccessKeyId:     "ACCESSKEY",
		SecretAccessKey: "SECREKEY",
		Token:           "TOKEN",
		Expiration:      time.Now().Add(time.Duration(2 * time.Hour)),
	}
}

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

func TestDefaultCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient := ec2.NewEC2MetadataClient(mockGetter)

	mockGetter.EXPECT().Get(ec2.EC2_METADATA_SERVICE_URL + ec2.SECURITY_CREDENTIALS_RESOURCE).Return(testSuccessResponse(testRoleName))
	mockGetter.EXPECT().Get(ec2.EC2_METADATA_SERVICE_URL + ec2.SECURITY_CREDENTIALS_RESOURCE + testRoleName).Return(testSuccessResponse(string(ignoreError(json.Marshal(makeTestRoleCredentials())).([]byte))))

	credentials, err := testClient.DefaultCredentials()
	if err != nil {
		t.Fail()
	}
	testCredentials := makeTestRoleCredentials()
	if credentials.AccessKeyId != testCredentials.AccessKeyId {
		t.Fail()
	}
}

func TestGetInstanceIdentityDoc(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient := ec2.NewEC2MetadataClient(mockGetter)

	mockGetter.EXPECT().Get(ec2.EC2_METADATA_SERVICE_URL + ec2.INSTANCE_IDENTITY_DOCUMENT_RESOURCE).Return(testSuccessResponse(testInstanceIdentityDoc))

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

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient := ec2.NewEC2MetadataClient(mockGetter)

	gomock.InOrder(
		mockGetter.EXPECT().Get(ec2.EC2_METADATA_SERVICE_URL+ec2.INSTANCE_IDENTITY_DOCUMENT_RESOURCE).Return(nil, errors.New("Something broke")),
		mockGetter.EXPECT().Get(ec2.EC2_METADATA_SERVICE_URL+ec2.INSTANCE_IDENTITY_DOCUMENT_RESOURCE).Return(testErrorResponse()),
		mockGetter.EXPECT().Get(ec2.EC2_METADATA_SERVICE_URL+ec2.INSTANCE_IDENTITY_DOCUMENT_RESOURCE).Return(testSuccessResponse(testInstanceIdentityDoc)),
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

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient := ec2.NewEC2MetadataClient(mockGetter)

	mockGetter.EXPECT().Get(ec2.EC2_METADATA_SERVICE_URL+ec2.INSTANCE_IDENTITY_DOCUMENT_RESOURCE).Return(nil, errors.New("Something broke")).AnyTimes()

	_, err := testClient.InstanceIdentityDocument()
	if err == nil {
		t.Fatal("Expected error to result")
	}
}
