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

package ec2

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"testing"
	"time"
)

func MakeTestRoleCredentials() RoleCredentials {
	return RoleCredentials{
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
	TEST_ROLE_NAME = "test-role"
)

var test_client = ec2MetadataClientImpl{client: testHttpClient{}}

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

var test_response = map[string]string{
	test_client.ResourceServiceUrl(SECURITY_CREDENTIALS_RESOURCE):                  TEST_ROLE_NAME,
	test_client.ResourceServiceUrl(SECURITY_CREDENTIALS_RESOURCE + TEST_ROLE_NAME): string(ignoreError(json.Marshal(MakeTestRoleCredentials())).([]byte)),
	test_client.ResourceServiceUrl(INSTANCE_IDENTITY_DOCUMENT_RESOURCE):            testInstanceIdentityDoc,
}

type testHttpClient struct{}

// Get is a mock of the http.Client.Get that reads its responses from the map
// above and defaults to erroring.
func (c testHttpClient) Get(url string) (*http.Response, error) {
	resp, ok := test_response[url]
	if ok {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Proto:      "HTTP/1.0",
			Body:       ioutil.NopCloser(bytes.NewReader([]byte(resp))),
		}, nil
	}
	return nil, errors.New("404")
}

func TestDefaultCredentials(t *testing.T) {
	credentials, err := test_client.DefaultCredentials()
	if err != nil {
		t.Fail()
	}
	testCredentials := MakeTestRoleCredentials()
	if credentials.AccessKeyId != testCredentials.AccessKeyId {
		t.Fail()
	}
}

func TestGetInstanceIdentityDoc(t *testing.T) {
	doc, err := test_client.InstanceIdentityDocument()
	if err != nil {
		t.Fatal("Expected to be able to get doc")
	}
	if doc.Region != "us-east-1" {
		t.Error("Wrong region; expected us-east-1 but got " + doc.Region)
	}
}
