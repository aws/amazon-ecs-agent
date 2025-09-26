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
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/stretchr/testify/require"
)

// Test that env vars are used when set.
func TestInstanceCredentialsCache_EnvVars(t *testing.T) {
	for _, isExternal := range []bool{true, false} {
		t.Run(fmt.Sprintf("isExternal=%t", isExternal), func(t *testing.T) {
			reset := setEnvVars(t, "TESTKEYID", "TESTSECRET")
			defer reset()

			p := NewInstanceCredentialsCache(isExternal, &nopCredsProvider{}, &nopIMDSClient{})
			creds, err := p.Retrieve(context.TODO())
			require.NotNil(t, creds)
			require.NoError(t, err)
			require.Equal(t, config.CredentialsSourceName, creds.Source)
			require.Equal(t, "TESTKEYID", creds.AccessKeyID)
			require.Equal(t, "TESTSECRET", creds.SecretAccessKey)
		})
	}
}

// Test that the shared credentials file is used when env vars are unset but
// shared credentials are set.
func TestInstanceCredentialsCache_SharedCredentialsFile(t *testing.T) {
	for _, isExternal := range []bool{true, false} {
		t.Run(fmt.Sprintf("isExternal=%t", isExternal), func(t *testing.T) {
			// unset any env var credentials
			resetEnvVars := setEnvVars(t, "", "")
			defer resetEnvVars()

			resetSharedCreds := setSharedCredentials(t, "TESTFILEKEYID", "TESTFILESECRET")
			defer resetSharedCreds()

			p := NewInstanceCredentialsCache(isExternal, &nopCredsProvider{}, &nopIMDSClient{})
			creds, err := p.Retrieve(context.TODO())
			require.NotNil(t, creds)
			require.NoError(t, err)
			require.Contains(t, creds.Source, "SharedConfigCredentials")
			require.Equal(t, "TESTFILEKEYID", creds.AccessKeyID)
			require.Equal(t, "TESTFILESECRET", creds.SecretAccessKey)
		})
	}
}

// Test that EC2 role credentials are used when env vars and
// shared credentials are unset, but the instance has an IAM role.
func TestInstanceCredentialsCache_EC2RoleCredentials(t *testing.T) {
	for _, isExternal := range []bool{true, false} {
		t.Run(fmt.Sprintf("isExternal=%t", isExternal), func(t *testing.T) {
			// unset any env var credentials
			resetEnvVars := setEnvVars(t, "", "")
			defer resetEnvVars()

			// unset any shared credentials
			sharedCredsFile := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
			os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE")
			defer os.Setenv("AWS_SHARED_CREDENTIALS_FILE", sharedCredsFile)

			p := NewInstanceCredentialsCache(isExternal, &nopCredsProvider{}, &testIMDSClient{})
			creds, err := p.Retrieve(context.TODO())
			require.NotNil(t, creds)
			require.NoError(t, err)
			require.Equal(t, ec2rolecreds.ProviderName, creds.Source)
			require.Equal(t, "TESTEC2ROLEKEYID", creds.AccessKeyID)
			require.Equal(t, "TESTEC2ROLESECRET", creds.SecretAccessKey)
		})
	}
}

// Test that the rotating shared credentials file is used when the
// default credentials chain has no credentials.
func TestInstanceCredentialsCache_RotatingSharedCredentials(t *testing.T) {
	for _, isExternal := range []bool{true, false} {
		t.Run(fmt.Sprintf("isExternal=%t", isExternal), func(t *testing.T) {
			// unset any env var credentials
			resetEnvVars := setEnvVars(t, "", "")
			defer resetEnvVars()

			// unset any shared credentials
			sharedCredsFile := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
			os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE")
			defer os.Setenv("AWS_SHARED_CREDENTIALS_FILE", sharedCredsFile)

			p := NewInstanceCredentialsCache(isExternal, &testRotatingSharedCredsProvider{}, &nopIMDSClient{})
			creds, err := p.Retrieve(context.TODO())
			require.NoError(t, err)
			require.Equal(t, RotatingSharedCredentialsProviderName, creds.Source)
			require.Equal(t, "TESTROTATINGCREDSKEYID", creds.AccessKeyID)
			require.Equal(t, "TESTROTATINGCREDSSECRET", creds.SecretAccessKey)
		})
	}
}

// Test that order of precedence is correct when the shared credentials file and
// rotating shared credentials are set.
func TestInstanceCredentialsCache_SharedCredentialsFile_RotatingSharedCredentials(t *testing.T) {
	for _, isExternal := range []bool{true, false} {
		t.Run(fmt.Sprintf("isExternal=%t", isExternal), func(t *testing.T) {
			// unset any env var credentials
			resetEnvVars := setEnvVars(t, "", "")
			defer resetEnvVars()

			// set shared credentials file
			resetSharedCreds := setSharedCredentials(t, "TESTFILEKEYID", "TESTFILESECRET")
			defer resetSharedCreds()

			p := NewInstanceCredentialsCache(isExternal, &testRotatingSharedCredsProvider{}, &nopIMDSClient{})
			creds, err := p.Retrieve(context.TODO())
			require.NoError(t, err)

			// For ECS-A rotating shared credentials take precedence over the shared credentials file.
			if isExternal {
				require.Equal(t, "TESTROTATINGCREDSKEYID", creds.AccessKeyID)
				require.Equal(t, "TESTROTATINGCREDSSECRET", creds.SecretAccessKey)
			} else {
				require.Contains(t, creds.Source, "SharedConfigCredentials")
				require.Equal(t, "TESTFILEKEYID", creds.AccessKeyID)
				require.Equal(t, "TESTFILESECRET", creds.SecretAccessKey)
			}
		})
	}
}

