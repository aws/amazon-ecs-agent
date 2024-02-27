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

package providers

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/stretchr/testify/require"
)

func TestNewRotatingSharedCredentialsProvider(t *testing.T) {
	p := NewRotatingSharedCredentialsProvider()
	require.Equal(t, time.Minute, p.RotationInterval)
	require.Equal(t, "default", p.sharedCredentialsProvider.Profile)
	require.Equal(t, defaultRotatingCredentialsFilename, p.sharedCredentialsProvider.Filename)
}

func TestNewRotatingSharedCredentialsProviderExternal(t *testing.T) {
	os.Setenv("ECS_ALTERNATE_CREDENTIAL_PROFILE", "external")
	defer os.Unsetenv("ECS_ALTERNATE_CREDENTIAL_PROFILE")
	p := NewRotatingSharedCredentialsProvider()
	require.Equal(t, time.Minute, p.RotationInterval)
	require.Equal(t, "external", p.sharedCredentialsProvider.Profile)
	require.Equal(t, defaultRotatingCredentialsFilename, p.sharedCredentialsProvider.Filename)
}

func TestRotatingSharedCredentialsProvider_RetrieveFail_BadPath(t *testing.T) {
	p := NewRotatingSharedCredentialsProvider()
	p.sharedCredentialsProvider.Filename = "/foo/bar/baz/bad/path"
	v, err := p.Retrieve()
	require.Error(t, err)
	require.Equal(t, RotatingSharedCredentialsProviderName, v.ProviderName)
}

func TestRotatingSharedCredentialsProvider_RetrieveFail_BadProfile(t *testing.T) {
	// create tmp credentials file and use that for this test
	tmpFile, err := ioutil.TempFile(os.TempDir(), "credentials")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	text := []byte(`[thisProfileDoesntExist]
aws_access_key_id = TESTFILEKEYID
aws_secret_access_key = TESTFILESECRET
`)
	_, err = tmpFile.Write(text)
	require.NoError(t, err)

	p := NewRotatingSharedCredentialsProvider()
	p.sharedCredentialsProvider.Filename = tmpFile.Name()
	v, err := p.Retrieve()
	require.Error(t, err)
	require.Equal(t, RotatingSharedCredentialsProviderName, v.ProviderName)
}

func TestRotatingSharedCredentialsProvider_Retrieve(t *testing.T) {
	// create tmp credentials file and use that for this test
	tmpFile, err := ioutil.TempFile(os.TempDir(), "credentials")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	text := []byte(`[default]
aws_access_key_id = TESTFILEKEYID
aws_secret_access_key = TESTFILESECRET
`)
	_, err = tmpFile.Write(text)
	require.NoError(t, err)

	p := NewRotatingSharedCredentialsProvider()
	p.sharedCredentialsProvider.Filename = tmpFile.Name()
	v, err := p.Retrieve()
	require.NoError(t, err)
	require.Equal(t, RotatingSharedCredentialsProviderName, v.ProviderName)
	require.Equal(t, "TESTFILEKEYID", v.AccessKeyID)
	require.Equal(t, "TESTFILESECRET", v.SecretAccessKey)
}

func TestRotatingSharedCredentialsProvider_Retrieve_ShortSecrets(t *testing.T) {
	// create tmp credentials file and use that for this test
	tmpFile, err := ioutil.TempFile(os.TempDir(), "credentials")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	text := []byte(`[default]
aws_access_key_id = A
aws_secret_access_key = B
`)
	_, err = tmpFile.Write(text)
	require.NoError(t, err)

	p := NewRotatingSharedCredentialsProvider()
	p.sharedCredentialsProvider.Filename = tmpFile.Name()
	v, err := p.Retrieve()
	require.NoError(t, err)
	require.Equal(t, RotatingSharedCredentialsProviderName, v.ProviderName)
	require.Equal(t, "A", v.AccessKeyID)
	require.Equal(t, "B", v.SecretAccessKey)
}

