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

package dockerapi

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"io/ioutil"
	"iter"
	"net/netip"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	mock_sdkclient "github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclient/mocks"
	mock_sdkclientfactory "github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory/mocks"
	mock_ecr "github.com/aws/amazon-ecs-agent/agent/ecr/mocks"
	ec2testutil "github.com/aws/amazon-ecs-agent/agent/utils/test/ec2util"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	mock_ttime "github.com/aws/amazon-ecs-agent/ecs-agent/utils/ttime/mocks"

	"github.com/aws/aws-sdk-go-v2/aws"
	ecr_types "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/golang/mock/gomock"
	"github.com/moby/moby/api/pkg/authconfig"
	dockercontainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/api/types/network"
	mobyplugin "github.com/moby/moby/api/types/plugin"
	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/api/types/volume"
	mobyclient "github.com/moby/moby/client"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xContainerShortTimeout is a short duration intended to be used by the
// docker client APIs that test if the underlying context gets canceled
// upon the expiration of the timeout duration.
// TODO Remove mock go-dockerclient calls and fields after migration is complete.
const xContainerShortTimeout = 1 * time.Millisecond

// xImgesShortTimeout is a short duration intended to be used by the
// docker client APIs that test if the underlying context gets canceled
// upon the expiration of the timeout duration.
const xImageShortTimeout = 1 * time.Millisecond

const (
	// retry settings for pulling images mock backoff
	xMaximumPullRetries        = 5
	xMinimumPullRetryDelay     = 25 * time.Millisecond
	xMaximumPullRetryDelay     = 100 * time.Microsecond
	xPullRetryDelayMultiplier  = 2
	xPullRetryJitterMultiplier = 0.2
	dockerEventBufferSize      = 100
)

func defaultTestConfig() *config.Config {
	cfg, _ := config.NewConfig(ec2testutil.FakeEC2MetadataClient{})
	return cfg
}

func dockerClientSetup(t *testing.T) (
	*mock_sdkclient.MockClient,
	*dockerGoClient,
	*mock_ttime.MockTime,
	*gomock.Controller,
	*mock_ecr.MockECRFactory,
	func()) {
	return dockerClientSetupWithConfig(t, config.DefaultConfig(ipcompatibility.NewIPv4OnlyCompatibility()))
}

func dockerClientSetupWithConfig(t *testing.T, conf config.Config) (
	*mock_sdkclient.MockClient,
	*dockerGoClient,
	*mock_ttime.MockTime,
	*gomock.Controller,
	*mock_ecr.MockECRFactory,
	func()) {
	ctrl := gomock.NewController(t)
	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any(), gomock.Any()).Return(mobyclient.PingResult{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	mockTime := mock_ttime.NewMockTime(ctrl)
	conf.EngineAuthData = config.NewSensitiveRawMessage([]byte{})
	client, _ := NewDockerGoClient(sdkFactory, &conf, ctx)
	goClient, _ := client.(*dockerGoClient)
	ecrClientFactory := mock_ecr.NewMockECRFactory(ctrl)
	goClient.ecrClientFactory = ecrClientFactory
	goClient._time = mockTime
	goClient.imagePullBackoff = retry.NewExponentialBackoff(xMinimumPullRetryDelay, xMaximumPullRetryDelay,
		xPullRetryJitterMultiplier, xPullRetryDelayMultiplier)
	return mockDockerSDK, goClient, mockTime, ctrl, ecrClientFactory, ctrl.Finish
}

func TestPullImageOutputTimeout(t *testing.T) {
	mockDockerSDK, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	pullBeginTimeout := make(chan time.Time)
	testTime.EXPECT().After(dockerclient.DockerPullBeginTimeout).Return(pullBeginTimeout).MinTimes(1)

	// multiple invocations will happen due to retries, but all should timeout
	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), "image:latest", gomock.Any()).DoAndReturn(
		func(x, y, z interface{}) (mobyclient.ImagePullResponse, error) {
			pullBeginTimeout <- time.Now()

			reader := &mockReadCloser{
				reader: strings.NewReader(`{"status":"pull in progress"}`),
			}
			return reader, nil
		}).Times(maximumPullRetries) // expected number of retries

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, "image", nil, defaultTestConfig().ImagePullTimeout)
	assert.Error(t, metadata.Error, "Expected error for pull timeout")
	assert.Equal(t, "DockerTimeoutError", metadata.Error.(apierrors.NamedError).ErrorName())
}

func TestImagePullGlobalTimeout(t *testing.T) {
	mockDockerSDK, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	pullBeginTimeout := make(chan time.Time, 1)
	testTime.EXPECT().After(dockerclient.DockerPullBeginTimeout).Return(pullBeginTimeout)

	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), "image:latest", gomock.Any()).Do(func(x, y, z interface{}) {
		wait.Wait()
	}).Return(mockReadCloser{reader: strings.NewReader(`{"status":"pull in progress"}`)}, nil).MaxTimes(1)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, "image", nil, xContainerShortTimeout)
	assert.Error(t, metadata.Error, "Expected error for pull timeout")
	assert.Equal(t, "DockerTimeoutError", metadata.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestPullImageInactivityTimeout(t *testing.T) {
	mockDockerSDK, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	client.config.ImagePullInactivityTimeout = 1 * time.Millisecond

	testTime.EXPECT().After(gomock.Any()).AnyTimes()
	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), "image:latest", gomock.Any()).DoAndReturn(
		func(x, y, z interface{}) (mobyclient.ImagePullResponse, error) {

			reader := mockReadCloser{
				reader: strings.NewReader(`{"status":"pull in progress"}`),
				delay:  300 * time.Millisecond,
			}
			return reader, nil
		}).Times(maximumPullRetries) // expected number of retries

	client.inactivityTimeoutHandler = func(reader io.ReadCloser, timeout time.Duration, cancelRequest func(), canceled *uint32) (io.ReadCloser, chan<- struct{}) {
		assert.Equal(t, client.config.ImagePullInactivityTimeout, timeout)
		atomic.AddUint32(canceled, 1)
		return reader, make(chan struct{})
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, "image", nil, defaultTestConfig().ImagePullTimeout)
	assert.Error(t, metadata.Error, "Expected error for pull inactivity timeout")
	assert.Equal(t, "CannotPullContainerError", metadata.Error.(apierrors.NamedError).ErrorName(), "Wrong error type")
}

