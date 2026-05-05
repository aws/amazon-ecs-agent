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

//go:build unit

package imds

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	mock_ec2 "github.com/aws/amazon-ecs-agent/ecs-agent/ec2/mocks"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	testTaskID1 = "0ee1b6f1feef4ff2bacdf2c99732d506"
	testTaskID2 = "aabbccdd11223344aabbccdd11223344"
	testRoleARN = "arn:aws:iam::123456789012:role/TestRole"
)

// testCredentialJSON returns a mock IMDS credential file JSON.
func testCredentialJSON(accessKeyID string) string {
	return fmt.Sprintf(`{
		"AccessKeyId": "%s",
		"SecretAccessKey": "secret",
		"Token": "token",
		"Expiration": "2026-04-28T00:00:00Z"
	}`, accessKeyID)
}

// testInfoJSONWithTimestamp returns a mock IMDS info file JSON
// with the given LastUpdated value.
func testInfoJSONWithTimestamp(
	lastUpdated string, entries map[string]string,
) string {
	entriesJSON := ""
	for key, roleARN := range entries {
		if entriesJSON != "" {
			entriesJSON += ","
		}
		entriesJSON += fmt.Sprintf(
			`"%s": {"RoleARN": "%s"}`, key, roleARN,
		)
	}
	return fmt.Sprintf(
		`{"LastUpdated": "%s", "TaskCredentials": {%s}}`,
		lastUpdated, entriesJSON,
	)
}

// testInfoJSON returns a mock IMDS info file JSON with a default timestamp.
func testInfoJSON(entries map[string]string) string {
	return testInfoJSONWithTimestamp("2026-04-28T00:00:00Z", entries)
}

// testCred is a helper func that returns a TaskCredential with the given fields.
func testCred(taskID, roleType, roleArn, accessKeyID string) TaskCredential {
	return TaskCredential{
		TaskID:          taskID,
		RoleType:        roleType,
		RoleArn:         roleArn,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Expiration:      "2026-04-28T00:00:00Z",
	}
}

func TestDiscoverNamespaces(t *testing.T) {
	tests := []struct {
		name      string
		imdsResp  string
		imdsErr   error
		expected  []string
		expectErr bool
	}{
		{
			name:     "no iam-ecs namespaces",
			imdsResp: "ami-id\ninstance-id\niam/",
			expected: nil,
		},
		{
			name:     "single namespace",
			imdsResp: "ami-id\niam-ecs-1\niam/",
			expected: []string{"iam-ecs-1"},
		},
		{
			name:     "multiple namespaces",
			imdsResp: "iam-ecs-1\niam-ecs-2\niam-ecs-10\niam/",
			expected: []string{"iam-ecs-1", "iam-ecs-2", "iam-ecs-10"},
		},
		{
			name:     "trailing slash on namespace",
			imdsResp: "iam-ecs-1/\niam/",
			expected: []string{"iam-ecs-1"},
		},
		{
			name:     "trailing newline",
			imdsResp: "iam-ecs-1\n",
			expected: []string{"iam-ecs-1"},
		},
		{
			name:     "empty response",
			imdsResp: "",
			expected: nil,
		},
		{
			name:      "IMDS error",
			imdsErr:   errors.New("connection refused"),
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mock := mock_ec2.NewMockEC2MetadataClient(ctrl)
			mock.EXPECT().GetMetadata("").Return(tc.imdsResp, tc.imdsErr)

			s := NewScanner(mock).(*scanner)
			namespaces, err := s.discoverNamespaces(context.Background())

			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, namespaces)
			}
		})
	}
}

