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

package fsx

import (
	"errors"
	"testing"

	mock_fsx "github.com/aws/amazon-ecs-agent/agent/fsx/mocks"
	"github.com/golang/mock/gomock"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/fsx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fileSystemId = "fs-12345678"
	dnsName      = "test"
)

func TestGetFileSystemDNSNames(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFSxClient := mock_fsx.NewMockFSxClient(ctrl)
	mockFSxClient.EXPECT().DescribeFileSystems(gomock.Any()).Return(&fsx.DescribeFileSystemsOutput{
		FileSystems: []*fsx.FileSystem{
			{
				FileSystemId: aws.String(fileSystemId),
				DNSName:      aws.String(dnsName),
			},
		},
	}, nil)

	out, err := GetFileSystemDNSNames([]string{fileSystemId}, mockFSxClient)
	require.NoError(t, err)
	assert.Equal(t, "test", out[fileSystemId])
}

func TestGetFileSystemDNSNamesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFSxClient := mock_fsx.NewMockFSxClient(ctrl)
	mockFSxClient.EXPECT().DescribeFileSystems(gomock.Any()).Return(nil, errors.New("test error"))

	_, err := GetFileSystemDNSNames([]string{fileSystemId}, mockFSxClient)
	assert.Error(t, err)
}
