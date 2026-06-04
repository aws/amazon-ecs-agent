//go:build linux && unit
// +build linux,unit

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

package config

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	mock_ec2 "github.com/aws/amazon-ecs-agent/ecs-agent/ec2/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// httpStatusError implements the HTTPStatusCode() interface for testing.
type httpStatusError struct {
	statusCode int
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("http response error StatusCode: %d, request to EC2 IMDS failed", e.statusCode)
}

func (e *httpStatusError) HTTPStatusCode() int {
	return e.statusCode
}

func TestCheckCloudInitFailure(t *testing.T) {
	tests := []struct {
		name        string
		fileContent *string
		setupMock   func(*mock_ec2.MockEC2MetadataClient)
		expectError bool
	}{
		{
			name:        "file missing",
			fileContent: nil,
		},
		{
			name:        "invalid JSON",
			fileContent: aws.String("not valid json{{{"),
		},
		{
			name:        "datasource is DataSourceEc2Local",
			fileContent: aws.String(`{"v1": {"datasource": "DataSourceEc2Local"}}`),
		},
		{
			name:        "datasource is DataSourceEc2",
			fileContent: aws.String(`{"v1": {"datasource": "DataSourceEc2"}}`),
		},
		{
			name:        "DataSourceNone but GetUserData returns error",
			fileContent: aws.String(`{"v1": {"datasource": "DataSourceNone"}}`),
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetUserData().Return("", errors.New("IMDS error"))
			},
		},
		{
			name:        "DataSourceNone but GetUserData returns 404 (no userdata configured)",
			fileContent: aws.String(`{"v1": {"datasource": "DataSourceNone"}}`),
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetUserData().Return("", &httpStatusError{statusCode: http.StatusNotFound})
			},
		},
		{
			name:        "DataSourceNone but userdata is empty",
			fileContent: aws.String(`{"v1": {"datasource": "DataSourceNone"}}`),
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetUserData().Return("", nil)
			},
		},
		{
			name:        "DataSourceNone with non-empty userdata - detection fires",
			fileContent: aws.String(`{"v1": {"datasource": "DataSourceNone"}}`),
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetUserData().Return("#!/bin/bash\necho ECS_CLUSTER=my-cluster >> /etc/ecs/ecs.config", nil)
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockEC2 := mock_ec2.NewMockEC2MetadataClient(ctrl)

			var resultFile string
			if tc.fileContent != nil {
				tmpDir := t.TempDir()
				resultFile = filepath.Join(tmpDir, "result.json")
				require.NoError(t, os.WriteFile(resultFile, []byte(*tc.fileContent), 0644))
			} else {
				resultFile = "/nonexistent/path/result.json"
			}

			if tc.setupMock != nil {
				tc.setupMock(mockEC2)
			}

			err := CheckCloudInitFailure(mockEC2, resultFile)
			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "DataSourceNone")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