func TestScanNamespace(t *testing.T) {
	key1 := testTaskID1 + "-" + credentials.ApplicationRoleType
	key2 := testTaskID2 + "-" + credentials.ExecutionRoleType

	tests := []struct {
		name          string
		setupMock     func(*mock_ec2.MockEC2MetadataClient)
		lastUpdated   map[string]time.Time
		expectedCreds []TaskCredential
		expectErr     bool
		// expectLastUpdatedCached is a pointer to distinguish "don't check" (nil)
		// from "assert not cached" (false).
		expectLastUpdatedCached *bool
	}{
		{
			name: "single credential",
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetMetadata("iam-ecs-1/info").Return(
					testInfoJSON(map[string]string{key1: testRoleARN}), nil)
				m.EXPECT().GetMetadata("iam-ecs-1/security-credentials/"+key1).Return(
					testCredentialJSON("AKID1"), nil)
			},
			expectedCreds: []TaskCredential{
				testCred(testTaskID1, credentials.ApplicationRoleType, testRoleARN, "AKID1"),
			},
			expectLastUpdatedCached: aws.Bool(true),
		},
		{
			name: "multiple credentials",
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetMetadata("iam-ecs-1/info").Return(
					testInfoJSON(map[string]string{
						key1: testRoleARN,
						key2: testRoleARN,
					}), nil)
				m.EXPECT().GetMetadata("iam-ecs-1/security-credentials/"+key1).Return(
					testCredentialJSON("AKID1"), nil)
				m.EXPECT().GetMetadata("iam-ecs-1/security-credentials/"+key2).Return(
					testCredentialJSON("AKID2"), nil)
			},
			expectedCreds: []TaskCredential{
				testCred(testTaskID1, credentials.ApplicationRoleType, testRoleARN, "AKID1"),
				testCred(testTaskID2, credentials.ExecutionRoleType, testRoleARN, "AKID2"),
			},
		},
		{
			name: "info file fetch fails",
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetMetadata("iam-ecs-1/info").Return(
					"", errors.New("not found"))
			},
			expectErr: true,
		},
		{
			name: "info file invalid JSON",
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetMetadata("iam-ecs-1/info").Return(
					"not json", nil)
			},
			expectErr: true,
		},
		{
			name: "invalid LastUpdated timestamp",
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetMetadata("iam-ecs-1/info").Return(
					testInfoJSONWithTimestamp("not-a-timestamp",
						map[string]string{key1: testRoleARN}), nil)
			},
			expectErr: true,
		},
		{
			name: "fetch for one credential fails, other succeeds",
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetMetadata("iam-ecs-1/info").Return(
					testInfoJSON(map[string]string{
						key1: testRoleARN,
						key2: testRoleARN,
					}), nil)
				m.EXPECT().GetMetadata("iam-ecs-1/security-credentials/"+key1).Return(
					"", errors.New("timeout"))
				m.EXPECT().GetMetadata("iam-ecs-1/security-credentials/"+key2).Return(
					testCredentialJSON("AKID2"), nil)
			},
			expectedCreds: []TaskCredential{
				testCred(testTaskID2, credentials.ExecutionRoleType, testRoleARN, "AKID2"),
			},
			expectLastUpdatedCached: aws.Bool(false),
		},
		{
			name: "credential response invalid JSON",
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetMetadata("iam-ecs-1/info").Return(
					testInfoJSON(map[string]string{key1: testRoleARN}), nil)
				m.EXPECT().GetMetadata("iam-ecs-1/security-credentials/"+key1).Return(
					"not json", nil)
			},
		},
		{
			name: "invalid credential key format",
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetMetadata("iam-ecs-1/info").Return(
					testInfoJSON(map[string]string{
						"nodelimiterkey": testRoleARN,
					}), nil)
			},
		},
		{
			name: "unchanged LastUpdated skips credential fetches",
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetMetadata("iam-ecs-1/info").Return(
					testInfoJSON(map[string]string{key1: testRoleARN}), nil)
			},
			lastUpdated: map[string]time.Time{
				"iam-ecs-1": time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "changed LastUpdated re-fetches credentials",
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetMetadata("iam-ecs-1/info").Return(
					testInfoJSONWithTimestamp("2026-04-28T01:00:00Z",
						map[string]string{key1: testRoleARN}), nil)
				m.EXPECT().GetMetadata("iam-ecs-1/security-credentials/"+key1).Return(
					testCredentialJSON("AKID_NEW"), nil)
			},
			lastUpdated: map[string]time.Time{
				"iam-ecs-1": time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC),
			},
			expectedCreds: []TaskCredential{
				testCred(testTaskID1, credentials.ApplicationRoleType, testRoleARN, "AKID_NEW"),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mock := mock_ec2.NewMockEC2MetadataClient(ctrl)
			tc.setupMock(mock)

			s := NewScanner(mock).(*scanner)
			if tc.lastUpdated != nil {
				s.lastUpdated = tc.lastUpdated
			}

			creds, err := s.scanNamespace(context.Background(), "iam-ecs-1")

			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, creds)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tc.expectedCreds, creds)
			}
			if tc.expectLastUpdatedCached != nil {
				if *tc.expectLastUpdatedCached {
					assert.Contains(t, s.lastUpdated, "iam-ecs-1")
				} else {
					assert.NotContains(t, s.lastUpdated, "iam-ecs-1")
				}
			}
		})
	}
}
func TestParseCredentialKey(t *testing.T) {
	tests := []struct {
		name             string
		key              string
		expectedTaskID   string
		expectedRoleType string
		expectError      bool
	}{
		{
			name:             "task application key",
			key:              testTaskID1 + "-TaskApplication",
			expectedTaskID:   testTaskID1,
			expectedRoleType: "TaskApplication",
		},
		{
			name:             "task execution key",
			key:              testTaskID1 + "-TaskExecution",
			expectedTaskID:   testTaskID1,
			expectedRoleType: "TaskExecution",
		},
		{
			name:        "no delimiter",
			key:         testTaskID1,
			expectError: true,
		},
		{
			name:        "empty string",
			key:         "",
			expectError: true,
		},
		{
			name:        "delimiter only",
			key:         "-",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			taskID, roleType, err := parseCredentialKey(tc.key)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedTaskID, taskID)
				assert.Equal(t, tc.expectedRoleType, roleType)
			}
		})
	}
}

