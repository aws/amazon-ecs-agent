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

package credentials

import (
	"os"
	"testing"
)

var fakeAccessKey = "12345678901234567890"
var fakeSecretKey = fakeAccessKey + fakeAccessKey
var fakeSessionToken = fakeSecretKey + fakeSecretKey

func TestCredentialsNotSet(t *testing.T) {
	os.Clearenv()
	_, err := new(EnvironmentCredentialProvider).Credentials()
	if err == nil {
		t.Fail()
	}
}

func TestCredentialsSet(t *testing.T) {
	os.Clearenv()
	os.Setenv("AWS_ACCESS_KEY_ID", fakeAccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", fakeSecretKey)
	credentials, err := new(EnvironmentCredentialProvider).Credentials()
	if credentials.AccessKey != fakeAccessKey || credentials.SecretKey != fakeSecretKey || err != nil {
		t.Fail()
	}
}

func TestCredentialsWithSessionSet(t *testing.T) {
	os.Clearenv()
	os.Setenv("AWS_ACCESS_KEY_ID", fakeAccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", fakeSecretKey)
	os.Setenv("AWS_SESSION_TOKEN", fakeSessionToken)
	credentials, err := new(EnvironmentCredentialProvider).Credentials()
	if credentials.AccessKey != fakeAccessKey || credentials.SecretKey != fakeSecretKey || credentials.Token != fakeSessionToken || err != nil {
		t.Fail()
	}
}
