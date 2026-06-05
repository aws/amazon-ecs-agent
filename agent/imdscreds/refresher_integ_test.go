//go:build integration
// +build integration

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

package imdscreds

import (
	"context"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	ecsagentimds "github.com/aws/amazon-ecs-agent/ecs-agent/credentials/imds"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials/imds/testutil"
	ecsagentec2 "github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"

	sdkimds "github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testScanInterval = 1 * time.Second

	taskID1  = "0ee1b6f1feef4ff2bacdf2c99732d506"
	taskID2  = "aabbccdd11223344aabbccdd11223344"
	taskARN1 = "arn:aws:ecs:us-west-2:123456789012:" +
		"task/cluster/" + taskID1
	taskARN2 = "arn:aws:ecs:us-west-2:123456789012:" +
		"task/cluster/" + taskID2

	roleARN1 = "arn:aws:iam::123456789012:role/TaskRoleA"
	roleARN2 = "arn:aws:iam::123456789012:role/TaskRoleB"

	credID1App  = "cred-a-app"
	credID2App  = "cred-b-app"
	credID2Exec = "cred-b-exec"
)

// TestIMDSCredentialsRefresh tests the integration between the IMDS
// credentials refresher, scanner, task engine state, credentials manager,
// and a mock IMDS HTTP server.
//
// Phase 1: Setup the mock IMDS server, task engine, and credentials refresher.
//
// Phase 2: Simulate task payloads arriving with initial credentials, then
// verify the IMDS scan overwrites them with values from the mock server.
//
// Phase 3: Rotate credentials on the mock server and verify the refresher
// picks up the new values on the next scan cycle.
func TestIMDSCredentialsRefresh(t *testing.T) {
	// Phase 1: Setup.
	//
	// Start a mock IMDS server.
	mockIMDS := testutil.NewMockIMDSServer()
	defer mockIMDS.Close()

	// Setup the task engine and credentials manager.
	taskEngine, cleanup, credManager := setupTestEngine(t)
	defer cleanup()

	// Setup the IMDS scanner.
	scanner := ecsagentimds.NewScanner(newEC2Client(t, mockIMDS.URL()), metrics.NewNopEntryFactory())
	ctx, cancel := context.WithCancel(context.Background())

	// Setup the credentials refresher.
	refresher := NewIMDSCredentialsRefresher(
		ctx, scanner, credManager, taskEngine, testScanInterval,
	)

	// Start the credentials refresher.
	// Using a done channel ensures that the scanner stops
	// before task engine and mock IMDS server are stopped.
	done := make(chan struct{})
	go func() {
		defer close(done)
		refresher.Start()
	}()
	defer func() {
		cancel()
		<-done
	}()

	// Simulate task payloads arriving with initial credentials.
	addTaskToState(taskEngine, taskARN1, credID1App, "")
	addTaskToState(taskEngine, taskARN2, credID2App, credID2Exec)

	setInitialTaskCredentials(t, credManager, taskARN1, credID1App, "AKID_ACS_A_APP")
	setInitialTaskCredentials(t, credManager, taskARN2, credID2App, "AKID_ACS_B_APP")
	setInitialTaskCredentials(t, credManager, taskARN2, credID2Exec, "AKID_ACS_B_EXEC")

	// Verify initial credentials are present in the credentials manager.
	verifyCredential(t, credManager,
		credID1App, taskARN1, "AKID_ACS_A_APP", "")
	verifyCredential(t, credManager,
		credID2App, taskARN2, "AKID_ACS_B_APP", "")
	verifyCredential(t, credManager,
		credID2Exec, taskARN2, "AKID_ACS_B_EXEC", "")

	// Phase 2: IMDS scan upserts credentials.
	//
	// Add credentials to the mock IMDS server.
	// Namespace 1: taskA with application role.
	mockIMDS.AddCredential(
		"iam-ecs-1", taskID1,
		credentials.ApplicationRoleType, roleARN1, "AKID_IMDS_A_APP",
	)
	// Namespace 2: taskB with application + execution roles.
	mockIMDS.AddCredential(
		"iam-ecs-2", taskID2,
		credentials.ApplicationRoleType, roleARN2, "AKID_IMDS_B_APP",
	)
	mockIMDS.AddCredential(
		"iam-ecs-2", taskID2,
		credentials.ExecutionRoleType, roleARN2, "AKID_IMDS_B_EXEC",
	)

	// Wait for the refresher to pick up the new credentials from IMDS
	// and deliver it to the credentials manager. The refresher's scan interval is 1s for the test.
	// require.Eventually polls every 200ms for up to 5s until the credentials appear.
	require.Eventually(t, func() bool {
		return hasAccessKey(credManager, credID1App, "AKID_IMDS_A_APP") &&
			hasAccessKey(credManager, credID2App, "AKID_IMDS_B_APP") &&
			hasAccessKey(credManager, credID2Exec, "AKID_IMDS_B_EXEC")
	}, 5*time.Second, 200*time.Millisecond,
		"IMDS credentials not upserted after scan")

	verifyCredential(t, credManager,
		credID1App, taskARN1, "AKID_IMDS_A_APP", roleARN1)
	verifyCredential(t, credManager,
		credID2App, taskARN2, "AKID_IMDS_B_APP", roleARN2)
	verifyCredential(t, credManager,
		credID2Exec, taskARN2, "AKID_IMDS_B_EXEC", roleARN2)

	// Phase 3: Credential rotation.
	//
	// Rotate credentials on the mock IMDS server and verify the refresher
	// picks up the new values on the next scan.
	mockIMDS.RotateCredential(t,
		"iam-ecs-1", taskID1,
		credentials.ApplicationRoleType, "AKID_ROTATED_A_APP",
	)
	mockIMDS.RotateCredential(t,
		"iam-ecs-2", taskID2,
		credentials.ExecutionRoleType, "AKID_ROTATED_B_EXEC",
	)

	require.Eventually(t, func() bool {
		return hasAccessKey(credManager, credID1App, "AKID_ROTATED_A_APP") &&
			hasAccessKey(credManager, credID2Exec, "AKID_ROTATED_B_EXEC") &&
			hasAccessKey(credManager, credID2App, "AKID_IMDS_B_APP")
	}, 5*time.Second, 200*time.Millisecond,
		"rotated credentials not picked up or non-rotated credential mutated")

	verifyCredential(t, credManager,
		credID1App, taskARN1, "AKID_ROTATED_A_APP", roleARN1)
	verifyCredential(t, credManager,
		credID2Exec, taskARN2, "AKID_ROTATED_B_EXEC", roleARN2)
	// Verify non-rotated credential in the same namespace is unchanged.
	verifyCredential(t, credManager,
		credID2App, taskARN2, "AKID_IMDS_B_APP", roleARN2)
}