func TestRotatingSharedCredentialsProvider_RetrieveAndRefresh(t *testing.T) {
	// create tmp credentials file and use that for this test
	tmpFile, err := ioutil.TempFile(os.TempDir(), "credentials")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	text := []byte(`[default]
aws_access_key_id = TESTFILEKEYID1
aws_secret_access_key = TESTFILESECRET1
`)
	_, err = tmpFile.Write(text)
	require.NoError(t, err)

	p := NewRotatingSharedCredentialsProvider()
	p.sharedCredentialsProvider.Filename = tmpFile.Name()
	p.RotationInterval = time.Second * 1
	v, err := p.Retrieve()
	require.NoError(t, err)
	require.Equal(t, RotatingSharedCredentialsProviderName, v.ProviderName)
	require.Equal(t, "TESTFILEKEYID1", v.AccessKeyID)
	require.Equal(t, "TESTFILESECRET1", v.SecretAccessKey)
	for i := 0; i < 10; i++ {
		if p.IsExpired() {
			break
		}
		time.Sleep(time.Second * 1)
	}
	require.True(t, p.IsExpired(), "Credentials should be expired by now")

	// overwrite the credentials file and expect to receive the new creds
	text2 := []byte(`[default]
aws_access_key_id = TESTFILEKEYID2
aws_secret_access_key = TESTFILESECRET2
`)
	_, err = tmpFile.WriteAt(text2, 0)
	require.NoError(t, err)
	v, err = p.Retrieve()
	require.Equal(t, RotatingSharedCredentialsProviderName, v.ProviderName)
	require.Equal(t, "TESTFILEKEYID2", v.AccessKeyID)
	require.Equal(t, "TESTFILESECRET2", v.SecretAccessKey)
}

// TestRotatingSharedCredentialsProvider_CredentialsCaching tests that our Provider
// interface operates correctly within the credentials.Credentials struct, which
// does caching on top of the Provider interface
func TestRotatingSharedCredentialsProvider_CredentialsCaching(t *testing.T) {
	// create tmp credentials file and use that for this test
	tmpFile, err := ioutil.TempFile(os.TempDir(), "credentials")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	text := []byte(`[default]
aws_access_key_id = TESTFILEKEYID1
aws_secret_access_key = TESTFILESECRET1
`)
	_, err = tmpFile.Write(text)
	require.NoError(t, err)

	p := NewRotatingSharedCredentialsProvider()
	p.sharedCredentialsProvider.Filename = tmpFile.Name()
	// set hour-long expiration for this test so we can test expiration functionality
	p.RotationInterval = time.Hour
	creds := credentials.NewCredentials(p)
	v, err := creds.Get()
	require.NoError(t, err)
	require.Equal(t, RotatingSharedCredentialsProviderName, v.ProviderName)
	require.Equal(t, "TESTFILEKEYID1", v.AccessKeyID)
	require.Equal(t, "TESTFILESECRET1", v.SecretAccessKey)

	ts, err := creds.ExpiresAt()
	require.NoError(t, err)
	d := time.Until(ts)
	require.True(t, d > time.Minute*45, "Expected expiration time of creds to be more than 45 minutes in future, was %s", d)

	// overwrite cred file and verify that old values still cached
	text2 := []byte(`[default]
aws_access_key_id = TESTFILEKEYID2
aws_secret_access_key = TESTFILESECRET2
	`)
	_, err = tmpFile.WriteAt(text2, 0)
	require.NoError(t, err)
	v, err = creds.Get()
	require.NoError(t, err)
	require.Equal(t, RotatingSharedCredentialsProviderName, v.ProviderName)
	require.Equal(t, "TESTFILEKEYID1", v.AccessKeyID)
	require.Equal(t, "TESTFILESECRET1", v.SecretAccessKey)

	// manually expire the creds
	creds.Expire()
	// should have new values
	v, err = creds.Get()
	require.NoError(t, err)
	require.Equal(t, RotatingSharedCredentialsProviderName, v.ProviderName)
	require.Equal(t, "TESTFILEKEYID2", v.AccessKeyID)
	require.Equal(t, "TESTFILESECRET2", v.SecretAccessKey)
}
