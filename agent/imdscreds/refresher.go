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

// Package imdscreds provides a periodic refresher that retrieves task
// credentials from IMDS and upserts them into the credentials manager.
package imdscreds

import (
	"context"
	"time"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials/imds"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
)

const (
	// scanInterval is the interval between IMDS credential scans.
	// TODO: this value will be finalized based on load testing.
	scanInterval = 15 * time.Minute
)

// IMDSCredentialRefresher periodically scans IMDS for rotated task credentials
// and upserts them into the credentials manager.
type IMDSCredentialRefresher struct {
	ctx         context.Context
	scanner     imds.Scanner
	credManager credentials.Manager
	taskEngine  engine.TaskEngine
}

// NewIMDSCredentialRefresher creates a new IMDS credential refresher.
func NewIMDSCredentialRefresher(
	ctx context.Context,
	scanner imds.Scanner,
	credManager credentials.Manager,
	taskEngine engine.TaskEngine,
) *IMDSCredentialRefresher {
	return &IMDSCredentialRefresher{
		ctx:         ctx,
		scanner:     scanner,
		credManager: credManager,
		taskEngine:  taskEngine,
	}
}

// Start begins the periodic IMDS credential scan loop. It blocks until the
// context is cancelled.
func (r *IMDSCredentialRefresher) Start() {
	logger.Info("Starting IMDS credential refresher")
	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.refresh()
		case <-r.ctx.Done():
			logger.Info("IMDS credential refresher stopped")
			return
		}
	}
}

// refresh performs a single IMDS credential scan and upserts any matching
// credentials into the credentials manager.
func (r *IMDSCredentialRefresher) refresh() {
	tasks, err := r.taskEngine.ListTasks()
	if err != nil {
		logger.Error("IMDS credential refresh: failed to list tasks", logger.Fields{
			field.Error: err,
		})
		return
	}

	// Build a map of task ID -> task for non-terminal tasks.
	nonTerminalTasks := nonTerminalTasksByID(tasks)
	if len(nonTerminalTasks) == 0 {
		logger.Debug("IMDS credential refresh: no non-terminal tasks, skipping scan")
		return
	}

	creds, err := r.scanner.Scan(r.ctx)
	if err != nil {
		logger.Error("IMDS credential refresh: scan failed", logger.Fields{
			field.Error: err,
		})
		return
	}

	for _, cred := range creds {
		task, ok := nonTerminalTasks[cred.TaskID]
		if !ok {
			// Credential for a task that's either terminal or unknown
			// (for example, due to data corruption); skip.
			logger.Debug("IMDS credential refresh: task not in non-terminal set, skipping", logger.Fields{
				field.TaskID: cred.TaskID,
			})
			continue
		}
		r.upsertCredential(task, cred)
	}
}

// upsertCredential maps an IMDS credential to the task's role type
// and upserts it into the credentials manager.
func (r *IMDSCredentialRefresher) upsertCredential(
	task *apitask.Task, cred imds.TaskCredential,
) {
	credentialsID := task.GetCredentialsIDForRoleType(cred.RoleType)
	if credentialsID == "" {
		logger.Warn("IMDS credential refresh: no credentials ID for task",
			logger.Fields{
				field.TaskID: cred.TaskID,
				"roleType":   cred.RoleType,
			})
		return
	}

	err := r.credManager.SetTaskCredentials(&credentials.TaskIAMRoleCredentials{
		ARN: task.Arn,
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			CredentialsID:   credentialsID,
			RoleArn:         cred.RoleArn,
			AccessKeyID:     cred.AccessKeyID,
			SecretAccessKey: cred.SecretAccessKey,
			SessionToken:    cred.SessionToken,
			Expiration:      cred.Expiration,
			RoleType:        cred.RoleType,
		},
	})
	if err != nil {
		logger.Error("IMDS credential refresh: failed to upsert credential",
			logger.Fields{
				field.TaskID: cred.TaskID,
				field.Error:  err,
			})
	}
}

// nonTerminalTasksByID returns a map of non-terminal ECS tasks keyed by the task ID.
func nonTerminalTasksByID(tasks []*apitask.Task) map[string]*apitask.Task {
	tasksByID := make(map[string]*apitask.Task, len(tasks))
	for _, task := range tasks {
		if task.GetKnownStatus().Terminal() {
			continue
		}
		if id := task.GetID(); id != "" {
			tasksByID[id] = task
		}
	}
	return tasksByID
}
