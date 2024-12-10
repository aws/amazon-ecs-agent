//go:build unit
// +build unit

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

package ec2_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	mock_ec2 "github.com/aws/amazon-ecs-agent/ecs-agent/ec2/mocks"
)

const (
	testRoleName = "test-role"
	mac          = "01:23:45:67:89:ab"
	macs         = "01:23:45:67:89:ab/\n01:23:45:67:89:ac"
	vpcID        = "vpc-1234"
	subnetID     = "subnet-1234"
	iidRegion    = "us-east-1"
	zoneID       = "use1-az1"
	privateIP    = "127.0.0.1"
	publicIP     = "127.0.0.1"
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

var testInstanceIdentityDoc = imds.InstanceIdentityDocument{
	PrivateIP:        "172.1.1.1",
	AvailabilityZone: "us-east-1a",
	Version:          "2010-08-31",
	Region:           "us-east-1",
	AccountID:        "012345678901",
	InstanceID:       "i-01234567",
	BillingProducts:  []string{"bp-01234567"},
	ImageID:          "ami-12345678",
	InstanceType:     "t2.micro",
	PendingTime:      time.Now(),
	Architecture:     "x86_64",
}

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
	testClient, err := ec2.NewEC2MetadataClient(mockGetter)
	assert.NoError(t, err)

	mockGetter.EXPECT().GetMetadata(gomock.Any(), &imds.GetMetadataInput{
		Path: ec2.SecurityCredentialsResource,
	}, gomock.Any()).Return(&imds.GetMetadataOutput{
		Content: nopReadCloser(testRoleName),
	}, nil)
	mockGetter.EXPECT().GetMetadata(gomock.Any(), &imds.GetMetadataInput{
		Path: ec2.SecurityCredentialsResource + testRoleName,
	}, gomock.Any()).Return(&imds.GetMetadataOutput{
		Content: nopReadCloser(string(ignoreError(json.Marshal(makeTestRoleCredentials())).([]byte))),
	}, nil)

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
	testClient, err := ec2.NewEC2MetadataClient(mockGetter)
	assert.NoError(t, err)

	mockGetter.EXPECT().GetInstanceIdentityDocument(
		gomock.Any(), &imds.GetInstanceIdentityDocumentInput{}, gomock.Any(),
	).Return(&imds.GetInstanceIdentityDocumentOutput{
		InstanceIdentityDocument: testInstanceIdentityDoc,
	}, nil)

	doc, err := testClient.InstanceIdentityDocument()
	assert.NoError(t, err)
	assert.Equal(t, iidRegion, doc.Region)
}

func TestErrorPropogatesUp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient, err := ec2.NewEC2MetadataClient(mockGetter)
	assert.NoError(t, err)

	mockGetter.EXPECT().GetInstanceIdentityDocument(
		gomock.Any(), &imds.GetInstanceIdentityDocumentInput{}, gomock.Any(),
	).Return(&imds.GetInstanceIdentityDocumentOutput{
		InstanceIdentityDocument: imds.InstanceIdentityDocument{},
	}, errors.New("Something broke"))

	_, err = testClient.InstanceIdentityDocument()
	assert.Error(t, err)
}

func TestPrimaryMAC(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient, err := ec2.NewEC2MetadataClient(mockGetter)
	assert.NoError(t, err)

	mockGetter.EXPECT().GetMetadata(gomock.Any(), &imds.GetMetadataInput{
		Path: ec2.MacResource,
	}).Return(&imds.GetMetadataOutput{
		Content: nopReadCloser(mac),
	}, nil)

	macResponse, err := testClient.PrimaryENIMAC()
	assert.NoError(t, err)
	assert.Equal(t, mac, macResponse)
}

func TestAllENIMacs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient, err := ec2.NewEC2MetadataClient(mockGetter)
	assert.NoError(t, err)

	mockGetter.EXPECT().GetMetadata(gomock.Any(), &imds.GetMetadataInput{
		Path: ec2.AllMacResource,
	}).Return(&imds.GetMetadataOutput{
		Content: nopReadCloser(macs),
	}, nil)

	macsResponse, err := testClient.AllENIMacs()
	assert.NoError(t, err)
	assert.Equal(t, macs, macsResponse)
}

