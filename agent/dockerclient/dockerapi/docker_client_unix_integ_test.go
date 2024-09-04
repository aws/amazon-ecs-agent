//go:build unix && integration
// +build unix,integration

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
package dockerapi

import (
	"context"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dockerEndpoint = "unix:///var/run/docker.sock"

// This integration test checks that dockerGoClient can pull image manifest from registries.
//
// The test is skipped on environments with a Docker Engine version that does not support
// API version 1.35 as older engine versions do not have Distribution API needed for pulling
// image manifest. Technically, API version >1.30 have Distribution API but engine versions
// between API version 1.30 and 1.35 can be configured to allow image pulls from v1 registries
// but Distribution API does not work with v1 registries. v1 registry support was dropped
// with engine version 17.12 that was shipped with API version 1.35.
//
// The test depends on local test registries that are set up by `make test-registry` command.
func TestImageManifestPullInteg(t *testing.T) {
	// Prepare a docker client that can pull image manifests
	sdkClientFactory := sdkclientfactory.NewFactory(context.Background(), dockerEndpoint)
	cfg := &config.Config{}
	defaultClient, err := NewDockerGoClient(sdkClientFactory, cfg, context.Background())
	require.NoError(t, err)
	version := dockerclient.GetSupportedDockerAPIVersion(dockerclient.Version_1_35)
	supportedClient, err := defaultClient.WithVersion(version)
	if err != nil {
		t.Skipf("Skipping test due to unsupported Docker version: %v", err)
	}
	// Make retries very fast
	supportedClient.(*dockerGoClient).manifestPullBackoff = retry.NewExponentialBackoff(
		time.Nanosecond, time.Nanosecond, 1, 1)

	tcs := []struct {
		name          string
		dockerClient  DockerClient
		imageRef      string
		authData      *container.RegistryAuthenticationData
		expectedError string
	}{
		{
			name:         "private registry success",
			dockerClient: supportedClient,
			imageRef:     "127.0.0.1:51671/busybox:latest",
			authData: func() *container.RegistryAuthenticationData {
				asmAuthData := &apicontainer.ASMAuthData{}
				asmAuthData.SetDockerAuthConfig(types.AuthConfig{
					Username: "username",
					Password: "password",
				})
				return &container.RegistryAuthenticationData{Type: apicontainer.AuthTypeASM,
					ASMAuthData: asmAuthData,
				}
			}(),
		},
		{
			name:          "private registry auth failure",
			dockerClient:  supportedClient,
			imageRef:      "127.0.0.1:51671/busybox:latest",
			expectedError: "no basic auth credentials",
		},
		{
			name:         "public registry success",
			dockerClient: supportedClient,
			imageRef:     "127.0.0.1:51670/busybox:latest",
		},
		{
			name:         "public registry success, no explicit tag",
			dockerClient: supportedClient,
			imageRef:     "127.0.0.1:51670/busybox",
		},
		{
			name:         "public ECR success",
			dockerClient: supportedClient,
			imageRef:     "public.ecr.aws/amazonlinux/amazonlinux:2",
		},
		{
			name: "Docker client version too old",
			dockerClient: func() DockerClient {
				// Prepare a Docker client with an older API version
				version := dockerclient.GetSupportedDockerAPIVersion(dockerclient.Version_1_29)
				unsupportedClient, err := supportedClient.WithVersion(version)
				require.NoError(t, err)
				return unsupportedClient
			}(),
			imageRef:      "public.ecr.aws/amazonlinux/amazonlinux:2",
			expectedError: `"distribution inspect" requires API version 1.30`,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			distInspect, err := tc.dockerClient.PullImageManifest(
				context.Background(), tc.imageRef, tc.authData)
			if tc.expectedError == "" {
				require.NoError(t, err)
				assert.NotEmpty(t, distInspect.Descriptor.Digest.Encoded())
			} else {
				assert.ErrorContains(t, err, tc.expectedError)
			}
		})
	}
}
