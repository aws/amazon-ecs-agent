//go:build unit

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
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func TestIPv6Address(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient, err := ec2.NewEC2MetadataClient(mockGetter)
	assert.NoError(t, err)

	ipv6 := "2001:db8::1"
	mockGetter.EXPECT().GetMetadata(gomock.Any(), &imds.GetMetadataInput{
		Path: ec2.IPv6Resource,
	}).Return(&imds.GetMetadataOutput{
		Content: nopReadCloser(ipv6),
	}, nil)

	ipv6Response, err := testClient.IPv6Address()
	assert.NoError(t, err)
	assert.Equal(t, ipv6, ipv6Response)
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

// TestMetadataCaching tests the caching behavior of retrieving Metadata from IMDS.
func TestMetadataCaching(t *testing.T) {
	testCases := []struct {
		name        string
		metadataKey string
		methodName  string
		methodParam string // Optional parameter for methods like VPCID(mac)
		expectedVal string
	}{
		{"PrimaryENIMAC", ec2.MacResource, "PrimaryENIMAC", "", mac},
		{"InstanceID", ec2.InstanceIDResource, "InstanceID", "", "i-1234567890abcdef0"},
		{"AvailabilityZoneID", ec2.AvailabilityZoneID, "AvailabilityZoneID", "", zoneID},
		{"Region", ec2.RegionResource, "Region", "", "us-west-2"},
		{"VPCID", fmt.Sprintf(ec2.VPCIDResourceFormat, mac), "VPCID", mac, vpcID},
		{"SubnetID", fmt.Sprintf(ec2.SubnetIDResourceFormat, mac), "SubnetID", mac, subnetID},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGetter := mock_ec2.NewMockHttpClient(ctrl)
			testClient, err := ec2.NewEC2MetadataClient(mockGetter)
			assert.NoError(t, err)

			// Setup mock to expect exactly one call
			mockGetter.EXPECT().GetMetadata(gomock.Any(), &imds.GetMetadataInput{
				Path: tc.metadataKey,
			}).Return(&imds.GetMetadataOutput{
				Content: nopReadCloser(tc.expectedVal),
			}, nil).Times(1)

			// Helper to call the appropriate method
			callMethod := func() (string, error) {
				switch tc.methodName {
				case "PrimaryENIMAC":
					return testClient.PrimaryENIMAC()
				case "InstanceID":
					return testClient.InstanceID()
				case "AvailabilityZoneID":
					return testClient.AvailabilityZoneID()
				case "Region":
					return testClient.Region()
				case "VPCID":
					return testClient.VPCID(tc.methodParam)
				case "SubnetID":
					return testClient.SubnetID(tc.methodParam)
				default:
					return "", fmt.Errorf("unknown method: %s", tc.methodName)
				}
			}

			// First call should hit the mock
			result1, err := callMethod()
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedVal, result1)

			// Second call should use cache (no additional mock expectation)
			result2, err := callMethod()
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedVal, result2)
		})
	}
}

// TestInstanceIdentityDocumentCaching verifies that InstanceIdentityDocument caches the result
// This uses a separate cache mechanism, so we keep this test separate
func TestInstanceIdentityDocumentCaching(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient, err := ec2.NewEC2MetadataClient(mockGetter)
	assert.NoError(t, err)

	// First call should hit the mock
	mockGetter.EXPECT().GetInstanceIdentityDocument(
		gomock.Any(), &imds.GetInstanceIdentityDocumentInput{}, gomock.Any(),
	).Return(&imds.GetInstanceIdentityDocumentOutput{
		InstanceIdentityDocument: testInstanceIdentityDoc,
	}, nil).Times(1)

	// First call
	doc1, err := testClient.InstanceIdentityDocument()
	assert.NoError(t, err)
	assert.Equal(t, iidRegion, doc1.Region)
	assert.Equal(t, testInstanceIdentityDoc.InstanceID, doc1.InstanceID)

	// Second call should use cache (no mock expectation needed)
	doc2, err := testClient.InstanceIdentityDocument()
	assert.NoError(t, err)
	assert.Equal(t, iidRegion, doc2.Region)
	assert.Equal(t, testInstanceIdentityDoc.InstanceID, doc2.InstanceID)
}