func TestImagePull(t *testing.T) {
	mockDockerSDK, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	testTime.EXPECT().After(gomock.Any()).AnyTimes()

	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), "image:latest", gomock.Any()).Return(
		mockReadCloser{
			reader: strings.NewReader(`{"status":"pull complete"}`),
		}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, "image", nil, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

func TestImagePullTag(t *testing.T) {
	mockDockerSDK, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()
	client.config.ImagePullInactivityTimeout = 10 * time.Second

	testTime.EXPECT().After(gomock.Any()).AnyTimes()

	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), "image:mytag", gomock.Any()).Return(
		mockReadCloser{
			reader: strings.NewReader(`{"status":"pull complete"}`),
		}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, "image:mytag", nil, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

func TestImagePullDigest(t *testing.T) {
	mockDockerSDK, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	testTime.EXPECT().After(gomock.Any()).AnyTimes()
	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), "image@sha256:bc8813ea7b3603864987522f02a76101c17ad122e1c46d790efc0fca78ca7bfb", gomock.Any()).Return(
		mockReadCloser{
			reader: strings.NewReader(`{"status":"pull complete"}`),
		}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, "image@sha256:bc8813ea7b3603864987522f02a76101c17ad122e1c46d790efc0fca78ca7bfb", nil, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

func TestPullImageECRSuccess(t *testing.T) {
	mockDockerSDK, client, mockTime, ctrl, ecrClientFactory, done := dockerClientSetup(t)
	defer done()

	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	ecrClient := mock_ecr.NewMockECRClient(ctrl)

	registryID := "123456789012"
	region := "eu-west-1"
	endpointOverride := "my.endpoint"
	authData := &apicontainer.RegistryAuthenticationData{
		Type: "ecr",
		ECRAuthData: &apicontainer.ECRAuthData{
			RegistryID:       registryID,
			Region:           region,
			EndpointOverride: endpointOverride,
		},
	}
	imageEndpoint := "registry.endpoint"
	image := imageEndpoint + "/myimage:tag"
	username := "username"
	password := "password"

	imagePullOpts := mobyclient.ImagePullOptions{
		All:          false,
		RegistryAuth: "eyJ1c2VybmFtZSI6InVzZXJuYW1lIiwicGFzc3dvcmQiOiJwYXNzd29yZCIsInNlcnZlcmFkZHJlc3MiOiJodHRwczovL3JlZ2lzdHJ5LmVuZHBvaW50In0K",
	}

	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil)
	ecrClient.EXPECT().GetAuthorizationToken(registryID).Return(
		&ecr_types.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
		}, nil)

	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), image, imagePullOpts).Return(
		mockReadCloser{
			reader: strings.NewReader(`{"status":"pull complete"}`),
		}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, image, authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

func TestPullImageManifest(t *testing.T) {
	someErr := errors.New("some error")
	testDigest, err := digest.Parse("sha256:98ea6e4f216f2fb4b69fff9b3a44842c38686ca685f3f55dc48c5d3fb1107be4")
	require.NoError(t, err)
	testDistributionInspect := registry.DistributionInspect{
		Descriptor: ocispec.Descriptor{Digest: testDigest},
	}
	type testCase struct {
		name                        string
		ctx                         context.Context
		imageRef                    string
		authData                    *apicontainer.RegistryAuthenticationData
		setSDKFactoryExpectations   func(f *mock_sdkclientfactory.MockFactory, ctrl *gomock.Controller)
		setECRClientExpectations    func(*mock_ecr.MockECRClient)
		assertError                 func(t *testing.T, err error)
		expectedDistributionInspect registry.DistributionInspect
	}
	tcs := []testCase{
		{
			name:     "failure in getting ECR auth data",
			ctx:      context.Background(),
			imageRef: "image",
			authData: &apicontainer.RegistryAuthenticationData{
				Type:        apicontainer.AuthTypeECR,
				ECRAuthData: &apicontainer.ECRAuthData{RegistryID: "registryId"},
			},
			setECRClientExpectations: func(me *mock_ecr.MockECRClient) {
				me.EXPECT().GetAuthorizationToken("registryId").Return(nil, someErr)
			},
			assertError: func(t *testing.T, err error) {
				require.Equal(t, CannotPullECRContainerError{someErr}, err)
			},
		},
		{
			name:     "Failure in getting SDK client",
			ctx:      context.Background(),
			imageRef: "image",
			setSDKFactoryExpectations: func(f *mock_sdkclientfactory.MockFactory, ctrl *gomock.Controller) {
				f.EXPECT().GetDefaultClient().Return(nil, someErr)
			},
			assertError: func(t *testing.T, err error) {
				require.Equal(t, CannotGetDockerClientError{version: "", err: someErr}, err)
			},
		},
		{
			name:     "Failure in DistributionInspect API - image URL is redacted",
			ctx:      context.Background(),
			imageRef: "image",
			setSDKFactoryExpectations: func(f *mock_sdkclientfactory.MockFactory, ctrl *gomock.Controller) {
				client := mock_sdkclient.NewMockClient(ctrl)
				client.EXPECT().
					DistributionInspect(
						gomock.Any(), "image", mobyclient.DistributionInspectOptions{EncodedRegistryAuth: base64.URLEncoding.EncodeToString([]byte("{}"))}).
					Times(maximumManifestPullRetries).
					Return(
						mobyclient.DistributionInspectResult{DistributionInspect: registry.DistributionInspect{}},
						errors.New("Some error for https://prod-us-east-1-starport-layer-bucket.s3.us-east-1.amazonaws.com"))
				f.EXPECT().GetDefaultClient().Return(client, nil)
			},
			assertError: func(t *testing.T, err error) {
				expectedErr := CannotPullImageManifestError{errors.New("Some error for REDACTED ECR URL related to image")}
				require.Equal(t, expectedErr, err)
			},
		},
		{
			name: "Error is returned if context is canceled",
			ctx: func() context.Context {
				c, cancel := context.WithCancel(context.Background())
				cancel()
				return c
			}(),
			setSDKFactoryExpectations: func(f *mock_sdkclientfactory.MockFactory, ctrl *gomock.Controller) {
				client := mock_sdkclient.NewMockClient(ctrl)
				f.EXPECT().GetDefaultClient().Return(client, nil)
			},
			imageRef: "image",
			assertError: func(t *testing.T, err error) {
				require.Equal(t,
					CannotPullImageManifestError{FromError: errors.New("context canceled")}, err)
			},
		},
		{
			name: "Error is returned if context deadline is exceeded",
			ctx: func() context.Context {
				c, cancel := context.WithTimeout(context.Background(), 0*time.Second)
				time.Sleep(2 * time.Millisecond) // Give some time for deadline to be exceeded
				cancel()
				return c
			}(),
			setSDKFactoryExpectations: func(f *mock_sdkclientfactory.MockFactory, ctrl *gomock.Controller) {
				client := mock_sdkclient.NewMockClient(ctrl)
				f.EXPECT().GetDefaultClient().Return(client, nil)
			},
			imageRef: "image",
			assertError: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "Could not transition to MANIFEST_PULLED; timed out")
			},
		},
		{
			name:     "Manifest is returned if there are no errors - no auth data",
			ctx:      context.Background(),
			imageRef: "image",
			setSDKFactoryExpectations: func(f *mock_sdkclientfactory.MockFactory, ctrl *gomock.Controller) {
				client := mock_sdkclient.NewMockClient(ctrl)
				client.EXPECT().
					DistributionInspect(
						gomock.Any(), "image", mobyclient.DistributionInspectOptions{EncodedRegistryAuth: base64.URLEncoding.EncodeToString([]byte("{}"))}).
					Return(mobyclient.DistributionInspectResult{DistributionInspect: testDistributionInspect}, nil)
				f.EXPECT().GetDefaultClient().Return(client, nil)
			},
			expectedDistributionInspect: testDistributionInspect,
		},
		func() testCase {
			authData := &apicontainer.RegistryAuthenticationData{
				Type:        apicontainer.AuthTypeASM,
				ASMAuthData: &apicontainer.ASMAuthData{},
			}
			authConfig := registry.AuthConfig{Username: "username", Password: "password"}
			authData.ASMAuthData.SetDockerAuthConfig(authConfig)
			encodedAuthConfig, err := authconfig.Encode(authConfig)
			require.NoError(t, err)
			return testCase{
				name:     "Manifest is returned if there are no errors - auth data",
				ctx:      context.Background(),
				imageRef: "image",
				authData: authData,
				setSDKFactoryExpectations: func(f *mock_sdkclientfactory.MockFactory, ctrl *gomock.Controller) {
					client := mock_sdkclient.NewMockClient(ctrl)
					client.EXPECT().
						DistributionInspect(gomock.Any(), "image", mobyclient.DistributionInspectOptions{EncodedRegistryAuth: encodedAuthConfig}).
						Return(mobyclient.DistributionInspectResult{DistributionInspect: testDistributionInspect}, nil)
					f.EXPECT().GetDefaultClient().Return(client, nil)
				},
				expectedDistributionInspect: testDistributionInspect,
			}
		}(),
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
			mockDockerSDK.EXPECT().Ping(gomock.Any(), gomock.Any()).Return(mobyclient.PingResult{}, nil)
			sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
			sdkFactory.EXPECT().GetDefaultClient().Return(mockDockerSDK, nil)

			client, err := NewDockerGoClient(sdkFactory, defaultTestConfig(), context.Background())
			require.NoError(t, err)

			if tc.setSDKFactoryExpectations != nil {
				tc.setSDKFactoryExpectations(sdkFactory, ctrl)
			}

			ecrClientFactory := mock_ecr.NewMockECRFactory(ctrl)
			ecrClient := mock_ecr.NewMockECRClient(ctrl)
			client.(*dockerGoClient).ecrClientFactory = ecrClientFactory
			client.(*dockerGoClient).manifestPullBackoff = retry.NewExponentialBackoff(
				1*time.Nanosecond, 1*time.Nanosecond, 1, 1)

			if tc.setECRClientExpectations != nil {
				ecrClientFactory.EXPECT().GetClient(tc.authData.ECRAuthData).Return(ecrClient, nil)
				tc.setECRClientExpectations(ecrClient)
			}

			res, err := client.PullImageManifest(tc.ctx, tc.imageRef, tc.authData)
			if tc.assertError != nil {
				tc.assertError(t, err)
			} else {
				require.Nil(t, err)
				assert.Equal(t, tc.expectedDistributionInspect, res)
			}
		})
	}
}

func TestPullImageECRAuthFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any(), gomock.Any()).Return(mobyclient.PingResult{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	client, _ := NewDockerGoClient(sdkFactory, defaultTestConfig(), ctx)
	goClient, _ := client.(*dockerGoClient)
	ecrClientFactory := mock_ecr.NewMockECRFactory(ctrl)
	ecrClient := mock_ecr.NewMockECRClient(ctrl)
	mockTime := mock_ttime.NewMockTime(ctrl)
	goClient.ecrClientFactory = ecrClientFactory
	goClient._time = mockTime

	mockTime.EXPECT().After(gomock.Any()).AnyTimes()

	registryID := "123456789012"
	region := "eu-west-1"
	endpointOverride := "my.endpoint"
	authData := &apicontainer.RegistryAuthenticationData{
		Type: "ecr",
		ECRAuthData: &apicontainer.ECRAuthData{
			RegistryID:       registryID,
			Region:           region,
			EndpointOverride: endpointOverride,
		},
	}
	imageEndpoint := "registry.endpoint"
	image := imageEndpoint + "/myimage:tag"

	// no retries for this error
	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil)
	ecrClient.EXPECT().GetAuthorizationToken(gomock.Any()).Return(nil, errors.New("test error"))

	metadata := client.PullImage(ctx, image, authData, defaultTestConfig().ImagePullTimeout)
	assert.Error(t, metadata.Error, "expected pull to fail")
}

func TestPullImageError(t *testing.T) {
	mockDockerSDK, client, testTime, _, _, _ := dockerClientSetup(t)

	testTime.EXPECT().After(gomock.Any()).AnyTimes()
	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), "image:latest", gomock.Any()).DoAndReturn(
		func(x, y, z interface{}) (mobyclient.ImagePullResponse, error) {

			reader := mockReadCloser{
				reader: strings.NewReader(`{"error":"toomanyrequests: Rate exceeded"}`),
				delay:  300 * time.Millisecond,
			}
			return reader, nil
		}).Times(maximumPullRetries) // expected number of retries

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, "image", nil, defaultTestConfig().ImagePullTimeout)
	assert.Error(t, metadata.Error, "toomanyrequests: Rate exceeded")
	assert.Equal(t, "CannotPullContainerError", metadata.Error.(apierrors.NamedError).ErrorName(), "Wrong error type")
}

type mockReadCloser struct {
	reader io.Reader
	delay  time.Duration
}

func (mr mockReadCloser) Read(data []byte) (n int, err error) {
	time.Sleep(mr.delay)
	return mr.reader.Read(data)
}
func (mr mockReadCloser) Close() error {
	return nil
}

// JSONMessages and Wait let mockReadCloser satisfy moby v29's
// client.ImagePullResponse interface. The agent's pull path consumes the
// plain io.Reader, so these are stubs.
func (mr mockReadCloser) JSONMessages(ctx context.Context) iter.Seq2[jsonstream.Message, error] {
	return nil
}
func (mr mockReadCloser) Wait(ctx context.Context) error {
	return nil
}
func TestGetRepositoryWithTaggedImage(t *testing.T) {
	image := "registry.endpoint/myimage:tag"
	repository := getRepository(image)

	assert.Equal(t, image, repository)
}

func TestGetRepositoryWithUntaggedImage(t *testing.T) {
	image := "registry.endpoint/myimage"
	repository := getRepository(image)

	assert.Equal(t, image+":"+dockerDefaultTag, repository)
}

func TestCreateContainerTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	hostConfig := &dockercontainer.HostConfig{Resources: dockercontainer.Resources{Memory: 100}}
	mockDockerSDK.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).Do(func(u, v interface{}) {
		wait.Wait()
	}).MaxTimes(1).Return(mobyclient.ContainerCreateResult{}, errors.New("test error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.CreateContainer(ctx, &dockercontainer.Config{}, hostConfig, "containerName", xContainerShortTimeout)
	assert.Error(t, metadata.Error, "expected error for pull timeout")
	assert.Equal(t, "DockerTimeoutError", metadata.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestCreateContainer(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	name := "containerName"
	hostConfig := &dockercontainer.HostConfig{Resources: dockercontainer.Resources{Memory: 100}}
	gomock.InOrder(
		mockDockerSDK.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
			Do(func(u, v interface{}) {
				opts := v.(mobyclient.ContainerCreateOptions)
				assert.True(t, reflect.DeepEqual(opts.HostConfig, hostConfig),
					"Mismatch in create container HostConfig, %v != %v", opts.HostConfig, hostConfig)
				assert.Equal(t, opts.Name, name,
					"Mismatch in create container options, %s != %s", opts.Name, name)
			}).Return(mobyclient.ContainerCreateResult{ID: "id"}, nil),
		mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "id", gomock.Any()).
			Return(mobyclient.ContainerInspectResult{Container: dockercontainer.InspectResponse{ID: "id"}}, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.CreateContainer(ctx, nil, hostConfig, name, defaultTestConfig().ContainerCreateTimeout)
	assert.NoError(t, metadata.Error)
	assert.Equal(t, "id", metadata.DockerID)
	assert.Nil(t, metadata.ExitCode, "Expected a created container to not have an exit code")
}

func TestCreateContainerExecTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	execConfig := mobyclient.ExecCreateOptions{
		Privileged:   false,
		AttachStdin:  false,
		AttachStderr: false,
		AttachStdout: false,
		DetachKeys:   "",
		Env:          []string{},
		Cmd:          []string{"ls"},
	}

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ExecCreate(gomock.Any(), gomock.Any(), execConfig).Do(func(v, w, x interface{}) {
		wait.Wait() // wait until timeout happens
	}).MaxTimes(1)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	_, err := client.CreateContainerExec(ctx, "id", execConfig, xContainerShortTimeout)
	assert.NotNil(t, err, "Expected error for create container exec")
	assert.Equal(t, "DockerTimeoutError", err.(apierrors.NamedError).ErrorName(), "Wrong error type")
	wait.Done()
}

func TestCreateContainerExec(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	name := "containerName"
	execEnv := make([]string, 0)
	execCmd := make([]string, 0)
	execCmd = append(execCmd, "ls")
	execConfig := mobyclient.ExecCreateOptions{
		Privileged:   false,
		AttachStdin:  false,
		AttachStderr: false,
		AttachStdout: false,
		DetachKeys:   "",
		Env:          execEnv,
		Cmd:          execCmd,
	}

	execCreateResponse := mobyclient.ExecCreateResult{ID: "id"}

	gomock.InOrder(
		mockDockerSDK.EXPECT().ExecCreate(gomock.Any(), gomock.Any(), execConfig).
			Do(func(v, w, x interface{}) {
				assert.True(t, reflect.DeepEqual(x, execConfig),
					"Mismatch in create container ExecConfig, %v != %v", x, execConfig)
			}).Return(execCreateResponse, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	response, err := client.CreateContainerExec(ctx, name, execConfig, dockerclient.ContainerExecCreateTimeout)
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, execCreateResponse, *response)
}

func TestStartContainerExecTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	execStartCheck := mobyclient.ExecStartOptions{
		Detach: true,
		TTY:    false,
	}

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ExecStart(gomock.Any(), "id", execStartCheck).Do(func(x, y, z interface{}) {
		wait.Wait() // wait until timeout happens
	}).MaxTimes(1).Return(mobyclient.ExecStartResult{}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.StartContainerExec(ctx, "id", mobyclient.ExecStartOptions{Detach: true, TTY: false}, xContainerShortTimeout)
	assert.NotNil(t, err, "Expected error for start container exec")
	assert.Equal(t, "DockerTimeoutError", err.(apierrors.NamedError).ErrorName(), "Wrong error type")
	wait.Done()
}

func TestStartContainerExec(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	execStartCheck := mobyclient.ExecStartOptions{
		Detach: true,
		TTY:    false,
	}

	gomock.InOrder(
		mockDockerSDK.EXPECT().ExecStart(gomock.Any(), "id", execStartCheck).Return(mobyclient.ExecStartResult{}, nil),
	)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.StartContainerExec(ctx, "id", mobyclient.ExecStartOptions{Detach: true, TTY: false}, dockerclient.ContainerExecStartTimeout)
	assert.NoError(t, err)
}

func TestInspectContainerExecTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ExecInspect(gomock.Any(), "id", gomock.Any()).Do(func(x, y, z interface{}) {
		wait.Wait() // wait until timeout happens
	}).MaxTimes(1)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	_, err := client.InspectContainerExec(ctx, "id", xContainerShortTimeout)
	assert.NotNil(t, err, "Expected error for inspect container exec")
	assert.Equal(t, "DockerTimeoutError", err.(apierrors.NamedError).ErrorName(), "Wrong error type")
	wait.Done()
}

func TestInspectContainerExec(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	inspectContainerResponse := mobyclient.ExecInspectResult{
		ID:          "id",
		ContainerID: "cont",
		Running:     true,
		ExitCode:    0,
		PID:         25537,
	}
	gomock.InOrder(
		mockDockerSDK.EXPECT().ExecInspect(gomock.Any(), "id", gomock.Any()).Return(inspectContainerResponse, nil),
	)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	resp, err := client.InspectContainerExec(ctx, "id", dockerclient.ContainerExecInspectTimeout)
	assert.NoError(t, err)
	assert.Equal(t, "id", resp.ID)
	assert.Equal(t, "cont", resp.ContainerID)
	assert.Equal(t, true, resp.Running)
	assert.Equal(t, 0, resp.ExitCode)
	assert.Equal(t, 25537, resp.PID)
}

func TestStartContainerTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ContainerStart(gomock.Any(), "id", mobyclient.ContainerStartOptions{}).Do(func(x, y, z interface{}) {
		wait.Wait() // wait until timeout happens
	}).MaxTimes(1)
	mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "id", gomock.Any()).Return(mobyclient.ContainerInspectResult{}, errors.New("test error")).AnyTimes()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.StartContainer(ctx, "id", xContainerShortTimeout)
	assert.NotNil(t, metadata.Error, "Expected error for pull timeout")
	assert.Equal(t, "DockerTimeoutError", metadata.Error.(apierrors.NamedError).ErrorName(), "Wrong error type")
	wait.Done()
}

func TestStartContainer(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	gomock.InOrder(
		mockDockerSDK.EXPECT().ContainerStart(gomock.Any(), "id", mobyclient.ContainerStartOptions{}).Return(mobyclient.ContainerStartResult{}, nil),
		mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "id", gomock.Any()).
			Return(mobyclient.ContainerInspectResult{Container: dockercontainer.InspectResponse{ID: "id"}}, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.StartContainer(ctx, "id", defaultTestConfig().ContainerStartTimeout)
	assert.NoError(t, metadata.Error)
	assert.Equal(t, "id", metadata.DockerID)
}

func TestStopContainerTimeout(t *testing.T) {
	cfg := config.DefaultConfig(ipcompatibility.NewIPv4OnlyCompatibility())
	cfg.DockerStopTimeout = xContainerShortTimeout
	mockDockerSDK, client, _, _, _, done := dockerClientSetupWithConfig(t, cfg)
	defer done()
	reset := stopContainerTimeoutBuffer
	stopContainerTimeoutBuffer = xContainerShortTimeout
	defer func() {
		stopContainerTimeoutBuffer = reset
	}()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	timeoutSeconds := int(client.config.DockerStopTimeout.Seconds())
	containerOptions := mobyclient.ContainerStopOptions{
		Timeout: &timeoutSeconds,
	}
	mockDockerSDK.EXPECT().ContainerStop(gomock.Any(), "id", containerOptions).Do(func(x, y, z interface{}) {
		wait.Wait()
		// Don't return, verify timeout happens
	}).MaxTimes(1).Return(mobyclient.ContainerStopResult{}, errors.New("test error"))
	mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.StopContainer(ctx, "id", xContainerShortTimeout)
	assert.Error(t, metadata.Error, "Expected error for stop timeout")
	assert.Equal(t, "DockerTimeoutError", metadata.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestStopContainer(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	timeoutSeconds := int(client.config.DockerStopTimeout.Seconds())
	containerOptions := mobyclient.ContainerStopOptions{
		Timeout: &timeoutSeconds,
	}
	gomock.InOrder(
		mockDockerSDK.EXPECT().ContainerStop(gomock.Any(), "id", containerOptions).Return(mobyclient.ContainerStopResult{}, nil),
		mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "id", gomock.Any()).
			Return(mobyclient.ContainerInspectResult{Container: dockercontainer.InspectResponse{ID: "id", State: &dockercontainer.State{ExitCode: 10}, Config: &dockercontainer.Config{}}}, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.StopContainer(ctx, "id", client.config.DockerStopTimeout)
	assert.NoError(t, metadata.Error)
	assert.Equal(t, "id", metadata.DockerID)
}

func TestRemoveContainerTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ContainerRemove(gomock.Any(), "id",
		mobyclient.ContainerRemoveOptions{
			RemoveVolumes: true,
			RemoveLinks:   false,
			Force:         false,
		}).Do(func(x, y, z interface{}) {
		wait.Wait() // wait until timeout happens
	}).MaxTimes(1)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	err := client.RemoveContainer(ctx, "id", xContainerShortTimeout)
	assert.Error(t, err, "Expected error for remove timeout")
	assert.Equal(t, "DockerTimeoutError", err.(apierrors.NamedError).ErrorName(), "Wrong error type")
	wait.Done()
}

func TestRemoveContainer(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().ContainerRemove(gomock.Any(), "id",
		mobyclient.ContainerRemoveOptions{
			RemoveVolumes: true,
			RemoveLinks:   false,
			Force:         false,
		}).Return(mobyclient.ContainerRemoveResult{}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.RemoveContainer(ctx, "id", dockerclient.RemoveContainerTimeout)
	assert.NoError(t, err)
}

func TestInspectContainerTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "id", gomock.Any()).Do(func(ctx, x, y interface{}) {
		wait.Wait()
		// Don't return, verify timeout happens
	}).MaxTimes(1).Return(mobyclient.ContainerInspectResult{}, errors.New("test error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	_, err := client.InspectContainer(ctx, "id", xContainerShortTimeout)
	assert.Error(t, err, "Expected error for inspect timeout")
	assert.Equal(t, "DockerTimeoutError", err.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestInspectContainer(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	containerOutput := dockercontainer.InspectResponse{

		ID: "id",
		State: &dockercontainer.State{
			ExitCode: 10,
			Health: &dockercontainer.Health{
				Status: "healthy",
				Log: []*dockercontainer.HealthcheckResult{
					{
						ExitCode: 1,
						Output:   "health output",
					},
				},
			},
		}}
	gomock.InOrder(
		mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "id", gomock.Any()).Return(mobyclient.ContainerInspectResult{Container: containerOutput}, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	container, err := client.InspectContainer(ctx, "id", dockerclient.InspectContainerTimeout)
	assert.NoError(t, err)
	assert.True(t, reflect.DeepEqual(&containerOutput, container))
}

func TestContainerEvents(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	eventsChan := make(chan events.Message, dockerEventBufferSize)
	errChan := make(chan error)
	mockDockerSDK.EXPECT().Events(gomock.Any(), gomock.Any()).Return(mobyclient.EventsResult{Messages: eventsChan, Err: errChan})

	dockerEvents, err := client.ContainerEvents(context.TODO())
	require.NoError(t, err, "Could not get container events")
	go func() {
		eventsChan <- events.Message{Type: "container", Actor: events.Actor{ID: "containerId"}, Action: "create"}
	}()

	event := <-dockerEvents
	assert.Equal(t, event.DockerID, "containerId", "Wrong docker id")
	assert.Equal(t, event.Status, apicontainerstatus.ContainerCreated, "Wrong status")

	container := dockercontainer.InspectResponse{

		ID: "cid2",

		NetworkSettings: &dockercontainer.NetworkSettings{

			Ports: network.PortMap{
				network.MustParsePort("80/tcp"): []network.PortBinding{{HostPort: "9001"}},
			},
		},
		Config: &dockercontainer.Config{},
		Mounts: []dockercontainer.MountPoint{
			{Source: "/host/path",
				Destination: "/container/path"},
		},
	}

	mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "cid2", gomock.Any()).Return(mobyclient.ContainerInspectResult{Container: container}, nil)
	go func() {
		eventsChan <- events.Message{Type: "container", Actor: events.Actor{ID: "cid2"}, Action: "start"}
	}()
	event = <-dockerEvents
	assert.Equal(t, event.DockerID, "cid2", "Wrong docker id")
	assert.Equal(t, event.Status, apicontainerstatus.ContainerRunning, "Wrong status")
	assert.Equal(t, event.PortBindings[0].ContainerPort, uint16(80), "Incorrect port bindings")
	assert.Equal(t, event.PortBindings[0].HostPort, uint16(9001), "Incorrect port bindings")
	assert.Equal(t, event.Volumes[0].Source, "/host/path", "Incorrect volume mapping")
	assert.Equal(t, event.Volumes[0].Destination, "/container/path", "Incorrect volume mapping")

	for i := 0; i < 2; i++ {
		stoppedContainer := dockercontainer.InspectResponse{

			ID: "cid3" + strconv.Itoa(i),
			State: &dockercontainer.State{
				FinishedAt: (time.Now()).Format(time.RFC3339),
				ExitCode:   20,
			},
		}
		mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "cid3"+strconv.Itoa(i), gomock.Any()).Return(mobyclient.ContainerInspectResult{Container: stoppedContainer}, nil)
	}
	go func() {
		eventsChan <- events.Message{Type: "container", Actor: events.Actor{ID: "cid30"}, Action: "stop"}
		eventsChan <- events.Message{Type: "container", Actor: events.Actor{ID: "cid31"}, Action: "die"}
	}()

	for i := 0; i < 2; i++ {
		anEvent := <-dockerEvents
		assert.True(t, anEvent.DockerID == "cid30" || anEvent.DockerID == "cid31", "Wrong container id: "+anEvent.DockerID)
		assert.Equal(t, anEvent.Status, apicontainerstatus.ContainerStopped, "Should be stopped")
		assert.Equal(t, aws.ToInt(anEvent.ExitCode), 20, "Incorrect exit code")
	}

	containerWithHealthInfo := dockercontainer.InspectResponse{
		ID: "container_health",
		State: &dockercontainer.State{
			Health: &dockercontainer.Health{
				Status: "healthy",
				Log: []*dockercontainer.HealthcheckResult{
					{
						ExitCode: 1,
						Output:   "health output",
					},
				},
			},
		},
	}
	mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), "container_health", gomock.Any()).Return(mobyclient.ContainerInspectResult{Container: containerWithHealthInfo}, nil)
	go func() {
		eventsChan <- events.Message{
			Type:   "container",
			Action: "health_status: unhealthy",
			Actor: events.Actor{
				ID: "container_health",
			},
		}
	}()

	anEvent := <-dockerEvents
	assert.Equal(t, anEvent.Type, apicontainer.ContainerHealthEvent, "unexpected docker events type received")
	assert.Equal(t, anEvent.Health.Status, apicontainerstatus.ContainerHealthy)
	assert.Equal(t, anEvent.Health.Output, "health output")

	// Verify the following events do not translate into our event stream

	//
	// Docker 1.8.3 sends the full command appended to exec_create and exec_start
	// events. Test that we ignore there as well..
	//
	ignore := []string{
		"pause",
		"exec_create",
		"exec_create: /bin/bash",
		"exec_start",
		"exec_start: /bin/bash",
		"top",
		"attach",
		"export",
		"pull",
		"push",
		"tag",
		"untag",
		"import",
		"delete",
		"oom",
		"kill",
	}
	for _, eventStatus := range ignore {
		eventsChan <- events.Message{Type: "container", Actor: events.Actor{ID: "123"}, Action: events.Action(eventStatus)}
		select {
		case <-dockerEvents:
			t.Error("No event should be available for " + eventStatus)
		default:
		}
	}

	// Verify only the container type event will translate to our event stream
	// Events type: network, image, volume, daemon, plugins won't be handled
	ignoreEventType := map[events.Type]string{
		"network": "connect",
		"image":   "pull",
		"volume":  "create",
		"plugin":  "install",
		"daemon":  "reload",
	}

	for eventType, eventStatus := range ignoreEventType {
		eventsChan <- events.Message{Type: events.Type(eventType), Actor: events.Actor{ID: "123"}, Action: events.Action(eventStatus)}
		select {
		case <-dockerEvents:
			t.Errorf("No event should be available for %v", eventType)
		default:
		}
	}
}

func TestContainerEventsError(t *testing.T) {
	testCases := []struct {
		name string
		err  error
	}{
		{
			name: "EOF error",
			err:  io.EOF,
		},
		{
			name: "Unexpected EOF error",
			err:  io.ErrUnexpectedEOF,
		},
		{
			name: "other error",
			err:  errors.New("test error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
			defer done()

			eventsChan := make(chan events.Message, dockerEventBufferSize)
			errChan := make(chan error)
			mockDockerSDK.EXPECT().Events(gomock.Any(), gomock.Any()).Return(mobyclient.EventsResult{Messages: eventsChan, Err: errChan}).MinTimes(1)

			dockerEvents, err := client.ContainerEvents(context.TODO())
			require.NoError(t, err, "Could not get container events")
			go func() {
				errChan <- tc.err
				eventsChan <- events.Message{Type: "container", Actor: events.Actor{ID: "containerId"}, Action: "create"}
			}()

			event := <-dockerEvents
			assert.Equal(t, event.DockerID, "containerId", "Wrong docker id")
			assert.Equal(t, event.Status, apicontainerstatus.ContainerCreated, "Wrong status")
		})
	}
}

func TestSetExitCodeFromEvent(t *testing.T) {
	var (
		exitCodeInt    = 42
		exitCodeStr    = "42"
		altExitCodeInt = 1
	)

	defaultEvent := &events.Message{
		Action: dockerContainerDieEvent,
		Actor: events.Actor{
			Attributes: map[string]string{
				dockerContainerEventExitCodeAttribute: exitCodeStr,
			},
		},
	}

	testCases := []struct {
		name             string
		event            *events.Message
		metadata         DockerContainerMetadata
		expectedExitCode *int
	}{
		{
			name:             "exit code set from event",
			event:            defaultEvent,
			metadata:         DockerContainerMetadata{},
			expectedExitCode: &exitCodeInt,
		},
		{
			name:  "exit code not set from event when metadata already has it",
			event: defaultEvent,
			metadata: DockerContainerMetadata{
				ExitCode: &altExitCodeInt,
			},
			expectedExitCode: &altExitCodeInt,
		},
		{
			name: "exit code not set from event when event does not has it",
			event: &events.Message{
				Action: dockerContainerDieEvent,
				Actor:  events.Actor{},
			},
			metadata:         DockerContainerMetadata{},
			expectedExitCode: nil,
		},
		{
			name: "exit code not set from event when event has invalid exit code",
			event: &events.Message{
				Action: dockerContainerDieEvent,
				Actor: events.Actor{
					Attributes: map[string]string{
						dockerContainerEventExitCodeAttribute: "invalid",
					},
				},
			},
			metadata:         DockerContainerMetadata{},
			expectedExitCode: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setExitCodeFromEvent(tc.event, &tc.metadata)
			assert.Equal(t, tc.expectedExitCode, tc.metadata.ExitCode)
		})
	}
}

func TestDockerVersion(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().ServerVersion(gomock.Any(), gomock.Any()).Return(mobyclient.ServerVersionResult{Version: "1.6.0"}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	str, err := client.Version(ctx, dockerclient.VersionTimeout)
	assert.NoError(t, err)
	assert.Equal(t, "1.6.0", str, "Got unexpected version string: "+str)
}

func TestSystemPing(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().Ping(gomock.Any(), gomock.Any()).Return(mobyclient.PingResult{APIVersion: "test_docker_api"}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	pingResponse := client.SystemPing(ctx, dockerclient.InfoTimeout)

	assert.NoError(t, pingResponse.Error)
	assert.Equal(t, "test_docker_api", pingResponse.Response.APIVersion)
}

func TestSystemPingError(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().Ping(gomock.Any(), gomock.Any()).Return(mobyclient.PingResult{}, errors.New("test error"))

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	pingResponse := client.SystemPing(ctx, dockerclient.InfoTimeout)

	assert.Error(t, pingResponse.Error)
	assert.Nil(t, pingResponse.Response)
}

func TestDockerInfo(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().Info(gomock.Any(), gomock.Any()).Return(mobyclient.SystemInfoResult{Info: system.Info{SecurityOptions: []string{"selinux"}}}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	info, err := client.Info(ctx, dockerclient.InfoTimeout)

	assert.NoError(t, err)
	assert.Equal(t, []string{"selinux"}, info.SecurityOptions)
}

func TestDockerInfoError(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	errorMsg := "Error getting  docker info"

	mockDockerSDK.EXPECT().Info(gomock.Any(), gomock.Any()).Return(mobyclient.SystemInfoResult{Info: system.Info{}}, errors.New(errorMsg))

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	info, err := client.Info(ctx, dockerclient.InfoTimeout)

	assert.Error(t, err, errorMsg)
	assert.Equal(t, system.Info{}, info)
}

func TestDockerInfoClientError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	errorMsg := "Error getting client"

	// Mock SDKFactory
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any(), gomock.Any()).Return(mobyclient.PingResult{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// Return the Docker Go client for the first call
	sdkFactory.EXPECT().GetDefaultClient().Times(1).Return(mockDockerSDK, nil)
	client, err := NewDockerGoClient(sdkFactory, defaultTestConfig(), ctx)
	assert.NoError(t, err)

	// Throw error when `Info` tries to get the client
	sdkFactory.EXPECT().GetDefaultClient().Return(nil, errors.New(errorMsg))
	info, err := client.Info(ctx, dockerclient.InfoTimeout)

	assert.Error(t, err, errorMsg)
	assert.Equal(t, system.Info{}, info)
}

func TestDockerVersionCached(t *testing.T) {
	_, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	// Explicitly set daemon version so that mockDocker (the docker client)
	// is not invoked again
	client.setDaemonVersion("1.6.0")
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	str, err := client.Version(ctx, dockerclient.VersionTimeout)
	assert.NoError(t, err)
	assert.Equal(t, "1.6.0", str, "Got unexpected version string: "+str)
}

func TestListContainers(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	containers := []dockercontainer.Summary{{ID: "id"}}
	mockDockerSDK.EXPECT().ContainerList(gomock.Any(), mobyclient.ContainerListOptions{All: true}).Return(mobyclient.ContainerListResult{Items: containers}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListContainers(ctx, true, dockerclient.ListContainersTimeout)
	assert.NoError(t, response.Error)

	containerIds := response.DockerIDs
	assert.Equal(t, 1, len(containerIds), "Unexpected number of containers in list: ", len(containerIds))
	assert.Equal(t, "id", containerIds[0], "Unexpected container id in the list: ", containerIds[0])
}

func TestListContainersTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ContainerList(gomock.Any(), mobyclient.ContainerListOptions{All: true}).Do(func(x, y interface{}) {
		wait.Wait()
		// Don't return, verify timeout happens
	}).MaxTimes(1).Return(mobyclient.ContainerListResult{}, errors.New("test error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListContainers(ctx, true, xContainerShortTimeout)
	assert.Error(t, response.Error, "Expected error for pull timeout")
	assert.Equal(t, "DockerTimeoutError", response.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestListImages(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	images := []image.Summary{{ID: "id"}}
	mockDocker.EXPECT().ImageList(gomock.Any(), gomock.Any()).Return(mobyclient.ImageListResult{Items: images}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListImages(ctx, dockerclient.ListImagesTimeout)
	assert.NoError(t, response.Error, "Did not expect error")

	imageIDs := response.ImageIDs
	assert.EqualValues(t, len(imageIDs), 1, "Unexpected number of images in list")
	assert.EqualValues(t, imageIDs[0], "id", "Unexpected id in list of images")
}

func TestListImagesTimeout(t *testing.T) {
	mockDocker, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDocker.EXPECT().ImageList(gomock.Any(), gomock.Any()).Do(func(x, y interface{}) {
		wait.Wait()
		// Don't return, verify timeout happens
	}).MaxTimes(1).Return(mobyclient.ImageListResult{}, errors.New("test error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	response := client.ListImages(ctx, xImageShortTimeout)
	assert.Error(t, response.Error, "Expected error for pull timeout")
	assert.Equal(t, response.Error.(apierrors.NamedError).ErrorName(), "DockerTimeoutError", "Wrong error type")

	wait.Done()
}

// Test for constructor fail when Docker SDK Client Ping() fails
func TestPingSdkFailError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any(), gomock.Any()).Return(mobyclient.PingResult{}, errors.New("test error"))
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	_, err := NewDockerGoClient(sdkFactory, defaultTestConfig(), ctx)
	assert.Error(t, err, "Expected ping error to result in constructor fail")
}

func TestUsesVersionedClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any(), gomock.Any()).Return(mobyclient.PingResult{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	client, err := NewDockerGoClient(sdkFactory, defaultTestConfig(), ctx)
	assert.NoError(t, err)

	sdkFactory.EXPECT().
		GetClient(dockerclient.DockerVersion("1.20")).
		Return(mockDockerSDK, nil)

	vclient, err := client.WithVersion(dockerclient.DockerVersion("1.20"))
	require.NoError(t, err)

	sdkFactory.EXPECT().GetClient(dockerclient.DockerVersion("1.20")).Times(2).Return(mockDockerSDK, nil)
	mockDockerSDK.EXPECT().ContainerStart(gomock.Any(), gomock.Any(), mobyclient.ContainerStartOptions{}).Return(mobyclient.ContainerStartResult{}, nil)
	mockDockerSDK.EXPECT().ContainerInspect(gomock.Any(), gomock.Any(), gomock.Any()).Return(mobyclient.ContainerInspectResult{}, errors.New("test error"))
	vclient.StartContainer(ctx, "foo", defaultTestConfig().ContainerStartTimeout)
}

func TestUnavailableVersionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	// Docker SDK tests
	mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
	mockDockerSDK.EXPECT().Ping(gomock.Any(), gomock.Any()).Return(mobyclient.PingResult{}, nil)
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(mockDockerSDK, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	client, err := NewDockerGoClient(sdkFactory, defaultTestConfig(), ctx)
	assert.NoError(t, err)

	sdkFactory.EXPECT().
		GetClient(dockerclient.DockerVersion("1.21")).
		Return(nil, errors.New("Cannot get client"))

	vclient, err := client.WithVersion(dockerclient.DockerVersion("1.21"))
	require.EqualError(t, err, "Cannot get client")

	sdkFactory.EXPECT().GetClient(dockerclient.DockerVersion("1.21")).Times(1).Return(nil, errors.New("Cannot get client"))
	metadata := vclient.StartContainer(ctx, "foo", defaultTestConfig().ContainerStartTimeout)
	assert.NotNil(t, metadata.Error, "Expected error, didn't get one")
	if namederr, ok := metadata.Error.(apierrors.NamedError); ok {
		if namederr.ErrorName() != "CannotGetDockerclientError" {
			t.Fatal("Wrong error name, expected CannotGetDockerclientError but got " + namederr.ErrorName())
		}
	} else {
		t.Fatal("Error was not a named error")
	}
}

func waitForStatsChanClose(statsChan <-chan *dockercontainer.StatsResponse) (closed bool) {
	i := 0
	for range statsChan {
		if i == 10 {
			return false
		}
		i++
		time.Sleep(time.Millisecond * 10)
	}
	return true
}

func TestStatsNormalExit(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()
	mockDockerSDK.EXPECT().ContainerStats(gomock.Any(), gomock.Any(), mobyclient.ContainerStatsOptions{Stream: true}).Return(mobyclient.ContainerStatsResult{
		Body: mockStream{
			data:  []byte(`{"memory_stats":{"Usage":50},"cpu_stats":{"system_cpu_usage":100}}`),
			index: 0,
		},
	}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	stats, _ := client.Stats(ctx, "foo", dockerclient.StatsInactivityTimeout)
	newStat := <-stats
	waitForStats(t, newStat)

	assert.Equal(t, uint64(50), newStat.MemoryStats.Usage)
	assert.Equal(t, uint64(100), newStat.CPUStats.SystemUsage)

	// stop container stats
	cancel()
	// verify stats chan was closed to avoid goroutine leaks
	closed := waitForStatsChanClose(stats)
	assert.True(t, closed, "stats channel was not properly closed")
}

func TestStatsErrorReading(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()
	mockDockerSDK.EXPECT().ContainerStats(gomock.Any(), gomock.Any(), gomock.Any()).Return(mobyclient.ContainerStatsResult{
		Body: mockStream{
			data:  []byte(`{"memory_stats":{"Usage":50},"cpu_stats":{"system_cpu_usage":100}}`),
			index: 0,
		},
	}, errors.New("test error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	statsC, errC := client.Stats(ctx, "foo", dockerclient.StatsInactivityTimeout)

	assert.Error(t, <-errC)
	// verify stats chan was closed to avoid goroutine leaks
	closed := waitForStatsChanClose(statsC)
	assert.True(t, closed, "stats channel was not properly closed")
}

func TestStatsErrorDecoding(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()
	mockDockerSDK.EXPECT().ContainerStats(gomock.Any(), gomock.Any(), mobyclient.ContainerStatsOptions{Stream: true}).Return(mobyclient.ContainerStatsResult{
		Body: mockStream{
			data:  []byte(`stuff`),
			index: 0,
		},
	}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	statsC, errC := client.Stats(ctx, "foo", dockerclient.StatsInactivityTimeout)
	assert.Error(t, <-errC)
	// verify stats chan was closed to avoid goroutine leaks
	closed := waitForStatsChanClose(statsC)
	assert.True(t, closed, "stats channel was not properly closed")
}

func TestStatsClientError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
	sdkFactory.EXPECT().GetDefaultClient().AnyTimes().Return(nil, errors.New("No client"))
	client := &dockerGoClient{
		sdkClientFactory: sdkFactory,
	}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	statsC, errC := client.Stats(ctx, "foo", dockerclient.StatsInactivityTimeout)
	// should get an error from the channel
	err := <-errC
	// stats channel should be closed
	closed := waitForStatsChanClose(statsC)
	assert.True(t, closed, "stats channel was not properly closed")
	assert.Error(t, err)
}

type mockStream struct {
	data  []byte
	index int64
	delay time.Duration
}

func (ms mockStream) Read(data []byte) (n int, err error) {
	time.Sleep(ms.delay)
	if ms.index >= int64(len(ms.data)) {
		err = io.EOF
		return
	}
	n = copy(data, ms.data[ms.index:])
	ms.index += int64(n)
	return
}
func (ms mockStream) Close() error {
	return nil
}
func waitForStats(t *testing.T, stat *dockercontainer.StatsResponse) {
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			t.Error("Timed out waiting for container stats")
		default:
			if stat == nil {
				time.Sleep(time.Second)
				continue
			}
			return
		}
	}
}

func TestStatsInactivityTimeout(t *testing.T) {
	shortInactivityTimeout := 1 * time.Millisecond
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()
	mockDockerSDK.EXPECT().ContainerStats(gomock.Any(), gomock.Any(), mobyclient.ContainerStatsOptions{Stream: true}).Return(mobyclient.ContainerStatsResult{
		Body: mockStream{
			data:  []byte(`{"memory_stats":{"Usage":50},"cpu_stats":{"system_cpu_usage":100}}`),
			index: 0,
			delay: 300 * time.Millisecond,
		},
	}, nil)

	client.inactivityTimeoutHandler = func(reader io.ReadCloser, timeout time.Duration, cancelRequest func(), canceled *uint32) (io.ReadCloser, chan<- struct{}) {
		assert.Equal(t, shortInactivityTimeout, timeout)
		atomic.AddUint32(canceled, 1)
		return reader, make(chan struct{})
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	_, errC := client.Stats(ctx, "foo", shortInactivityTimeout)
	assert.Error(t, <-errC)
}

func TestPollStatsTimeout(t *testing.T) {
	shortTimeout := 1 * time.Millisecond
	mockDockerSDK, _, _, _, _, done := dockerClientSetup(t)
	defer done()
	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ContainerStats(gomock.Any(), gomock.Any(), mobyclient.ContainerStatsOptions{Stream: false}).Do(func(x, y, z interface{}) {
		wait.Wait()
	}).MaxTimes(1).Return(mobyclient.ContainerStatsResult{Body: mockStream{}}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	_, err := getContainerStatsNotStreamed(mockDockerSDK, ctx, "", shortTimeout)
	assert.Error(t, err)
	wait.Done()
}

func TestPollStatsError(t *testing.T) {
	shortTimeout := 1 * time.Millisecond
	mockDockerSDK, _, _, _, _, done := dockerClientSetup(t)
	defer done()
	mockDockerSDK.EXPECT().ContainerStats(gomock.Any(), gomock.Any(), mobyclient.ContainerStatsOptions{Stream: false}).MaxTimes(1).Return(mobyclient.ContainerStatsResult{
		Body: nil},
		errors.New("Container stats error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	_, err := getContainerStatsNotStreamed(mockDockerSDK, ctx, "foo", shortTimeout)
	assert.Error(t, err)
}

func TestStatsInactivityTimeoutNoHit(t *testing.T) {
	longInactivityTimeout := 500 * time.Millisecond
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()
	mockDockerSDK.EXPECT().ContainerStats(gomock.Any(), gomock.Any(), mobyclient.ContainerStatsOptions{Stream: true}).Return(mobyclient.ContainerStatsResult{
		Body: mockStream{
			data:  []byte(`{"memory_stats":{"Usage":50},"cpu_stats":{"system_cpu_usage":100}}`),
			index: 0,
			delay: 300 * time.Millisecond,
		},
	}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	stats, _ := client.Stats(ctx, "foo", longInactivityTimeout)
	newStat := <-stats

	waitForStats(t, newStat)
	assert.Equal(t, uint64(50), newStat.MemoryStats.Usage)
	assert.Equal(t, uint64(100), newStat.CPUStats.SystemUsage)
}

func TestRemoveImageTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ImageRemove(gomock.Any(), "image", mobyclient.ImageRemoveOptions{}).Do(func(x, y, z interface{}) {
		wait.Wait()
	})
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.RemoveImage(ctx, "image", 2*time.Millisecond)
	assert.Error(t, err, "Expected error for remove image timeout")
	wait.Done()
}

func TestRemoveImage(t *testing.T) {
	mockDockerSDK, client, testTime, _, _, done := dockerClientSetup(t)
	defer done()

	testTime.EXPECT().After(gomock.Any()).AnyTimes()
	mockDockerSDK.EXPECT().ImageRemove(gomock.Any(), "image", mobyclient.ImageRemoveOptions{}).Return(mobyclient.ImageRemoveResult{Items: []image.DeleteResponse{}}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.RemoveImage(ctx, "image", dockerclient.RemoveImageTimeout)
	assert.NoError(t, err, "Did not expect error, err: %v", err)
}

func TestLoadImageHappyPath(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().ImageLoad(gomock.Any(), gomock.Any()).Return(ioutil.NopCloser(strings.NewReader("dummy load message")), nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.LoadImage(ctx, nil, dockerclient.LoadImageTimeout)
	assert.NoError(t, err)
}

func TestLoadImageTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().ImageLoad(gomock.Any(), gomock.Any()).Do(func(x, y interface{}, z ...interface{}) {
		wait.Wait()
	}).MaxTimes(1).Return(nil, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.LoadImage(ctx, nil, time.Millisecond)
	assert.Error(t, err)
	_, ok := err.(*DockerTimeoutError)
	assert.True(t, ok)
	wait.Done()
}

// TestECRAuthCache tests the client will use cached docker auth if pulling
// from same registry on ecr with default instance profile
func TestECRAuthCacheWithoutExecutionRole(t *testing.T) {
	mockDockerSDK, client, mockTime, ctrl, ecrClientFactory, done := dockerClientSetup(t)

	defer done()

	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	ecrClient := mock_ecr.NewMockECRClient(ctrl)

	region := "eu-west-1"
	registryID := "1234567890"
	endpointOverride := "my.endpoint"
	authData := &apicontainer.RegistryAuthenticationData{
		Type: "ecr",
		ECRAuthData: &apicontainer.ECRAuthData{
			RegistryID:       registryID,
			Region:           region,
			EndpointOverride: endpointOverride,
		},
	}

	imageEndpoint := "registry.endpoint"
	image := imageEndpoint + "myimage:tag"
	username := "username"
	password := "password"

	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil).Times(1)
	ecrClient.EXPECT().GetAuthorizationToken(registryID).Return(
		&ecr_types.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			ExpiresAt:          aws.Time(time.Now().Add(10 * time.Hour)),
		}, nil).Times(1)
	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		mockReadCloser{
			reader: strings.NewReader(`{"status":"pull complete"}`),
		}, nil).Times(4)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, image, authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry shouldn't expect ecr client call
	metadata = client.PullImage(ctx, image+"2", authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry shouldn't expect ecr client call
	metadata = client.PullImage(ctx, image+"3", authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry shouldn't expect ecr client call
	metadata = client.PullImage(ctx, image+"4", authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

// TestECRAuthCacheForDifferentRegistry tests the client will call ecr client to get docker
// auth for different registry
func TestECRAuthCacheForDifferentRegistry(t *testing.T) {
	mockDockerSDK, client, mockTime, ctrl, ecrClientFactory, done := dockerClientSetup(t)
	defer done()

	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	ecrClient := mock_ecr.NewMockECRClient(ctrl)

	region := "eu-west-1"
	registryID := "1234567890"
	endpointOverride := "my.endpoint"
	authData := &apicontainer.RegistryAuthenticationData{
		Type: "ecr",
		ECRAuthData: &apicontainer.ECRAuthData{
			RegistryID:       registryID,
			Region:           region,
			EndpointOverride: endpointOverride,
		},
	}

	imageEndpoint := "registry.endpoint"
	image := imageEndpoint + "/myimage:tag"
	username := "username"
	password := "password"

	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil).Times(1)
	ecrClient.EXPECT().GetAuthorizationToken(registryID).Return(
		&ecr_types.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			ExpiresAt:          aws.Time(time.Now().Add(10 * time.Hour)),
		}, nil).Times(1)
	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		mockReadCloser{
			reader: strings.NewReader(`{"status":"pull complete"}`),
		}, nil).Times(2)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, image, authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the different registry should expect ECR client call
	authData.ECRAuthData.RegistryID = "another"
	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil).Times(1)
	ecrClient.EXPECT().GetAuthorizationToken("another").Return(
		&ecr_types.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			ExpiresAt:          aws.Time(time.Now().Add(10 * time.Hour)),
		}, nil).Times(1)
	metadata = client.PullImage(ctx, image, authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

// TestECRAuthCacheWithExecutionRole tests the client will use the cached docker auth
// for ecr when pull from the same registry with same execution role
func TestECRAuthCacheWithSameExecutionRole(t *testing.T) {
	mockDockerSDK, client, mockTime, ctrl, ecrClientFactory, done := dockerClientSetup(t)
	defer done()

	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	ecrClient := mock_ecr.NewMockECRClient(ctrl)

	region := "eu-west-1"
	registryID := "1234567890"
	imageEndpoint := "registry.endpoint"
	image := imageEndpoint + "/myimage:tag"
	endpointOverride := "my.endpoint"
	authData := &apicontainer.RegistryAuthenticationData{
		Type: "ecr",
		ECRAuthData: &apicontainer.ECRAuthData{
			RegistryID:       registryID,
			Region:           region,
			EndpointOverride: endpointOverride,
		},
	}
	authData.ECRAuthData.SetPullCredentials(credentials.IAMRoleCredentials{
		RoleArn: "executionRole",
	})

	username := "username"
	password := "password"

	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil).Times(1)
	ecrClient.EXPECT().GetAuthorizationToken(registryID).Return(
		&ecr_types.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			ExpiresAt:          aws.Time(time.Now().Add(10 * time.Hour)),
		}, nil).Times(1)
	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		mockReadCloser{
			reader: strings.NewReader(`{"status":"pull complete"}`),
		}, nil).Times(3)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, image, authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry shouldn't expect ecr client call
	metadata = client.PullImage(ctx, image+"2", authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry shouldn't expect ecr client call
	metadata = client.PullImage(ctx, image+"3", authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

// TestECRAuthCacheWithDifferentExecutionRole tests client will call ecr client to get
// docker auth credentials for different execution role
func TestECRAuthCacheWithDifferentExecutionRole(t *testing.T) {
	mockDockerSDK, client, mockTime, ctrl, ecrClientFactory, done := dockerClientSetup(t)
	defer done()

	mockTime.EXPECT().After(gomock.Any()).AnyTimes()
	ecrClient := mock_ecr.NewMockECRClient(ctrl)

	region := "eu-west-1"
	registryID := "1234567890"
	endpointOverride := "my.endpoint"
	authData := &apicontainer.RegistryAuthenticationData{
		Type: "ecr",
		ECRAuthData: &apicontainer.ECRAuthData{
			RegistryID:       registryID,
			Region:           region,
			EndpointOverride: endpointOverride,
		},
	}
	authData.ECRAuthData.SetPullCredentials(credentials.IAMRoleCredentials{
		RoleArn: "executionRole",
	})

	imageEndpoint := "registry.endpoint"
	image := imageEndpoint + "/myimage:tag"
	username := "username"
	password := "password"

	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil).Times(1)
	ecrClient.EXPECT().GetAuthorizationToken(registryID).Return(
		&ecr_types.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			ExpiresAt:          aws.Time(time.Now().Add(10 * time.Hour)),
		}, nil).Times(1)
	mockDockerSDK.EXPECT().ImagePull(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		mockReadCloser{
			reader: strings.NewReader(`{"status":"pull complete"}`),
		}, nil).Times(2)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	metadata := client.PullImage(ctx, image, authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")

	// Pull from the same registry but with different role
	authData.ECRAuthData.SetPullCredentials(credentials.IAMRoleCredentials{
		RoleArn: "executionRole2",
	})
	ecrClientFactory.EXPECT().GetClient(authData.ECRAuthData).Return(ecrClient, nil).Times(1)
	ecrClient.EXPECT().GetAuthorizationToken(registryID).Return(
		&ecr_types.AuthorizationData{
			ProxyEndpoint:      aws.String("https://" + imageEndpoint),
			AuthorizationToken: aws.String(base64.StdEncoding.EncodeToString([]byte(username + ":" + password))),
			ExpiresAt:          aws.Time(time.Now().Add(10 * time.Hour)),
		}, nil).Times(1)
	metadata = client.PullImage(ctx, image, authData, defaultTestConfig().ImagePullTimeout)
	assert.NoError(t, metadata.Error, "Expected pull to succeed")
}

func TestMetadataFromContainer(t *testing.T) {
	ports := network.PortMap{
		network.MustParsePort("80/tcp"): []network.PortBinding{
			{
				HostIP:   netip.MustParseAddr("0.0.0.0"),
				HostPort: "80",
			},
		},
	}
	// Representation of Volumes in ContainerJSON
	volumes := []dockercontainer.MountPoint{
		{Destination: "/foo",
			Source: "/bar",
		},
	}
	labels := map[string]string{
		"name": "metadata",
	}

	created := time.Now().Format(time.RFC3339)
	started := time.Now().Format(time.RFC3339)
	finished := time.Now().Format(time.RFC3339)

	dockerContainer := dockercontainer.InspectResponse{
		NetworkSettings: &dockercontainer.NetworkSettings{
			Ports: ports,
			Networks: map[string]*network.EndpointSettings{
				"bridge": {IPAddress: netip.MustParseAddr("17.0.0.3")},
			},
		},

		ID:      "1234",
		Created: created,
		State: &dockercontainer.State{
			Running:    true,
			StartedAt:  started,
			FinishedAt: finished,
		},
		HostConfig: &dockercontainer.HostConfig{
			NetworkMode: dockercontainer.NetworkMode("bridge"),
		},

		Config: &dockercontainer.Config{
			Labels: labels,
		},
		Mounts: volumes,
	}

	metadata := MetadataFromContainer(&dockerContainer)
	assert.Equal(t, "1234", metadata.DockerID)
	assert.Equal(t, volumes, metadata.Volumes)
	assert.Equal(t, labels, metadata.Labels)
	assert.Len(t, metadata.PortBindings, 1)
	assert.Equal(t, "bridge", metadata.NetworkMode)
	assert.NotNil(t, metadata.NetworkSettings)
	assert.Equal(t, "17.0.0.3", metadata.NetworkSettings.Networks["bridge"].IPAddress.String())

	// Need to convert both strings to same format to be able to compare. Parse and Format are not inverses.
	createdTimeSDK, _ := time.Parse(time.RFC3339, dockerContainer.Created)
	startedTimeSDK, _ := time.Parse(time.RFC3339, dockerContainer.State.StartedAt)
	finishedTimeSDK, _ := time.Parse(time.RFC3339, dockerContainer.State.FinishedAt)

	createdTime, _ := time.Parse(time.RFC3339, created)
	startedTime, _ := time.Parse(time.RFC3339, started)
	finishedTime, _ := time.Parse(time.RFC3339, finished)

	assert.True(t, createdTime.Equal(createdTimeSDK))
	assert.True(t, startedTime.Equal(startedTimeSDK))
	assert.True(t, finishedTime.Equal(finishedTimeSDK))
}

func TestMetadataFromContainerHealthCheckWithNoLogs(t *testing.T) {

	dockerContainer := &dockercontainer.InspectResponse{

		State: &dockercontainer.State{
			Health: &dockercontainer.Health{Status: "unhealthy"},
		},
	}

	metadata := MetadataFromContainer(dockerContainer)
	assert.Equal(t, apicontainerstatus.ContainerUnhealthy, metadata.Health.Status)
}

func TestCreateVolumeTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().VolumeCreate(gomock.Any(), gomock.Any()).Do(func(ctx context.Context, x interface{}) {
		wait.Wait()
	}).MaxTimes(1).Return(mobyclient.VolumeCreateResult{Volume: volume.Volume{}}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volumeResponse := client.CreateVolume(ctx, "name", "driver", nil, nil, xContainerShortTimeout)
	assert.Error(t, volumeResponse.Error, "expected error for timeout")
	assert.Equal(t, "DockerTimeoutError", volumeResponse.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestCreateVolumeError(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().VolumeCreate(gomock.Any(), gomock.Any()).Return(mobyclient.VolumeCreateResult{Volume: volume.Volume{}}, errors.New("some docker error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volumeResponse := client.CreateVolume(ctx, "name", "driver", nil, nil, dockerclient.CreateVolumeTimeout)
	assert.Equal(t, "CannotCreateVolumeError", volumeResponse.Error.(apierrors.NamedError).ErrorName())
}

func TestCreateVolume(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	volumeName := "volumeName"
	mountPoint := "some/mount/point"
	driver := "driver"
	driverOptions := map[string]string{
		"opt1": "val1",
		"opt2": "val2",
	}
	gomock.InOrder(
		mockDockerSDK.EXPECT().VolumeCreate(gomock.Any(), gomock.Any()).Do(func(ctx context.Context, opts mobyclient.VolumeCreateOptions) {
			assert.Equal(t, opts.Name, volumeName)
			assert.Equal(t, opts.Driver, driver)
			assert.EqualValues(t, opts.DriverOpts, driverOptions)
		}).Return(mobyclient.VolumeCreateResult{Volume: volume.Volume{Name: volumeName, Driver: driver, Mountpoint: mountPoint, Labels: nil}}, nil),
	)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	// This function eventually makes an API call, is that not possible in testing
	volumeResponse := client.CreateVolume(ctx, volumeName, driver, driverOptions, nil, dockerclient.CreateVolumeTimeout)
	assert.NoError(t, volumeResponse.Error)
	assert.Equal(t, volumeResponse.DockerVolume.Name, volumeName)
	assert.Equal(t, volumeResponse.DockerVolume.Driver, driver)
	assert.Equal(t, volumeResponse.DockerVolume.Mountpoint, mountPoint)
	assert.Nil(t, volumeResponse.DockerVolume.Labels)
}

func TestInspectVolumeTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().VolumeInspect(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(ctx context.Context, x, y interface{}) {
		wait.Wait()
	}).MaxTimes(1).Return(mobyclient.VolumeInspectResult{Volume: volume.Volume{}}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volumeResponse := client.InspectVolume(ctx, "name", xContainerShortTimeout)
	assert.Error(t, volumeResponse.Error, "expected error for timeout")
	assert.Equal(t, "DockerTimeoutError", volumeResponse.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestInspectVolumeError(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().VolumeInspect(gomock.Any(), gomock.Any(), gomock.Any()).Return(mobyclient.VolumeInspectResult{Volume: volume.Volume{}}, errors.New("some docker error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volumeResponse := client.InspectVolume(ctx, "name", dockerclient.InspectVolumeTimeout)
	assert.Equal(t, "CannotInspectVolumeError", volumeResponse.Error.(apierrors.NamedError).ErrorName())
}

func TestInspectVolume(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	volumeName := "volumeName"

	volumeOutput := volume.Volume{
		Name:       volumeName,
		Driver:     "driver",
		Mountpoint: "local/mount/point",
		Labels: map[string]string{
			"label1": "val1",
			"label2": "val2",
		},
	}

	mockDockerSDK.EXPECT().VolumeInspect(gomock.Any(), volumeName, gomock.Any()).Return(mobyclient.VolumeInspectResult{Volume: volumeOutput}, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	volumeResponse := client.InspectVolume(ctx, volumeName, dockerclient.InspectVolumeTimeout)
	assert.NoError(t, volumeResponse.Error)
	assert.Equal(t, volumeOutput.Driver, volumeResponse.DockerVolume.Driver)
	assert.Equal(t, volumeOutput.Mountpoint, volumeResponse.DockerVolume.Mountpoint)
	assert.Equal(t, volumeOutput.Labels, volumeResponse.DockerVolume.Labels)
}

func TestRemoveVolumeTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().VolumeRemove(gomock.Any(), "name", mobyclient.VolumeRemoveOptions{Force: false}).Do(func(ctx context.Context,
		x interface{}, y mobyclient.VolumeRemoveOptions) {
		wait.Wait()
	}).MaxTimes(1).Return(mobyclient.VolumeRemoveResult{}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.RemoveVolume(ctx, "name", xContainerShortTimeout)
	assert.Error(t, err, "expected error for timeout")
	assert.Equal(t, "DockerTimeoutError", err.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestRemoveVolumeError(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().VolumeRemove(gomock.Any(), "name", mobyclient.VolumeRemoveOptions{Force: false}).Return(mobyclient.VolumeRemoveResult{}, errors.New("some docker error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.RemoveVolume(ctx, "name", dockerclient.RemoveVolumeTimeout)
	assert.Equal(t, "CannotRemoveVolumeError", err.(apierrors.NamedError).ErrorName())
	assert.NotNil(t, err.Error(), "Nested error cannot be nil")
}

func TestRemoveVolume(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	volumeName := "volumeName"

	mockDockerSDK.EXPECT().VolumeRemove(gomock.Any(), volumeName, mobyclient.VolumeRemoveOptions{Force: false}).Return(mobyclient.VolumeRemoveResult{}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	err := client.RemoveVolume(ctx, volumeName, dockerclient.RemoveVolumeTimeout)
	assert.NoError(t, err)
}

func TestListPluginsTimeout(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	wait := &sync.WaitGroup{}
	wait.Add(1)
	mockDockerSDK.EXPECT().PluginList(gomock.Any(), mobyclient.PluginListOptions{Filters: mobyclient.Filters{}}).Do(func(x, y interface{}) {
		wait.Wait()
	}).MaxTimes(1).Return(mobyclient.PluginListResult{}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListPlugins(ctx, xContainerShortTimeout, mobyclient.Filters{})
	assert.Error(t, response.Error, "expected error for timeout")
	assert.Equal(t, "DockerTimeoutError", response.Error.(apierrors.NamedError).ErrorName())
	wait.Done()
}

func TestListPluginsError(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	mockDockerSDK.EXPECT().PluginList(gomock.Any(), mobyclient.PluginListOptions{Filters: mobyclient.Filters{}}).Return(mobyclient.PluginListResult{}, errors.New("some docker error"))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListPlugins(ctx, dockerclient.ListPluginsTimeout, mobyclient.Filters{})
	assert.Equal(t, "CannotListPluginsError", response.Error.(apierrors.NamedError).ErrorName())
}

func TestListPlugins(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	pluginID := "id"
	pluginName := "name"
	plugin := mobyplugin.Plugin{
		ID:      pluginID,
		Name:    pluginName,
		Enabled: true,
	}

	mockDockerSDK.EXPECT().PluginList(gomock.Any(), mobyclient.PluginListOptions{Filters: mobyclient.Filters{}}).Return(mobyclient.PluginListResult{Items: []mobyplugin.Plugin{plugin}}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	response := client.ListPlugins(ctx, dockerclient.ListPluginsTimeout, mobyclient.Filters{})
	assert.NoError(t, response.Error)
	assert.Equal(t, plugin, response.Plugins[0])
}

func TestListPluginsWithFilter(t *testing.T) {
	mockDockerSDK, client, _, _, _, done := dockerClientSetup(t)
	defer done()

	plugins := []*mobyplugin.Plugin{
		&mobyplugin.Plugin{
			ID:      "id1",
			Name:    "name1",
			Enabled: false,
		},
		&mobyplugin.Plugin{
			ID:      "id2",
			Name:    "name2",
			Enabled: true,
			Config: mobyplugin.Config{
				Description: "A sample volume plugin for Docker",
				Interface: mobyplugin.Interface{
					Types: []mobyplugin.CapabilityID{
						{Capability: "docker.volumedriver/1.0"},
					},
					Socket: "plugins.sock",
				},
			},
		},
		&mobyplugin.Plugin{
			ID:      "id3",
			Name:    "name3",
			Enabled: true,
			Config: mobyplugin.Config{
				Description: "A sample network plugin for Docker",
				Interface: mobyplugin.Interface{
					Types: []mobyplugin.CapabilityID{
						{Capability: "docker.networkdriver/1.0"},
					},
					Socket: "plugins.sock",
				},
			},
		},
	}

	filterList := mobyclient.Filters{}.Add("enabled", "true")
	filterList.Add("capability", VolumeDriverType)
	mockDockerSDK.EXPECT().PluginList(gomock.Any(), mobyclient.PluginListOptions{Filters: filterList}).Return(mobyclient.PluginListResult{Items: []mobyplugin.Plugin{*plugins[1]}}, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	pluginNames, error := client.ListPluginsWithFilters(ctx, true, []string{VolumeDriverType}, dockerclient.ListPluginsTimeout)
	assert.NoError(t, error)
	assert.Equal(t, 1, len(pluginNames))
	assert.Equal(t, "name2", pluginNames[0])
}

func TestTagImage(t *testing.T) {
	someError := errors.New("some error")
	tcs := []struct {
		name                      string
		source                    string
		target                    string
		setSDKFactoryExpectations func(f *mock_sdkclientfactory.MockFactory, ctrl *gomock.Controller)
		ctx                       context.Context
		expectedError             string
		expectedSleeps            int
	}{
		{
			name: "failed to get sdkclient",
			setSDKFactoryExpectations: func(f *mock_sdkclientfactory.MockFactory, ctrl *gomock.Controller) {
				f.EXPECT().GetDefaultClient().Return(nil, someError)
			},
			expectedError:  someError.Error(),
			expectedSleeps: 0,
		},
		{
			name:   "all attempts exhausted",
			source: "source",
			target: "target",
			setSDKFactoryExpectations: func(f *mock_sdkclientfactory.MockFactory, ctrl *gomock.Controller) {
				client := mock_sdkclient.NewMockClient(ctrl)
				client.EXPECT().
					ImageTag(gomock.Any(), gomock.Any()).
					Times(tagImageRetryAttempts).
					Return(mobyclient.ImageTagResult{}, someError)
				f.EXPECT().GetDefaultClient().Return(client, nil)
			},
			ctx:            context.Background(),
			expectedError:  "failed to tag image 'source' as 'target': " + someError.Error(),
			expectedSleeps: tagImageRetryAttempts - 1,
		},
		{
			name:   "second attempt worked",
			source: "source",
			target: "target",
			setSDKFactoryExpectations: func(f *mock_sdkclientfactory.MockFactory, ctrl *gomock.Controller) {
				client := mock_sdkclient.NewMockClient(ctrl)
				client.EXPECT().ImageTag(gomock.Any(), gomock.Any()).Return(mobyclient.ImageTagResult{}, someError)
				client.EXPECT().ImageTag(gomock.Any(), gomock.Any()).Return(mobyclient.ImageTagResult{}, nil)
				f.EXPECT().GetDefaultClient().Return(client, nil)
			},
			ctx:            context.Background(),
			expectedSleeps: 1,
		},
		{
			name: "canceled context",
			setSDKFactoryExpectations: func(f *mock_sdkclientfactory.MockFactory, ctrl *gomock.Controller) {
				client := mock_sdkclient.NewMockClient(ctrl)
				f.EXPECT().GetDefaultClient().Return(client, nil)
			},
			ctx: func() context.Context {
				c, cancel := context.WithCancel(context.Background())
				cancel()
				return c
			}(),
			expectedError: "context canceled",
		},
		{
			name: "deadline exceeded",
			setSDKFactoryExpectations: func(f *mock_sdkclientfactory.MockFactory, ctrl *gomock.Controller) {
				client := mock_sdkclient.NewMockClient(ctrl)
				f.EXPECT().GetDefaultClient().Return(client, nil)
			},
			ctx: func() context.Context {
				c, cancel := context.WithTimeout(context.Background(), 0)
				<-c.Done() // wait for deadline to be exceeded
				cancel()
				return c
			}(),
			expectedError: "context deadline exceeded",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Set up mocks
			mockDockerSDK := mock_sdkclient.NewMockClient(ctrl)
			mockDockerSDK.EXPECT().Ping(gomock.Any(), gomock.Any()).Return(mobyclient.PingResult{}, nil)
			sdkFactory := mock_sdkclientfactory.NewMockFactory(ctrl)
			sdkFactory.EXPECT().GetDefaultClient().Return(mockDockerSDK, nil)

			// Set up docker client for testing
			client, err := NewDockerGoClient(sdkFactory, defaultTestConfig(), context.Background())
			require.NoError(t, err)
			// Make retries fast
			client.(*dockerGoClient).imageTagBackoff = retry.NewConstantBackoff(0)

			if tc.setSDKFactoryExpectations != nil {
				tc.setSDKFactoryExpectations(sdkFactory, ctrl)
			}

			err = client.TagImage(tc.ctx, tc.source, tc.target)
			if tc.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.expectedError)
			}
		})
	}
}
