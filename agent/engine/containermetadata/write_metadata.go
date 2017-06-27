package containermetadata

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
)

const (
	metadataFile = "metadata.json"
)

// getTaskIDfromArn parses a task Arn and produces the task ID
func getTaskIDfromArn(taskarn string) string {
	colonSplitArn := strings.SplitN(taskarn, ":", 6)
	// Incorrectly formatted Arn (Should not happen)
	if len(colonSplitArn) < 6 {
		return ""
	}
	arnTaskPartSplit := strings.SplitN(colonSplitArn[5], "/", 2)
	// Incorrectly formatted Arn (Should not happen)
	if len(arnTaskPartSplit) < 2 {
		return ""
	}
	return arnTaskPartSplit[1]
}

// GetMetadataFilePath gives the metadata file path for any agent-managed container
func GetMetadataFilePath(task *api.Task, container *api.Container, dataDir string) string {
	taskID := getTaskIDfromArn(task.Arn)
	// Empty task ID indicates malformed Arn (Should not happen)
	if taskID == "" {
		return ""
	}
	return fmt.Sprintf("%s/metadata/%s/%s/", dataDir, taskID, container.Name)
}

// writeToMetadata writes given data into the metadata file
func writeToMetadata(task *api.Task, container *api.Container, data []byte, dataDir string) error {
	mdFilePath := GetMetadataFilePath(task, container, dataDir) + metadataFile

	mdfile, err := os.OpenFile(mdFilePath, os.O_WRONLY, 0644)
	defer mdfile.Close()
	if err != nil {
		return err
	}
	_, err = mdfile.Write(data)
	return err
}

// InitMetadataFile initializes metadata file and populates it with initially available
// metadata
func InitMetadataFile(cfg *config.Config, task *api.Task, container *api.Container, dataDir string) (string, error) {
	// Create task and container directories if they do not yet exist
	mdDirectoryPath := GetMetadataFilePath(task, container, dataDir)
	err := os.MkdirAll(mdDirectoryPath, os.ModePerm)
	if err != nil {
		return "", err
	}

	// Create metadata file
	mdFilePath := mdDirectoryPath + metadataFile
	err = ioutil.WriteFile(mdFilePath, nil, 0644)
	if err != nil {
		return mdFilePath, err
	}

	// Get common metadata of all containers of this task and write it to file
	md := acquireStaticMetadata(cfg, task)
	data, err := json.MarshalIndent(*md, "", "\t")
	if err != nil {
		return mdFilePath, err
	}
	return mdFilePath, writeToMetadata(task, container, data, dataDir)
}

// WriteToMetadata puts the metadata into JSON format and writes into
// the metadatafile
func WriteToMetadata(task *api.Task, container *api.Container, metadata *Metadata, dataDir string) error {
	data, err := json.MarshalIndent(metadata, "", "\t")
	if err != nil {
		return err
	}
	return writeToMetadata(task, container, data, dataDir)
}

// getTaskMetadataDir acquires the directory of a task with all of the metadata
// files of that task
func getTaskMetadataDir(task *api.Task, dataDir string) string {
	return fmt.Sprintf("%s/metadata/%s/", dataDir, getTaskIDfromArn(task.Arn))
}

// removeContents removes a directory and all its children. We use this
// instead of os.RemoveAll to handle case where the directory does not exist
func removeContents(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return nil
	}
	for _, name := range names {
		err = os.RemoveAll(filepath.Join(dir, name))
		if err != nil {
			return err
		}
	}
	return os.Remove(dir)
}

// CleanTask removes the metadata files of all containers associated with a task
func CleanTask(task *api.Task, dataDir string) error {
	mdPath := getTaskMetadataDir(task, dataDir)
	return removeContents(mdPath)
}
