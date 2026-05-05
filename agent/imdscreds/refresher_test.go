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

package imdscreds

import (
	"context"
	"errors"
	"testing"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials/imds"
	mock_imds "github.com/aws/amazon-ecs-agent/ecs-agent/credentials/imds/mocks"
	mock_credentials "github.com/aws/amazon-ecs-agent/ecs-agent/credentials/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	testTaskARN1 = "arn:aws:ecs:us-west-2:123456789012:task/" +
		"test-cluster/0ee1b6f1feef4ff2bacdf2c99732d506"
	testTaskARN2 = "arn:aws:ecs:us-west-2:123456789012:task/" +
		"test-cluster/aabbccdd11223344aabbccdd11223344"
	testTaskID1  = "0ee1b6f1feef4ff2bacdf2c99732d506"
	testTaskID2  = "aabbccdd11223344aabbccdd11223344"
	testCredID1  = "cred-id-task-role-1"
	testCredID2  = "cred-id-exec-role-1"
	testCredID3  = "cred-id-task-role-2"
	testRoleARN1 = "arn:aws:iam::123456789012:role/TaskRole"
	testRoleARN2 = "arn:aws:iam::123456789012:role/ExecRole"
	testRoleARN3 = "arn:aws:iam::123456789012:role/TaskRole2"
)

// newTestTask creates a task with the given ARN, status, and credential IDs
// for use in unit tests.
func newTestTask(
	arn string, taskStatus status.TaskStatus,
	credID, execCredID string,
) *apitask.Task {
	task := &apitask.Task{Arn: arn}
	task.SetKnownStatus(taskStatus)
	if credID != "" {
		task.SetCredentialsID(credID)
	}
	if execCredID != "" {
		task.SetExecutionRoleCredentialsID(execCredID)
	}
	return task
}