func TestScan(t *testing.T) {
	tests := []struct {
		name          string
		ctx           context.Context
		setupMock     func(*mock_ec2.MockEC2MetadataClient)
		expectedCreds []TaskCredential
		expectErr     bool
	}{
		{
			name: "no namespaces",
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetMetadata("").Return("ami-id\niam/", nil)
			},
		},
		{
			name: "end to end with multiple namespaces",
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				key1 := testTaskID1 + "-" + credentials.ApplicationRoleType
				key2 := testTaskID2 + "-" + credentials.ExecutionRoleType
				m.EXPECT().GetMetadata("").Return("iam-ecs-1\niam-ecs-2", nil)
				m.EXPECT().GetMetadata("iam-ecs-1/info").Return(
					testInfoJSON(map[string]string{key1: testRoleARN}), nil)
				m.EXPECT().GetMetadata("iam-ecs-1/security-credentials/"+key1).Return(
					testCredentialJSON("AKID1"), nil)
				m.EXPECT().GetMetadata("iam-ecs-2/info").Return(
					testInfoJSON(map[string]string{key2: testRoleARN}), nil)
				m.EXPECT().GetMetadata("iam-ecs-2/security-credentials/"+key2).Return(
					testCredentialJSON("AKID2"), nil)
			},
			expectedCreds: []TaskCredential{
				testCred(testTaskID1, credentials.ApplicationRoleType, testRoleARN, "AKID1"),
				testCred(testTaskID2, credentials.ExecutionRoleType, testRoleARN, "AKID2"),
			},
		},
		{
			name: "namespace discovery fails",
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetMetadata("").Return("", errors.New("unreachable"))
			},
			expectErr: true,
		},
		{
			name: "all namespaces fail",
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {
				m.EXPECT().GetMetadata("").Return("iam-ecs-1\niam-ecs-2", nil)
				m.EXPECT().GetMetadata("iam-ecs-1/info").Return(
					"", errors.New("timeout"))
				m.EXPECT().GetMetadata("iam-ecs-2/info").Return(
					"", errors.New("timeout"))
			},
			expectErr: true,
		},
		{
			name: "cancelled context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			setupMock: func(m *mock_ec2.MockEC2MetadataClient) {},
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mock := mock_ec2.NewMockEC2MetadataClient(ctrl)
			tc.setupMock(mock)

			ctx := tc.ctx
			if ctx == nil {
				ctx = context.Background()
			}

			s := NewScanner(mock)
			creds, err := s.Scan(ctx)

			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, creds)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tc.expectedCreds, creds)
			}
		})
	}
}