// TestGetUserDataCaching tests GetUserData caching (uses custom GetUserData API)
func TestGetUserDataCaching(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := mock_ec2.NewMockHttpClient(ctrl)
	testClient, err := ec2.NewEC2MetadataClient(mockGetter)
	assert.NoError(t, err)

	userData := "#!/bin/bash\necho 'Hello World'"

	mockGetter.EXPECT().GetUserData(gomock.Any(), &imds.GetUserDataInput{}, gomock.Any()).Return(
		&imds.GetUserDataOutput{Content: nopReadCloser(userData)}, nil,
	).Times(1)

	userDataResponse1, err := testClient.GetUserData()
	assert.NoError(t, err)
	assert.Equal(t, userData, userDataResponse1)

	userDataResponse2, err := testClient.GetUserData()
	assert.NoError(t, err)
	assert.Equal(t, userData, userDataResponse2)
}

// TestMetadataNoCaching tests that certain metadata functions do NOT cache results
// and make fresh calls to IMDS on each invocation.
func TestMetadataNoCaching(t *testing.T) {
	testCases := []struct {
		name        string
		metadataKey string
		methodName  string
		firstVal    string
		secondVal   string
	}{
		{
			name:        "AllENIMacs",
			metadataKey: ec2.AllMacResource,
			methodName:  "AllENIMacs",
			firstVal:    macs,
			secondVal:   "01:23:45:67:89:ab",
		},
		{
			name:        "PublicIPv4Address",
			metadataKey: ec2.PublicIPv4Resource,
			methodName:  "PublicIPv4Address",
			firstVal:    "1.2.3.4",
			secondVal:   "5.6.7.8",
		},
		{
			name:        "PrivateIPv4Address",
			metadataKey: ec2.PrivateIPv4Resource,
			methodName:  "PrivateIPv4Address",
			firstVal:    "10.0.0.1",
			secondVal:   "10.0.0.2",
		},
		{
			name:        "IPv6Address",
			metadataKey: ec2.IPv6Resource,
			methodName:  "IPv6Address",
			firstVal:    "2001:aaa::1",
			secondVal:   "2001:aaa::2",
		},
		{
			name:        "SpotInstanceAction",
			metadataKey: ec2.SpotInstanceActionResource,
			methodName:  "SpotInstanceAction",
			firstVal:    "{\"action\": \"terminate\", \"time\": \"2017-09-18T08:22:00Z\"}",
			secondVal:   "{\"action\": \"stop\", \"time\": \"2017-09-18T09:00:00Z\"}",
		},
		{
			name:        "OutpostARN",
			metadataKey: ec2.OutpostARN,
			methodName:  "OutpostARN",
			firstVal:    "arn:aws:outposts:us-west-2:123456789012:outpost/op-1234567890abcdef0",
			secondVal:   "arn:aws:outposts:us-west-2:123456789012:outpost/op-abcdef1234567890",
		},
		{
			name:        "TargetLifecycleState",
			metadataKey: ec2.TargetLifecycleState,
			methodName:  "TargetLifecycleState",
			firstVal:    "InService",
			secondVal:   "Terminating",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGetter := mock_ec2.NewMockHttpClient(ctrl)
			client, err := ec2.NewEC2MetadataClient(mockGetter)
			assert.NoError(t, err)

			// Setup mock to expect TWO calls with different return values
			gomock.InOrder(
				mockGetter.EXPECT().GetMetadata(gomock.Any(), &imds.GetMetadataInput{
					Path: tc.metadataKey,
				}).Return(&imds.GetMetadataOutput{
					Content: nopReadCloser(tc.firstVal),
				}, nil),
				mockGetter.EXPECT().GetMetadata(gomock.Any(), &imds.GetMetadataInput{
					Path: tc.metadataKey,
				}).Return(&imds.GetMetadataOutput{
					Content: nopReadCloser(tc.secondVal),
				}, nil),
			)

			// Helper to call the appropriate method
			callMethod := func() (string, error) {
				switch tc.methodName {
				case "AllENIMacs":
					return client.AllENIMacs()
				case "PublicIPv4Address":
					return client.PublicIPv4Address()
				case "PrivateIPv4Address":
					return client.PrivateIPv4Address()
				case "IPv6Address":
					return client.IPv6Address()
				case "SpotInstanceAction":
					return client.SpotInstanceAction()
				case "OutpostARN":
					return client.OutpostARN()
				case "TargetLifecycleState":
					return client.TargetLifecycleState()
				default:
					return "", fmt.Errorf("unknown method: %s", tc.methodName)
				}
			}

			// First call should return first value
			result1, err := callMethod()
			assert.NoError(t, err)
			assert.Equal(t, tc.firstVal, result1)

			// Second call should make a fresh request and return second value (not cached)
			result2, err := callMethod()
			assert.NoError(t, err)
			assert.Equal(t, tc.secondVal, result2)
			assert.NotEqual(t, result1, result2, "Expected different values on successive calls (no caching)")
		})
	}
}
