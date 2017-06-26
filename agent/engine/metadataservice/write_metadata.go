package metadataservice

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/api"
)

//Gets the metadata file path for any agent-managed container
func GetMetadataFilePath(task *api.Task, container *api.Container, dataDir string) string {
	taskID := getIDfromArn(task.Arn)
	//Empty task ID indicates malformed Arn (Should not happen)
	if taskID == "" {
		return ""
	}
	return dataDir + "/" + "metadata/" + getIDfromArn(task.Arn) + "/" + container.Name + "/"
}

//Initialize Metadata file
//TODO: Write static data to it initially and use Metadata struct for dynamic
//data exclusively
func InitMetadataFile(task *api.Task, container *api.Container, dataDir string) (string, error) {
	mddir_path := GetMetadataFilePath(task, container, dataDir)
	err := os.MkdirAll(mddir_path, os.ModePerm)
	if err != nil {
		return "", err
	}
	mdfile_path := mddir_path + "metadata.json"
	return mdfile_path, ioutil.WriteFile(mdfile_path, nil, 0644)
}

func writeToMetadata(task *api.Task, container *api.Container, data []byte, dataDir string) error {
	mdfile_path := GetMetadataFilePath(task, container, dataDir) + "metadata.json"

	mdfile, err := os.OpenFile(mdfile_path, os.O_WRONLY|os.O_APPEND, 0644)
	defer mdfile.Close()
	if err != nil {
		return err
	}
	_, err = mdfile.Write(data)
	return err
}

//Writes Metadata struct in JSON format into the metadatafile
func WriteToMetadata(task *api.Task, container *api.Container, metadata *Metadata, dataDir string) error {
	data, err := json.MarshalIndent(*metadata, "", "\t")
	if err != nil {
		return err
	}
	return writeToMetadata(task, container, data, dataDir)
}

func getTaskMetadataDir(task *api.Task, dataDir string) string {
	return dataDir + "/" + "metadata/" + getIDfromArn(task.Arn) + "/"
}

//Removes directory and all its children. We use this instead of os.RemoveAll to handle case
//where the directory does not exist
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

//Cleans the metadata files of all containers associated with a task
func CleanTask(task *api.Task, dataDir string) error {
	md_path := getTaskMetadataDir(task, dataDir)
	return removeContents(md_path)
}

func getIDfromArn(taskarn string) string {
	colonSplitArn := strings.SplitN(taskarn, ":", 6)
	//Incorrectly formatted Arn (Should not happen)
	if len(colonSplitArn) < 6 {
		return ""
	}
	arnTaskPartSplit := strings.SplitN(colonSplitArn[5], "/", 2)
	//Incorrectly formatted Arn (Should not happen)
	if len(arnTaskPartSplit) < 2 {
		return ""
	}
	return arnTaskPartSplit[1]
}
