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

package instancecreds

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetCredentials(t *testing.T) {
	credentialChain = nil
	credsA := GetCredentials(false)
	require.NotNil(t, credsA)
	credsB := GetCredentials(true)
	require.NotNil(t, credsB)
}

// test that env vars override all other provider types
func TestGetCredentials_EnvVars(t *testing.T) {
	credentialChain = nil
	// unset any env var credentials on system to run this test
	origAKID := os.Getenv("AWS_ACCESS_KEY_ID")
	origSecret := os.Getenv("AWS_SECRET_ACCESS_KEY")
	os.Setenv("AWS_ACCESS_KEY_ID", "TESTKEYID")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "TESTSECRET")
	// reset before exiting
	defer os.Setenv("AWS_ACCESS_KEY_ID", origAKID)
	defer os.Setenv("AWS_SECRET_ACCESS_KEY", origSecret)

	creds := GetCredentials(false)
	require.NotNil(t, creds)
	v, err := creds.Get()
	require.NoError(t, err)
	require.Equal(t, "EnvProvider", v.ProviderName)
	require.Equal(t, "TESTKEYID", v.AccessKeyID)
	require.Equal(t, "TESTSECRET", v.SecretAccessKey)
}

// test that shared credentials file is used when set and env vars are unset
func TestGetCredentials_SharedCredentialsFile(t *testing.T) {
	credentialChain = nil
	// unset any env var credentials to run this test
	origAKID := os.Getenv("AWS_ACCESS_KEY_ID")
	origSecret := os.Getenv("AWS_SECRET_ACCESS_KEY")
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	os.Unsetenv("AWS_SECRET_ACCESS_KEY")
	// reset before exiting test
	defer os.Setenv("AWS_ACCESS_KEY_ID", origAKID)
	defer os.Setenv("AWS_SECRET_ACCESS_KEY", origSecret)

	// create tmp AWS_SHARED_CREDENTIALS_FILE and use that for this test
	tmpFile, err := ioutil.TempFile(os.TempDir(), "credentials")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	text := []byte(`[default]
aws_access_key_id = TESTFILEKEYID
aws_secret_access_key = TESTFILESECRET
`)
	_, err = tmpFile.Write(text)
	require.NoError(t, err)
	origEnv := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE")
	os.Setenv("AWS_SHARED_CREDENTIALS_FILE", tmpFile.Name())
	// reset before exiting
	defer os.Setenv("AWS_SHARED_CREDENTIALS_FILE", origEnv)

	creds := GetCredentials(false)
	require.NotNil(t, creds)
	v, err := creds.Get()
	require.NoError(t, err)
	require.Equal(t, "SharedCredentialsProvider", v.ProviderName)
	require.Equal(t, "TESTFILEKEYID", v.AccessKeyID)
	require.Equal(t, "TESTFILESECRET", v.SecretAccessKey)
}
