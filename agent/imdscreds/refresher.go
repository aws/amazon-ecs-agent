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
	// ScanInterval is the default interval between IMDS credentials scans.
	// TODO: this value will be finalized based on load testing.
	ScanInterval = 15 * time.Minute
)

// IMDSCredentialsRefresher periodically scans IMDS for rotated task credentials
// and upserts them into the credentials manager.
type IMDSCredentialsRefresher struct {
	ctx          context.Context
	scanner      imds.Scanner
	credManager  credentials.Manager
	taskEngine   engine.TaskEngine
	scanInterval time.Duration
}

// NewIMDSCredentialsRefresher creates a new IMDS credentials refresher.
func NewIMDSCredentialsRefresher(
	ctx context.Context,
	scanner imds.Scanner,
	credManager credentials.Manager,
	taskEngine engine.TaskEngine,
	scanInterval time.Duration,
) *IMDSCredentialsRefresher {
	return &IMDSCredentialsRefresher{
		ctx:          ctx,
		scanner:      scanner,
		credManager:  credManager,
		taskEngine:   taskEngine,
		scanInterval: scanInterval,
	}
}

// Start begins the periodic IMDS credentials scan loop. It blocks until the
// context is cancelled.
func (r *IMDSCredentialsRefresher) Start() {
	logger.Info("Starting IMDS credentials refresher")
	ticker := time.NewTicker(r.scanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.refresh()
		case <-r.ctx.Done():
			logger.Info("IMDS credentials refresher stopped")
			return
		}
	}
}

// refresh performs a single IMDS credentials scan and upserts any matching
// credentials into the credentials manager.
func (r *IMDSCredentialsRefresher) refresh() {
	tasks, err := r.taskEngine.ListTasks()
	if err != nil {
		logger.Error("IMDS credentials refresh: failed to list tasks", logger.Fields{
			field.Error: err,
		})
		return
	}

	// Build a map of task ID -> task for non-terminal tasks.
	nonTerminalTasks := nonTerminalTasksByID(tasks)
	if len(nonTerminalTasks) == 0 {
		logger.Debug("IMDS credentials refresh: no non-terminal tasks, skipping scan")
		return
	}

	creds, err := r.scanner.Scan(r.ctx)
	if err != nil {
		logger.Error("IMDS credentials refresh: scan failed", logger.Fields{
			field.Error: err,
		})
		return
	}

	for _, cred := range creds {
		task, ok := nonTerminalTasks[cred.TaskID]
		if !ok {
			// Credential for a task that's either terminal or unknown
			// (for example, due to data corruption); skip.
			logger.Debug("IMDS credentials refresh: task not in non-terminal set, skipping", logger.Fields{
				field.TaskID: cred.TaskID,
			})
			continue
		}
		r.upsertCredential(task, cred)
	}
}

// upsertCredential maps an IMDS credential to the task's role type
// and upserts it into the credentials manager.
func (r *IMDSCredentialsRefresher) upsertCredential(
	task *apitask.Task, cred imds.TaskCredential,
) {
	credentialsID := task.GetCredentialsIDForRoleType(cred.RoleType)
	if credentialsID == "" {
		logger.Warn("IMDS credentials refresh: no credentials ID for task",
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
		logger.Error("IMDS credentials refresh: failed to upsert credential",
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
