//go:build unit
// +build unit

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//    http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package s3

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	s3sdk "github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mock_s3 "github.com/aws/amazon-ecs-agent/agent/s3/mocks"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"
)

const (
	testBucket  = "testbucket"
	testKey     = "testkey"
	testTimeout = 1 * time.Second
)

type mockGetObjectValue struct {
	s3iface.S3API
	Resp s3sdk.GetObjectOutput
}

func (m mockGetObjectValue) GetObject(input *s3sdk.GetObjectInput) (*s3sdk.GetObjectOutput, error) {
	return &m.Resp, nil
}

func TestDownloadFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFile := mock_oswrapper.NewMockFile()
	mockS3Client := mock_s3.NewMockS3Client(ctrl)

	mockS3Client.EXPECT().DownloadWithContext(gomock.Any(), mockFile, gomock.Any()).Do(func(ctx aws.Context,
		w io.WriterAt, input *s3sdk.GetObjectInput) {
		assert.Equal(t, testBucket, aws.StringValue(input.Bucket))
		assert.Equal(t, testKey, aws.StringValue(input.Key))
	})

	err := DownloadFile(testBucket, testKey, testTimeout, mockFile, mockS3Client)
	assert.NoError(t, err)
}

func TestDownloadFileError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockS3Client := mock_s3.NewMockS3Client(ctrl)
	mockFile := mock_oswrapper.NewMockFile()

	mockS3Client.EXPECT().DownloadWithContext(gomock.Any(), mockFile, gomock.Any()).Return(int64(0), errors.New("test error"))

	err := DownloadFile(testBucket, testKey, testTimeout, mockFile, mockS3Client)
	assert.Error(t, err)
}

func TestParseS3ARN(t *testing.T) {
	bucket, key, err := ParseS3ARN("arn:aws:s3:::bucket/key")
	assert.NoError(t, err)
	assert.Equal(t, "bucket", bucket)
	assert.Equal(t, "key", key)
}

func TestParseS3ARNInvalid(t *testing.T) {
	_, _, err := ParseS3ARN("arn:aws:xxx:::xxx")
	assert.Error(t, err)
}

func TestGetObject(t *testing.T) {
	expectedValue := "testdata"
	s3Client := createS3Interface(expectedValue)
	actualValue, err := GetObject(testBucket, testKey, s3Client)
	assert.NoError(t, err)
	assert.Equal(t, actualValue, expectedValue)
}

func createS3Interface(value string) mockGetObjectValue {
	return mockGetObjectValue{
		Resp: s3sdk.GetObjectOutput{
			Body: io.NopCloser(strings.NewReader(value)),
		},
	}
}