func TestRefresh(t *testing.T) {
	tests := []struct {
		name            string
		listTasksErr    error
		tasks           []*apitask.Task
		scanResult      []imds.TaskCredential
		scanErr         error
		expectedUpserts []*credentials.TaskIAMRoleCredentials
	}{
		{
			name:  "no tasks skips scan",
			tasks: []*apitask.Task{},
		},
		{
			name:         "list tasks error",
			listTasksErr: errors.New("db error"),
		},
		{
			name: "scan error",
			tasks: []*apitask.Task{
				newTestTask(testTaskARN1, status.TaskRunning, testCredID1, ""),
			},
			scanErr: errors.New("imds unreachable"),
		},
		{
			name: "all terminal tasks skips scan",
			tasks: []*apitask.Task{
				newTestTask(testTaskARN1, status.TaskStopped, testCredID1, ""),
			},
		},
		{
			name: "upserts task role credential",
			tasks: []*apitask.Task{
				newTestTask(testTaskARN1, status.TaskRunning, testCredID1, testCredID2),
			},
			scanResult: []imds.TaskCredential{
				{
					TaskID:          testTaskID1,
					RoleType:        credentials.ApplicationRoleType,
					RoleArn:         testRoleARN1,
					AccessKeyID:     "AKID_NEW",
					SecretAccessKey: "secret_new",
					SessionToken:    "token_new",
					Expiration:      "2026-05-05T12:00:00Z",
				},
			},
			expectedUpserts: []*credentials.TaskIAMRoleCredentials{
				{
					ARN: testTaskARN1,
					IAMRoleCredentials: credentials.IAMRoleCredentials{
						CredentialsID:   testCredID1,
						RoleArn:         testRoleARN1,
						AccessKeyID:     "AKID_NEW",
						SecretAccessKey: "secret_new",
						SessionToken:    "token_new",
						Expiration:      "2026-05-05T12:00:00Z",
						RoleType:        credentials.ApplicationRoleType,
					},
				},
			},
		},
		{
			name: "upserts execution role credential",
			tasks: []*apitask.Task{
				newTestTask(testTaskARN1, status.TaskRunning, testCredID1, testCredID2),
			},
			scanResult: []imds.TaskCredential{
				{
					TaskID:          testTaskID1,
					RoleType:        credentials.ExecutionRoleType,
					RoleArn:         testRoleARN2,
					AccessKeyID:     "AKID_EXEC",
					SecretAccessKey: "secret_exec",
					SessionToken:    "token_exec",
					Expiration:      "2026-05-05T12:00:00Z",
				},
			},
			expectedUpserts: []*credentials.TaskIAMRoleCredentials{
				{
					ARN: testTaskARN1,
					IAMRoleCredentials: credentials.IAMRoleCredentials{
						CredentialsID:   testCredID2,
						RoleArn:         testRoleARN2,
						AccessKeyID:     "AKID_EXEC",
						SecretAccessKey: "secret_exec",
						SessionToken:    "token_exec",
						Expiration:      "2026-05-05T12:00:00Z",
						RoleType:        credentials.ExecutionRoleType,
					},
				},
			},
		},
		{
			name: "skips unknown task ID",
			tasks: []*apitask.Task{
				newTestTask(testTaskARN1, status.TaskRunning, testCredID1, ""),
			},
			scanResult: []imds.TaskCredential{
				{
					TaskID:   "unknown00000000000000000000000000",
					RoleType: credentials.ApplicationRoleType,
					RoleArn:  testRoleARN1,
				},
			},
		},
		{
			name: "skips credential with no credentials ID on task",
			tasks: []*apitask.Task{
				newTestTask(testTaskARN1, status.TaskRunning, "", ""),
			},
			scanResult: []imds.TaskCredential{
				{
					TaskID:   testTaskID1,
					RoleType: credentials.ApplicationRoleType,
					RoleArn:  testRoleARN1,
				},
			},
		},
		{
			name: "multiple tasks multiple credentials",
			tasks: []*apitask.Task{
				newTestTask(testTaskARN1, status.TaskRunning, testCredID1, ""),
				newTestTask(testTaskARN2, status.TaskRunning, testCredID3, ""),
			},
			scanResult: []imds.TaskCredential{
				{
					TaskID:          testTaskID1,
					RoleType:        credentials.ApplicationRoleType,
					RoleArn:         testRoleARN1,
					AccessKeyID:     "AKID1",
					SecretAccessKey: "secret1",
					SessionToken:    "token1",
					Expiration:      "2026-05-05T12:00:00Z",
				},
				{
					TaskID:          testTaskID2,
					RoleType:        credentials.ApplicationRoleType,
					RoleArn:         testRoleARN3,
					AccessKeyID:     "AKID2",
					SecretAccessKey: "secret2",
					SessionToken:    "token2",
					Expiration:      "2026-05-05T12:00:00Z",
				},
			},
			expectedUpserts: []*credentials.TaskIAMRoleCredentials{
				{
					ARN: testTaskARN1,
					IAMRoleCredentials: credentials.IAMRoleCredentials{
						CredentialsID:   testCredID1,
						RoleArn:         testRoleARN1,
						AccessKeyID:     "AKID1",
						SecretAccessKey: "secret1",
						SessionToken:    "token1",
						Expiration:      "2026-05-05T12:00:00Z",
						RoleType:        credentials.ApplicationRoleType,
					},
				},
				{
					ARN: testTaskARN2,
					IAMRoleCredentials: credentials.IAMRoleCredentials{
						CredentialsID:   testCredID3,
						RoleArn:         testRoleARN3,
						AccessKeyID:     "AKID2",
						SecretAccessKey: "secret2",
						SessionToken:    "token2",
						Expiration:      "2026-05-05T12:00:00Z",
						RoleType:        credentials.ApplicationRoleType,
					},
				},
			},
		},
		{
			name: "same role ARN for task and execution role",
			tasks: []*apitask.Task{
				newTestTask(testTaskARN1, status.TaskRunning, testCredID1, testCredID2),
			},
			scanResult: []imds.TaskCredential{
				{
					TaskID:          testTaskID1,
					RoleType:        credentials.ApplicationRoleType,
					RoleArn:         testRoleARN1,
					AccessKeyID:     "AKID_TASK",
					SecretAccessKey: "secret_task",
					SessionToken:    "token_task",
					Expiration:      "2026-05-05T12:00:00Z",
				},
				{
					TaskID:          testTaskID1,
					RoleType:        credentials.ExecutionRoleType,
					RoleArn:         testRoleARN1,
					AccessKeyID:     "AKID_EXEC",
					SecretAccessKey: "secret_exec",
					SessionToken:    "token_exec",
					Expiration:      "2026-05-05T12:00:00Z",
				},
			},
			expectedUpserts: []*credentials.TaskIAMRoleCredentials{
				{
					ARN: testTaskARN1,
					IAMRoleCredentials: credentials.IAMRoleCredentials{
						CredentialsID:   testCredID1,
						RoleArn:         testRoleARN1,
						AccessKeyID:     "AKID_TASK",
						SecretAccessKey: "secret_task",
						SessionToken:    "token_task",
						Expiration:      "2026-05-05T12:00:00Z",
						RoleType:        credentials.ApplicationRoleType,
					},
				},
				{
					ARN: testTaskARN1,
					IAMRoleCredentials: credentials.IAMRoleCredentials{
						CredentialsID:   testCredID2,
						RoleArn:         testRoleARN1,
						AccessKeyID:     "AKID_EXEC",
						SecretAccessKey: "secret_exec",
						SessionToken:    "token_exec",
						Expiration:      "2026-05-05T12:00:00Z",
						RoleType:        credentials.ExecutionRoleType,
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockEngine := mock_engine.NewMockTaskEngine(ctrl)
			mockCredManager := mock_credentials.NewMockManager(ctrl)
			mockScanner := mock_imds.NewMockScanner(ctrl)

			mockEngine.EXPECT().ListTasks().Return(tc.tasks, tc.listTasksErr)

			if tc.listTasksErr == nil && len(nonTerminalTasksByID(tc.tasks)) > 0 {
				mockScanner.EXPECT().
					Scan(gomock.Any()).
					Return(tc.scanResult, tc.scanErr)
			}

			if len(tc.expectedUpserts) > 0 {
				for _, expected := range tc.expectedUpserts {
					mockCredManager.EXPECT().
						SetTaskCredentials(expected).
						Return(nil)
				}
			} else {
				mockCredManager.EXPECT().
					SetTaskCredentials(gomock.Any()).
					Times(0)
			}

			refresher := &IMDSCredentialRefresher{
				ctx:         context.Background(),
				scanner:     mockScanner,
				credManager: mockCredManager,
				taskEngine:  mockEngine,
			}
			refresher.refresh()
		})
	}
}

func TestUpsertCredential(t *testing.T) {
	tests := []struct {
		name           string
		task           *apitask.Task
		cred           imds.TaskCredential
		expectedUpsert *credentials.TaskIAMRoleCredentials
		upsertErr      error
	}{
		{
			name: "upserts with correct credentials",
			task: newTestTask(testTaskARN1, status.TaskRunning, testCredID1, ""),
			cred: imds.TaskCredential{
				TaskID:          testTaskID1,
				RoleType:        credentials.ApplicationRoleType,
				RoleArn:         testRoleARN1,
				AccessKeyID:     "AKID",
				SecretAccessKey: "secret",
				SessionToken:    "token",
				Expiration:      "2026-05-05T12:00:00Z",
			},
			expectedUpsert: &credentials.TaskIAMRoleCredentials{
				ARN: testTaskARN1,
				IAMRoleCredentials: credentials.IAMRoleCredentials{
					CredentialsID:   testCredID1,
					RoleArn:         testRoleARN1,
					AccessKeyID:     "AKID",
					SecretAccessKey: "secret",
					SessionToken:    "token",
					Expiration:      "2026-05-05T12:00:00Z",
					RoleType:        credentials.ApplicationRoleType,
				},
			},
		},
		{
			name: "SetTaskCredentials error is handled gracefully",
			task: newTestTask(testTaskARN1, status.TaskRunning, testCredID1, ""),
			cred: imds.TaskCredential{
				TaskID:          testTaskID1,
				RoleType:        credentials.ApplicationRoleType,
				RoleArn:         testRoleARN1,
				AccessKeyID:     "AKID",
				SecretAccessKey: "secret",
				SessionToken:    "token",
				Expiration:      "2026-05-05T12:00:00Z",
			},
			expectedUpsert: &credentials.TaskIAMRoleCredentials{
				ARN: testTaskARN1,
				IAMRoleCredentials: credentials.IAMRoleCredentials{
					CredentialsID:   testCredID1,
					RoleArn:         testRoleARN1,
					AccessKeyID:     "AKID",
					SecretAccessKey: "secret",
					SessionToken:    "token",
					Expiration:      "2026-05-05T12:00:00Z",
					RoleType:        credentials.ApplicationRoleType,
				},
			},
			upsertErr: errors.New("write failed"),
		},
		{
			name: "no credentials ID for role type, does not upsert",
			task: newTestTask(testTaskARN1, status.TaskRunning, "", ""),
			cred: imds.TaskCredential{
				TaskID:   testTaskID1,
				RoleType: credentials.ApplicationRoleType,
				RoleArn:  testRoleARN1,
			},
			expectedUpsert: nil,
		},
		{
			name: "unknown role type, does not upsert",
			task: newTestTask(testTaskARN1, status.TaskRunning, testCredID1, ""),
			cred: imds.TaskCredential{
				TaskID:   testTaskID1,
				RoleType: "UnknownType",
				RoleArn:  testRoleARN1,
			},
			expectedUpsert: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockCredManager := mock_credentials.NewMockManager(ctrl)
			if tc.expectedUpsert != nil {
				mockCredManager.EXPECT().
					SetTaskCredentials(tc.expectedUpsert).
					Return(tc.upsertErr)
			} else {
				mockCredManager.EXPECT().
					SetTaskCredentials(gomock.Any()).
					Times(0)
			}

			refresher := &IMDSCredentialRefresher{
				ctx:         context.Background(),
				credManager: mockCredManager,
			}
			refresher.upsertCredential(tc.task, tc.cred)
		})
	}
}

func TestNonTerminalTasksByID(t *testing.T) {
	tests := []struct {
		name        string
		tasks       []*apitask.Task
		expectedIDs []string
	}{
		{
			name:        "empty",
			tasks:       nil,
			expectedIDs: nil,
		},
		{
			name: "filters terminal tasks",
			tasks: []*apitask.Task{
				newTestTask(testTaskARN1, status.TaskRunning, testCredID1, ""),
				newTestTask(testTaskARN2, status.TaskStopped, testCredID3, ""),
			},
			expectedIDs: []string{testTaskID1},
		},
		{
			name: "includes all non-terminal",
			tasks: []*apitask.Task{
				newTestTask(testTaskARN1, status.TaskRunning, testCredID1, ""),
				newTestTask(testTaskARN2, status.TaskRunning, testCredID3, ""),
			},
			expectedIDs: []string{testTaskID1, testTaskID2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tasksByID := nonTerminalTasksByID(tc.tasks)
			assert.Len(t, tasksByID, len(tc.expectedIDs))
			for _, id := range tc.expectedIDs {
				assert.Contains(t, tasksByID, id)
			}
		})
	}
}
