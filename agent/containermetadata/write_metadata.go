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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/cihub/seelog"
)

const (
	metadataJoinSuffix = "metadata"
	metadataFile       = "metadata.json"
	readOnlyPerm       = 0644
)

// getTaskIDfromARN parses a task ARN and produces the task ID
// A task ARN has format arn:aws:ecs:[region]:[account-id]:task/[task-id]
// For a correctly formatted ARN we split it over ":" into 6 parts, the last part
// containing the task-id which we extract by splitting it by "/".
// This function should be eventually be replaced by a standardized ARN parsing
// module
func getTaskIDfromARN(taskarn string) string {
	colonSplitARN := strings.SplitN(taskarn, ":", 6)
	// Incorrectly formatted ARN (Should not happen)
	if len(colonSplitARN) < 6 {
		seelog.Errorf("Failed to parse task ARN: invalid TaskARN %s", taskarn)
		return ""
	}
	arnTaskPartSplit := strings.SplitN(colonSplitARN[5], "/", 2)
	// Incorrectly formatted ARN (Should not happen)
	if len(arnTaskPartSplit) < 2 {
		seelog.Errorf("Failed to parse task ARN: invalid TaskARN %s", taskarn)
		return ""
	}
	return arnTaskPartSplit[1]
}

// getMetadataFilePath gives the metadata file path for any agent-managed container
func getMetadataFilePath(task *api.Task, container *api.Container, dataDir string) (string, error) {
	taskID := getTaskIDfromARN(task.Arn)
	// Empty task ID indicates malformed ARN (Should not happen)
	if taskID == "" {
		err := fmt.Errorf("Failed to form metdata file path: invalid task ARN")
		return "", err
	}
	return filepath.Join(dataDir, metadataJoinSuffix, taskID, container.Name), nil
}

// metadataFileExists checks if metadata file exists or not
func metadataFileExists(task *api.Task, container *api.Container, dataDir string) bool {
	metadataFileDir, err := getMetadataFilePath(task, container, dataDir)
	// Case when file path is invalid (Due to malformed task ARN)
	if err != nil {
		seelog.Errorf("Failed to find metadata file: %v", err)
		return false
	}

	metadataFilePath := filepath.Join(metadataFileDir, metadataFile)
	if _, err = os.Stat(metadataFilePath); err != nil {
		if !os.IsNotExist(err) {
			// We should specifically log any error besides "IsNotExist"
			seelog.Errorf("Failed to find metadata file: %v", err)
		}
		return false
	}
	return true
}

// writeToMetadata puts the metadata into JSON format and writes into
// the metadata file
/*func (md *Metadata) writeToMetadataFile(task *api.Task, container *api.Container, dataDir string) error {
	data, err := json.MarshalIndent(md, "", "\t")
	if err != nil {
		return err
	}
	metadataFileDir, err := getMetadataFilePath(task, container, dataDir)
	// Boundary case if file path is bad (Such as if task arn is incorrectly formatted)
	if err != nil {
		return fmt.Errorf("Failed to write to metadata file: %v", err)
	}
	metadataFilePath := filepath.Join(metadataFileDir, metadataFile)

	return ioutil.WriteFile(metadataFilePath, data, readOnlyPerm)
}*/

// getMetdataFileUpdateTime gets the last update time of the metadata file
func getMetadataFileUpdateTime(task *api.Task, container *api.Container, dataDir string) (time.Time, error) {
	metadataFileDir, err := getMetadataFilePath(task, container, dataDir)
	if err != nil {
		return time.Time{}, err
	}
	metadataFilePath := filepath.Join(metadataFileDir, metadataFile)
	metadataFile, err := os.Open(metadataFilePath)
	if err != nil {
		return time.Time{}, err
	}
	defer metadataFile.Close()
	fileInfo, err := metadataFile.Stat()
	if err != nil {
		return time.Time{}, err
	}
	return fileInfo.ModTime(), err
}

// getTaskMetadataDir acquires the directory with all of the metadata
// files of a given task
func getTaskMetadataDir(task *api.Task, dataDir string) string {
	return filepath.Join(dataDir, metadataJoinSuffix, getTaskIDfromARN(task.Arn))
}
