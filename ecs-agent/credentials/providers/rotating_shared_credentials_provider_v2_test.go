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
	"context"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"
)

func TestNewRotatingSharedCredentialsProviderV2(t *testing.T) {
	p := NewRotatingSharedCredentialsProviderV2()
	require.Equal(t, time.Minute, p.RotationInterval)
	require.Equal(t, "default", p.profile)
	require.Equal(t, defaultRotatingCredentialsFilename, p.file)
}

func TestNewRotatingSharedCredentialsProviderV2External(t *testing.T) {
	os.Setenv("ECS_ALTERNATE_CREDENTIAL_PROFILE", "external")
	defer os.Unsetenv("ECS_ALTERNATE_CREDENTIAL_PROFILE")
	p := NewRotatingSharedCredentialsProviderV2()
	require.Equal(t, time.Minute, p.RotationInterval)
	require.Equal(t, "external", p.profile)
	require.Equal(t, defaultRotatingCredentialsFilename, p.file)
}

func TestRotatingSharedCredentialsProviderV2_RetrieveFail_BadPath(t *testing.T) {
	p := NewRotatingSharedCredentialsProviderV2()
	p.file = "/foo/bar/baz/bad/path"
	v, err := p.Retrieve(context.TODO())
	require.Error(t, err)
	require.Equal(t, RotatingSharedCredentialsProviderName, v.Source)
}

func TestRotatingSharedCredentialsProviderV2_RetrieveFail_BadProfile(t *testing.T) {
	// create tmp credentials file and use that for this test
	tmpFile, err := os.CreateTemp(os.TempDir(), "credentials")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	text := []byte(`[thisProfileDoesntExist]
aws_access_key_id = TESTFILEKEYID
aws_secret_access_key = TESTFILESECRET
`)
	_, err = tmpFile.Write(text)
	require.NoError(t, err)

	p := NewRotatingSharedCredentialsProviderV2()
	p.file = tmpFile.Name()
	creds, err := p.Retrieve(context.TODO())
	require.Error(t, err)
	require.Equal(t, RotatingSharedCredentialsProviderName, creds.Source)
}

func TestRotatingSharedCredentialsProviderV2_Retrieve(t *testing.T) {
	// create tmp credentials file and use that for this test
	tmpFile, err := os.CreateTemp(os.TempDir(), "credentials")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	text := []byte(`[default]
aws_access_key_id = TESTFILEKEYID
aws_secret_access_key = TESTFILESECRET
`)
	_, err = tmpFile.Write(text)
	require.NoError(t, err)

	p := NewRotatingSharedCredentialsProviderV2()
	p.file = tmpFile.Name()
	creds, err := p.Retrieve(context.TODO())
	require.NoError(t, err)
	require.Equal(t, RotatingSharedCredentialsProviderName, creds.Source)
	require.Equal(t, "TESTFILEKEYID", creds.AccessKeyID)
	require.Equal(t, "TESTFILESECRET", creds.SecretAccessKey)
	require.True(t, creds.CanExpire)
}

func TestRotatingSharedCredentialsProviderV2_Retrieve_ShortSecrets(t *testing.T) {
	// create tmp credentials file and use that for this test
	tmpFile, err := os.CreateTemp(os.TempDir(), "credentials")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	text := []byte(`[default]
aws_access_key_id = A
aws_secret_access_key = B
`)
	_, err = tmpFile.Write(text)
	require.NoError(t, err)

	p := NewRotatingSharedCredentialsProviderV2()
	p.file = tmpFile.Name()
	creds, err := p.Retrieve(context.TODO())
	require.NoError(t, err)
	require.Equal(t, RotatingSharedCredentialsProviderName, creds.Source)
	require.Equal(t, "A", creds.AccessKeyID)
	require.Equal(t, "B", creds.SecretAccessKey)
	require.True(t, creds.CanExpire)
}