// setupTestEngine creates a test DockerTaskEngine with a credentials manager.
func setupTestEngine(t *testing.T) (
	engine.TaskEngine, func(), credentials.Manager,
) {
	taskEngine, cleanup, _, credManager := engine.SetupIntegTestTaskEngine(
		engine.DefaultTestConfigIntegTest(), nil, t,
	)
	return taskEngine, cleanup, credManager
}

// addTaskToState adds a task to the engine's state.
func addTaskToState(
	taskEngine engine.TaskEngine,
	arn, credID, execCredID string,
) {
	testTask := &task.Task{Arn: arn}
	testTask.SetDesiredStatus(apitaskstatus.TaskRunning)
	testTask.SetKnownStatus(apitaskstatus.TaskRunning)
	if credID != "" {
		testTask.SetCredentialsID(credID)
	}
	if execCredID != "" {
		testTask.SetExecutionRoleCredentialsID(execCredID)
	}
	taskEngine.(*engine.DockerTaskEngine).State().AddTask(testTask)
}

// setInitialTaskCredentials sets the initial task credentials in the credentials manager.
func setInitialTaskCredentials(
	t *testing.T,
	credManager credentials.Manager,
	taskARN, credID, accessKeyID string,
) {
	err := credManager.SetTaskCredentials(&credentials.TaskIAMRoleCredentials{
		ARN: taskARN,
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			CredentialsID:   credID,
			AccessKeyID:     accessKeyID,
			SecretAccessKey: "secret-" + accessKeyID,
			SessionToken:    "token-" + accessKeyID,
		},
	})
	require.NoError(t, err)
}

// newEC2Client creates an EC2MetadataClient configured with the mock IMDS server.
func newEC2Client(t *testing.T, endpoint string) ecsagentec2.EC2MetadataClient {
	imdsClient := sdkimds.New(sdkimds.Options{Endpoint: endpoint})
	ec2Client, err := ecsagentec2.NewEC2MetadataClient(imdsClient)
	require.NoError(t, err)
	return ec2Client
}

// hasAccessKey returns true if the credential with the given ID has the
// expected access key.
func hasAccessKey(
	credManager credentials.Manager, credID, expectedKey string,
) bool {
	cred, ok := credManager.GetTaskCredentials(credID)
	return ok && cred.IAMRoleCredentials.AccessKeyID == expectedKey
}

// verifyCredential asserts all fields of a credential in the credentials manager.
func verifyCredential(
	t *testing.T,
	credManager credentials.Manager,
	credID, expectedARN, expectedAccessKey, expectedRoleArn string,
) {
	cred, ok := credManager.GetTaskCredentials(credID)
	require.True(t, ok, "credential %s not found", credID)
	assert.Equal(t, expectedARN, cred.ARN)
	assert.Equal(t, credID, cred.IAMRoleCredentials.CredentialsID)
	assert.Equal(t, expectedRoleArn, cred.IAMRoleCredentials.RoleArn)
	assert.Equal(t, expectedAccessKey, cred.IAMRoleCredentials.AccessKeyID)
	assert.Equal(t,
		"secret-"+expectedAccessKey, cred.IAMRoleCredentials.SecretAccessKey)
	assert.Equal(t,
		"token-"+expectedAccessKey, cred.IAMRoleCredentials.SessionToken)
}
