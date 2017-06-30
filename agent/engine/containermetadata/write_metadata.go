package containermetadata

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/cihub/seelog"
)

const (
	inspectContainerTimeout = 30 * time.Second
	metadataFile            = "metadata.json"
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

// getMetadataFilePath gives the metadata file path for any agent-managed container
func getMetadataFilePath(task *api.Task, container *api.Container, dataDir string) string {
	taskID := getTaskIDfromArn(task.Arn)
	// Empty task ID indicates malformed Arn (Should not happen)
	if taskID == "" {
		return ""
	}
	return fmt.Sprintf("%s/metadata/%s/%s/", dataDir, taskID, container.Name)
}

// writeToMetadata writes given data into the metadata file
func writeToMetadata(task *api.Task, container *api.Container, data []byte, dataDir string) error {
	mdFileDir := getMetadataFilePath(task, container, dataDir)
	mdFilePath := fmt.Sprintf("%s/%s", mdFileDir, metadataFile)

	mdFile, err := os.OpenFile(mdFilePath, os.O_WRONLY, 0644)
	defer mdFile.Close()
	if err != nil {
		return err
	}
	_, err = mdFile.Write(data)
	return err
}

// WriteJSONToMetadata puts the metadata into JSON format and writes into
// the metadata file
func writeJSONToMetadataFile(task *api.Task, container *api.Container, metadata Metadata, dataDir string) error {
	data, err := json.MarshalIndent(&metadata, "", "\t")
	if err != nil {
		return err
	}
	return writeToMetadata(task, container, data, dataDir)
}

// initMetadataFile initializes metadata file and populates it with initially available
// metadata about the container's task and instance
func initMetadataFile(client dockerDummyClient, cfg *config.Config, task *api.Task, container *api.Container) (string, error) {
	// Create task and container directories if they do not yet exist
	mdDirectoryPath := getMetadataFilePath(task, container, cfg.DataDir)
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
	md := acquireMetadataAtContainerCreate(client, cfg, task)
	return mdFilePath, writeJSONToMetadata(task, container, data, cfg.DataDir)
}

// CreateMetadata creates the metadata file and adds the metadata directory to
// the container's mounted host volumes
func CreateMetadata(client dockerDummyClient, cfg *config.Config, binds *[]string, task *api.Task, container *api.Container) error {
	// Do not create metadata file for internal containers
	// TODO: Add error handling for this case?
	if container.IsInternal {
		return nil
	}

	metadataPath, err := initMetadataFile(client, cfg, task, container)
	if err != nil {
		seelog.Errorf("Failed to create metadata file at %s. Error: %s", metadataPath, err.Error())
		return err
	}

	// Add the directory of this container's metadata to the container's mount binds
	metadataFilePath := getMetadataFilePath(task, container, cfg.DataDir)
	instanceBind := fmt.Sprintf("%s/%s:/ecs/metadata/%s", cfg.InstanceDataDir, metadataFilePath, container.Name)
	*binds = append(*binds, instanceBind)
	return nil
}

// UpdateMetadata updates the metadata file after container starts and dynamic
// metadata is available
func UpdateMetadata(client dockerDummyClient, cfg *config.Config, dockerID string, task *api.Task, container *api.Container) error {
	// Do not update (non-existent) metadata file for internal containers
	if container.IsInternal {
		return nil
	}

	dockerContainer, err := client.InspectContainer(dockerID, inspectContainerTimeout)
	if err != nil {
		seelog.Errorf("Failed to inspect container %s of task %s, error: %s", container, task, err.Error())
		return err
	}
	metadata := acquireMetadata(client, dockerContainer, cfg, task)
	err = writeJSONToMetadataFile(task, container, metadata, cfg.DataDir)
	if err != nil {
		seelog.Errorf("Failed to update metadata file for task %s container %s, error: %s", task, container, err.Error())
	} else {
		seelog.Debugf("Updated metadata file for task %s container %s", task, container)
	}
	return err
}

// getTaskMetadataDir acquires the directory with all of the metadata
// files of a given task
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

// CleanTaskMetadata removes the metadata files of all containers associated with a task
func CleanTaskMetadata(task *api.Task, dataDir string) error {
	mdPath := getTaskMetadataDir(task, dataDir)
	return removeContents(mdPath)
}