func TestRotatingSharedCredentialsProviderV2_RetrieveAndRefresh(t *testing.T) {
	// create tmp credentials file and use that for this test
	tmpFile, err := os.CreateTemp(os.TempDir(), "credentials")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	text := []byte(`[default]
aws_access_key_id = TESTFILEKEYID1
aws_secret_access_key = TESTFILESECRET1
`)
	_, err = tmpFile.Write(text)
	require.NoError(t, err)

	p := NewRotatingSharedCredentialsProviderV2()
	p.file = tmpFile.Name()
	p.RotationInterval = time.Second * 1

	var creds aws.Credentials
	for i := 0; i < 10; i++ {
		creds, err = p.Retrieve(context.TODO())
		require.NoError(t, err)
		require.Equal(t, RotatingSharedCredentialsProviderName, creds.Source)
		require.Equal(t, "TESTFILEKEYID1", creds.AccessKeyID)
		require.Equal(t, "TESTFILESECRET1", creds.SecretAccessKey)
		require.True(t, creds.CanExpire)

		if creds.Expired() {
			break
		}
		time.Sleep(time.Second * 1)
	}
	require.True(t, creds.Expired(), "Credentials should be expired by now")

	// overwrite the credentials file and expect to receive the new creds
	text2 := []byte(`[default]
aws_access_key_id = TESTFILEKEYID2
aws_secret_access_key = TESTFILESECRET2
`)
	_, err = tmpFile.WriteAt(text2, 0)
	require.NoError(t, err)
	creds, err = p.Retrieve(context.TODO())
	require.Equal(t, RotatingSharedCredentialsProviderName, creds.Source)
	require.Equal(t, "TESTFILEKEYID2", creds.AccessKeyID)
	require.Equal(t, "TESTFILESECRET2", creds.SecretAccessKey)
	require.True(t, creds.CanExpire)
}

// TestRotatingSharedCredentialsProviderV2_CredentialsCaching tests that our Provider
// interface operates correctly within the aws.CredentialsCache struct, which
// does caching on top of the CredentialsProvider interface
func TestRotatingSharedCredentialsProviderV2_CredentialsCaching(t *testing.T) {
	// create tmp credentials file and use that for this test
	tmpFile, err := os.CreateTemp(os.TempDir(), "credentials")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	text := []byte(`[default]
aws_access_key_id = TESTFILEKEYID1
aws_secret_access_key = TESTFILESECRET1
`)
	_, err = tmpFile.Write(text)
	require.NoError(t, err)

	p := NewRotatingSharedCredentialsProviderV2()
	p.file = tmpFile.Name()
	// set hour-long expiration for this test so we can test expiration functionality
	p.RotationInterval = time.Hour
	cache := aws.NewCredentialsCache(p)
	creds, err := cache.Retrieve(context.TODO())
	require.NoError(t, err)
	require.Equal(t, RotatingSharedCredentialsProviderName, creds.Source)
	require.Equal(t, "TESTFILEKEYID1", creds.AccessKeyID)
	require.Equal(t, "TESTFILESECRET1", creds.SecretAccessKey)
	require.True(t, creds.CanExpire)
	d := time.Until(creds.Expires)
	require.True(t, d > time.Minute*45, "Expected expiration time of creds to be more than 45 minutes in future, was %s", d)

	// overwrite cred file and verify that old values still cached
	text2 := []byte(`[default]
aws_access_key_id = TESTFILEKEYID2
aws_secret_access_key = TESTFILESECRET2
	`)
	_, err = tmpFile.WriteAt(text2, 0)
	require.NoError(t, err)
	creds, err = cache.Retrieve(context.TODO())
	require.NoError(t, err)
	require.Equal(t, RotatingSharedCredentialsProviderName, creds.Source)
	require.Equal(t, "TESTFILEKEYID1", creds.AccessKeyID)
	require.Equal(t, "TESTFILESECRET1", creds.SecretAccessKey)

	// manually expire the creds
	cache.Invalidate()
	// should have new values
	creds, err = cache.Retrieve(context.TODO())
	require.NoError(t, err)
	require.Equal(t, RotatingSharedCredentialsProviderName, creds.Source)
	require.Equal(t, "TESTFILEKEYID2", creds.AccessKeyID)
	require.Equal(t, "TESTFILESECRET2", creds.SecretAccessKey)
	require.True(t, creds.CanExpire)
}
