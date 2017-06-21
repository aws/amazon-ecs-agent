package metadataservice

import (
	"encoding/json"
	"path/filepath"
	"io"
	"io/ioutil"
	"os"
	"regexp"

	"github.com/aws/amazon-ecs-agent/agent/api"
//	"github.com/cihub/seelog"
	//docker "github.com/fsouza/go-dockerclient"
)

const (
	taskArnRegexExpr = "(:task/)|(arn:aws:ecs:)|:"
	ecsDataMount = "/data/"
)

func InitMetadataDir(task *api.Task, container *api.Container) (string, error) {
	mddir_path := GetMetadataFilePath(task, container)
	return mddir_path, os.MkdirAll(mddir_path, os.ModePerm)
}

func InitMetadataFile(task *api.Task, container *api.Container) error {
	mdfile_path := GetMetadataFilePath(task, container)
	//tmpfile, err := ioutil.TempFile(mdfile_path, "tmp_metadata")
	return ioutil.WriteFile(mdfile_path + "metadata.json", nil, 0644)
/*	if err != nil {
		return err
	}
	//err = os.Rename(tmpfile.Name(), mdfile_path + "metadata.json")
	return err */
}

func WriteToMetadata(task *api.Task, container *api.Container, metadata *Metadata) error {
	data, err := json.MarshalIndent(*metadata, "", "\t")
	if err != nil {
		return err
	}
	return writeToMetadata(task, container, data)
}

func writeToMetadata(task *api.Task, container *api.Container, data []byte) error {
	mdfile_path := GetMetadataFilePath(task, container) + "metadata.json"

	mdfile, err := os.OpenFile(mdfile_path, os.O_WRONLY | os.O_APPEND, 0644)
	defer mdfile.Close()
	if err != nil {
		return err
	}
	_, err = mdfile.Write(data)
	return err
}

func CleanContainer(task *api.Task, container *api.Container) error {
	mdfile_path := GetMetadataFilePath(task, container)
	return removeContents(mdfile_path)
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

func GetTaskMetadataDir(task *api.Task) string {
	return ecsDataMount + "metadata/" + getIDfromArn(task.Arn) + "/"
}

func GetMetadataFilePath(task *api.Task, container *api.Container) string {
	return ecsDataMount + "metadata/" + getIDfromArn(task.Arn) + "/" + container.Name + "/"
}

//TODO Change to SplitN instead of regex split
func getIDfromArn(taskarn string) string {
	taskArnRegex := regexp.MustCompile(taskArnRegexExpr)
	arnsplit := taskArnRegex.Split(taskarn, -1)
	return arnsplit[3]
}