// Test that order of precedence is correct when the instance has an EC2Role and
// rotating shared credentials are set.
func TestInstanceCredentialsCache_EC2RoleCredentials_RotatingSharedCredentials(t *testing.T) {
	for _, isExternal := range []bool{true, false} {
		t.Run(fmt.Sprintf("isExternal=%t", isExternal), func(t *testing.T) {
			// unset any env var credentials
			resetEnvVars := setEnvVars(t, "", "")
			defer resetEnvVars()

			// unset any shared credentials
			sharedCredsFile := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
			os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE")
			defer os.Setenv("AWS_SHARED_CREDENTIALS_FILE", sharedCredsFile)

			p := NewInstanceCredentialsCache(isExternal, &testRotatingSharedCredsProvider{}, &testIMDSClient{})
			creds, err := p.Retrieve(context.TODO())
			require.NoError(t, err)

			// For ECS-A rotating shared credentials take precedence over the EC2 Role Credential.
			if isExternal {
				require.Equal(t, RotatingSharedCredentialsProviderName, creds.Source)
				require.Equal(t, "TESTROTATINGCREDSKEYID", creds.AccessKeyID)
				require.Equal(t, "TESTROTATINGCREDSSECRET", creds.SecretAccessKey)
			} else {
				require.Equal(t, ec2rolecreds.ProviderName, creds.Source)
				require.Equal(t, "TESTEC2ROLEKEYID", creds.AccessKeyID)
				require.Equal(t, "TESTEC2ROLESECRET", creds.SecretAccessKey)
			}
		})
	}
}

func TestInstanceCredentialsCache_NoValidProviders(t *testing.T) {
	for _, isExternal := range []bool{true, false} {
		t.Run(fmt.Sprintf("isExternal=%t", isExternal), func(t *testing.T) {
			// unset any env var credentials
			resetEnvVars := setEnvVars(t, "", "")
			defer resetEnvVars()

			// unset any shared credentials
			sharedCredsFile := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
			os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE")
			defer os.Setenv("AWS_SHARED_CREDENTIALS_FILE", sharedCredsFile)

			p := NewInstanceCredentialsCache(isExternal, &nopCredsProvider{}, &nopIMDSClient{})
			creds, err := p.Retrieve(context.TODO())
			require.Error(t, err)
			require.ErrorContains(t, err, "no valid providers in chain")
			require.False(t, creds.HasKeys())
		})
	}
}

func setEnvVars(t *testing.T, key string, secret string) func() {
	t.Helper()

	// unset any env var credentials
	origAKID := os.Getenv("AWS_ACCESS_KEY_ID")
	origSecret := os.Getenv("AWS_SECRET_ACCESS_KEY")
	os.Setenv("AWS_ACCESS_KEY_ID", key)
	os.Setenv("AWS_SECRET_ACCESS_KEY", secret)

	return func() {
		// reset before exiting
		os.Setenv("AWS_ACCESS_KEY_ID", origAKID)
		os.Setenv("AWS_SECRET_ACCESS_KEY", origSecret)
	}
}

func setSharedCredentials(t *testing.T, key string, secret string) func() {
	t.Helper()

	// create temp AWS_SHARED_CREDENTIALS_FILE and use that for this test
	tmpFile, err := os.CreateTemp(os.TempDir(), "credentials")
	require.NoError(t, err)

	text := []byte(`[default]
	aws_access_key_id = ` + key + `
	aws_secret_access_key = ` + secret,
	)
	_, err = tmpFile.Write(text)
	require.NoError(t, err)
	origEnv := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	os.Setenv("AWS_SHARED_CREDENTIALS_FILE", tmpFile.Name())

	return func() {
		// remove temp AWS_SHARED_CREDENTIALS_FILE
		os.Remove(tmpFile.Name())
		// reset before exiting
		os.Setenv("AWS_SHARED_CREDENTIALS_FILE", origEnv)
	}
}

type nopIMDSClient struct{}

func (c *nopIMDSClient) GetMetadata(_ context.Context, input *imds.GetMetadataInput, _ ...func(*imds.Options)) (*imds.GetMetadataOutput, error) {
	return nil, errors.New("no metadata")
}

type testIMDSClient struct{}

func (c *testIMDSClient) GetMetadata(_ context.Context, input *imds.GetMetadataInput, _ ...func(*imds.Options)) (*imds.GetMetadataOutput, error) {
	if input.Path == "/iam/security-credentials/" {
		return &imds.GetMetadataOutput{
			Content: io.NopCloser(strings.NewReader("EC2InstanceRole")),
		}, nil
	}
	return &imds.GetMetadataOutput{
		Content: io.NopCloser(strings.NewReader(`
{
	"Code": "Success",
	"AccessKeyId": "TESTEC2ROLEKEYID",
	"SecretAccessKey": "TESTEC2ROLESECRET"
}`,
		)),
	}, nil
}

type nopCredsProvider struct{}

func (p *nopCredsProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	return aws.Credentials{}, errors.New("no credentials")
}

type testRotatingSharedCredsProvider struct{}

func (p *testRotatingSharedCredsProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	return aws.Credentials{
		AccessKeyID:     "TESTROTATINGCREDSKEYID",
		SecretAccessKey: "TESTROTATINGCREDSSECRET",
		Source:          RotatingSharedCredentialsProviderName,
	}, nil
}
