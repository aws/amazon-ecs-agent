// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package credentials

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/aws-sdk-go/aws"
)

// TestIAMRoleCredentialsFromACS tests if credentials sent from ACS can be
// represented correctly as IAMRoleCredentials
func TestIAMRoleCredentialsFromACS(t *testing.T) {
	acsCredentials := &ecsacs.IAMRoleCredentials{
		CredentialsId:   aws.String("credsId"),
		AccessKeyId:     aws.String("keyId"),
		Expiration:      aws.String("soon"),
		RoleArn:         aws.String("roleArn"),
		SecretAccessKey: aws.String("OhhSecret"),
		SessionToken:    aws.String("sessionToken"),
	}
	credentials := IAMRoleCredentialsFromACS(acsCredentials)
	expectedCredentials := IAMRoleCredentials{
		CredentialsId:   "credsId",
		AccessKeyId:     "keyId",
		Expiration:      "soon",
		RoleArn:         "roleArn",
		SecretAccessKey: "OhhSecret",
		SessionToken:    "sessionToken",
	}
	if !reflect.DeepEqual(credentials, expectedCredentials) {
		t.Error("Mismatch between expected and constructed credentials")
	}
}

// TestGetTaskCredentialsUnknownId tests if GetTaskCredentials returns a false value
// when credentials for a given id are not be found in the engine
func TestGetTaskCredentialsUnknownId(t *testing.T) {
	manager := NewManager()
	_, ok := manager.GetTaskCredentials("id")
	if ok {
		t.Error("GetTaskCredentials should return false for non existing id")
	}
}

// TestSetTaskCredentialsEmptyTaskCredentials tests if credentials manager returns an
// error when invalid credentials are used to set credentials
func TestSetTaskCredentialsEmptyTaskCredentials(t *testing.T) {
	manager := NewManager()
	err := manager.SetTaskCredentials(TaskIAMRoleCredentials{})
	if err == nil {
		t.Error("Expected error adding empty task credentials")
	}
}

// TestSetTaskCredentialsNoCredentialsId tests if credentials manager returns an
// error when credentials object with no credentials id is used to set credentials
func TestSetTaskCredentialsNoCredentialsId(t *testing.T) {
	manager := NewManager()
	err := manager.SetTaskCredentials(TaskIAMRoleCredentials{ARN: "t1", IAMRoleCredentials: IAMRoleCredentials{}})
	if err == nil {
		t.Error("Expected error adding credentials payload without credential id")
	}
}

// TestSetTaskCredentialsNoTaskArn tests if credentials manager returns an
// error when credentials object with no task arn used to set credentials
func TestSetTaskCredentialsNoTaskArn(t *testing.T) {
	manager := NewManager()
	err := manager.SetTaskCredentials(TaskIAMRoleCredentials{IAMRoleCredentials: IAMRoleCredentials{CredentialsId: "id"}})
	if err == nil {
		t.Error("Expected error adding credentials payload without credential id")
	}
}

// TestSetAndGetTaskCredentialsHappyPath tests the happy path workflow for setting
// and getting credentials
func TestSetAndGetTaskCredentialsHappyPath(t *testing.T) {
	manager := NewManager()
	credentials := TaskIAMRoleCredentials{
		ARN: "t1",
		IAMRoleCredentials: IAMRoleCredentials{
			RoleArn:         "r1",
			AccessKeyId:     "akid1",
			SecretAccessKey: "skid1",
			SessionToken:    "stkn",
			Expiration:      "ts",
			CredentialsId:   "cid1",
		},
	}

	err := manager.SetTaskCredentials(credentials)
	if err != nil {
		t.Errorf("Error adding credentials: %v", err)
	}
	credentialsFromManager, ok := manager.GetTaskCredentials("cid1")
	if !ok {
		t.Error("GetTaskCredentials returned false for existing credentials")
	}
	if !reflect.DeepEqual(credentials, *credentialsFromManager) {
		t.Error("Mismatch between added and retrieved credentials")
	}

	updatedCredentials := TaskIAMRoleCredentials{
		ARN: "t1",
		IAMRoleCredentials: IAMRoleCredentials{
			RoleArn:         "r1",
			AccessKeyId:     "akid2",
			SecretAccessKey: "skid2",
			SessionToken:    "stkn2",
			Expiration:      "ts2",
			CredentialsId:   "cid1",
		},
	}
	err = manager.SetTaskCredentials(updatedCredentials)
	if err != nil {
		t.Errorf("Error updating credentials: %v", err)
	}
	credentialsFromManager, ok = manager.GetTaskCredentials("cid1")
	if !ok {
		t.Error("GetTaskCredentials returned false for existing credentials")
	}
	if !reflect.DeepEqual(updatedCredentials, *credentialsFromManager) {
		t.Error("Mismatch between added and retrieved credentials")
	}
}

// TestGenerateCredentialsEndpointRelativeURI tests if the relative credentials endpoint
// URI is generated correctly
func TestGenerateCredentialsEndpointRelativeURI(t *testing.T) {
	credentials := IAMRoleCredentials{
		RoleArn:         "r1",
		AccessKeyId:     "akid1",
		SecretAccessKey: "skid1",
		SessionToken:    "stkn",
		Expiration:      "ts",
		CredentialsId:   "cid1",
	}
	generatedURI := credentials.GenerateCredentialsEndpointRelativeURI()
	url, err := url.Parse(generatedURI)
	if err != nil {
		t.Fatalf("Error parsing url: %s, error: %v", generatedURI, err)
	}

	if CredentialsPath != url.Path {
		t.Errorf("Credentials Endpoint mismatch. Expected path: %s, got %s", CredentialsPath, url.Path)
	}

	id := url.Query().Get(CredentialsIdQueryParameterName)
	if "cid1" != id {
		t.Errorf("Credentials Endpoing mismatch. Expected value for %s: %s, got %s", CredentialsIdQueryParameterName, "cid1", id)
	}
}

// TestRemoveExistingCredentials tests that GetTaskCredentials returns false when
// credentials are removed from the credentials manager
func TestRemoveExistingCredentials(t *testing.T) {
	manager := NewManager()
	credentials := TaskIAMRoleCredentials{
		ARN: "t1",
		IAMRoleCredentials: IAMRoleCredentials{
			RoleArn:         "r1",
			AccessKeyId:     "akid1",
			SecretAccessKey: "skid1",
			SessionToken:    "stkn",
			Expiration:      "ts",
			CredentialsId:   "cid1",
		},
	}
	err := manager.SetTaskCredentials(credentials)
	if err != nil {
		t.Errorf("Error adding credentials: %v", err)
	}
	credentialsFromManager, ok := manager.GetTaskCredentials("cid1")
	if !ok {
		t.Error("GetTaskCredentials returned false for existing credentials")
	}
	if !reflect.DeepEqual(credentials, *credentialsFromManager) {
		t.Error("Mismatch between added and retrieved credentials")
	}

	manager.RemoveCredentials("cid1")
	_, ok = manager.GetTaskCredentials("cid1")
	if ok {
		t.Error("Expected GetTaskCredentials to return false for removed credentials")
	}
}
