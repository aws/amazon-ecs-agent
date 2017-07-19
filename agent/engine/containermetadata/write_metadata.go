// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package containermetadata

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/cihub/seelog"
)

const (
	metadataJoinSuffix = "metadata"
	metadataFile       = "metadata.json"
	readOnlyPerm       = 0644
)

// getTaskIDfromArn parses a task Arn and produces the task ID
// A task Arn has format arn:aws:ecs:[region]:[account-id]:task/[task-id]
// For a correctly formatted Arn we split it over ":" into 6 parts, the last part
// containing the task-id which we extract by splitting it by "/".
func getTaskIDfromArn(taskarn string) string {
	colonSplitArn := strings.SplitN(taskarn, ":", 6)
	// Incorrectly formatted Arn (Should not happen)
	if len(colonSplitArn) < 6 {
		seelog.Errorf("Error in parsing task Arn: Invalid TaskArn %s", taskarn)
		return ""
	}
	arnTaskPartSplit := strings.SplitN(colonSplitArn[5], "/", 2)
	// Incorrectly formatted Arn (Should not happen)
	if len(arnTaskPartSplit) < 2 {
		seelog.Errorf("Error in parsing task Arn: Invalid Arn %s", taskarn)
		return ""
	}
	return arnTaskPartSplit[1]
}

// getMetadataFilePath gives the metadata file path for any agent-managed container
func getMetadataFilePath(task *api.Task, container *api.Container, dataDir string) (string, error) {
	taskID := getTaskIDfromArn(task.Arn)
	// Empty task ID indicates malformed Arn (Should not happen)
	if taskID == "" {
		err := fmt.Errorf("Error in getting metadata file path: Malformed task Arn")
		return "", err
	}
	return filepath.Join(dataDir, metadataJoinSuffix, taskID, container.Name), nil
}

// mdFileExist checks if metadata file exists or not
func mdFileExist(task *api.Task, container *api.Container, dataDir string) bool {
	mdFileDir, err := getMetadataFilePath(task, container, dataDir)
	// Case when file path is invalid (Due to malformed task Arn)
	if err != nil {
		seelog.Errorf("Metadata file not found due to error: %v", err)
		return false
	}

	mdFilePath := filepath.Join(mdFileDir, metadataFile)
	if _, err = os.Stat(mdFilePath); err != nil {
		if os.IsNotExist(err) {
			return false
		} else if err != nil {
			// We should specifically log any error besides "IsNotExist"
			seelog.Errorf("Metadata file not found due to error: %v", err)
			return false
		}
	}
	return true
}

// writeToMetadata puts the metadata into JSON format and writes into
// the metadata file
func (md *Metadata) writeToMetadataFile(task *api.Task, container *api.Container, dataDir string) error {
	data, err := json.MarshalIndent(md, "", "\t")
	if err != nil {
		return err
	}
	mdFileDir, err := getMetadataFilePath(task, container, dataDir)
	// Boundary case if file path is bad (Such as if task arn is incorrectly formatted)
	if err != nil {
		err = fmt.Errorf("Failed to write to metadata: %v", err)
		return err
	}
	mdFilePath := filepath.Join(mdFileDir, metadataFile)

	return ioutil.WriteFile(mdFilePath, data, readOnlyPerm)
}

// getTaskMetadataDir acquires the directory with all of the metadata
// files of a given task
func getTaskMetadataDir(task *api.Task, dataDir string) string {
	return filepath.Join(dataDir, metadataJoinSuffix, getTaskIDfromArn(task.Arn))
}