func TestVPCID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient, err := ec2.NewEC2MetadataClient(mockGetter)
	assert.NoError(t, err)

	mockGetter.EXPECT().GetMetadata(gomock.Any(), &imds.GetMetadataInput{
		Path: fmt.Sprintf(ec2.VPCIDResourceFormat, mac),
	}).Return(&imds.GetMetadataOutput{
		Content: nopReadCloser(vpcID),
	}, nil)

	vpcIDResponse, err := testClient.VPCID(mac)
	assert.NoError(t, err)
	assert.Equal(t, vpcID, vpcIDResponse)
}

func TestSubnetID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient, err := ec2.NewEC2MetadataClient(mockGetter)
	assert.NoError(t, err)

	mockGetter.EXPECT().GetMetadata(gomock.Any(), &imds.GetMetadataInput{
		Path: fmt.Sprintf(ec2.SubnetIDResourceFormat, mac),
	}).Return(&imds.GetMetadataOutput{
		Content: nopReadCloser(subnetID),
	}, nil)

	subnetIDResponse, err := testClient.SubnetID(mac)
	assert.NoError(t, err)
	assert.Equal(t, subnetID, subnetIDResponse)
}

func TestAvailabilityZoneID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient, err := ec2.NewEC2MetadataClient(mockGetter)
	assert.NoError(t, err)

	mockGetter.EXPECT().GetMetadata(gomock.Any(), &imds.GetMetadataInput{
		Path: ec2.AvailabilityZoneID,
	}).Return(&imds.GetMetadataOutput{
		Content: nopReadCloser(zoneID),
	}, nil)

	zoneIDResponse, err := testClient.AvailabilityZoneID()
	assert.NoError(t, err)
	assert.Equal(t, zoneID, zoneIDResponse)
}

func TestPrivateIPv4Address(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient, err := ec2.NewEC2MetadataClient(mockGetter)
	assert.NoError(t, err)

	mockGetter.EXPECT().GetMetadata(gomock.Any(), &imds.GetMetadataInput{
		Path: ec2.PrivateIPv4Resource,
	}).Return(&imds.GetMetadataOutput{
		Content: nopReadCloser(privateIP),
	}, nil)

	privateIPResponse, err := testClient.PrivateIPv4Address()
	assert.NoError(t, err)
	assert.Equal(t, privateIP, privateIPResponse)
}

func TestPublicIPv4Address(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient, err := ec2.NewEC2MetadataClient(mockGetter)
	assert.NoError(t, err)

	mockGetter.EXPECT().GetMetadata(gomock.Any(), &imds.GetMetadataInput{
		Path: ec2.PublicIPv4Resource,
	}).Return(&imds.GetMetadataOutput{
		Content: nopReadCloser(publicIP),
	}, nil)

	publicIPResponse, err := testClient.PublicIPv4Address()
	assert.NoError(t, err)
	assert.Equal(t, publicIP, publicIPResponse)
}

func TestSpotInstanceAction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient, err := ec2.NewEC2MetadataClient(mockGetter)
	assert.NoError(t, err)

	mockGetter.EXPECT().GetMetadata(gomock.Any(), &imds.GetMetadataInput{
		Path: ec2.SpotInstanceActionResource,
	}).Return(&imds.GetMetadataOutput{
		Content: nopReadCloser("{\"action\": \"terminate\", \"time\": \"2017-09-18T08:22:00Z\"}"),
	}, nil)

	resp, err := testClient.SpotInstanceAction()
	assert.NoError(t, err)
	assert.Equal(t, "{\"action\": \"terminate\", \"time\": \"2017-09-18T08:22:00Z\"}", resp)
}

func TestSpotInstanceActionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient, err := ec2.NewEC2MetadataClient(mockGetter)
	assert.NoError(t, err)

	mockGetter.EXPECT().GetMetadata(gomock.Any(), &imds.GetMetadataInput{
		Path: ec2.SpotInstanceActionResource,
	}).Return(nil, fmt.Errorf("ERROR"))

	resp, err := testClient.SpotInstanceAction()
	assert.Error(t, err)
	assert.Equal(t, "", resp)
}

func nopReadCloser(s string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(s))
}
