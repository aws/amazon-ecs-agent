package metadataservice

import (
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
	tmpfile, err := ioutil.TempFile(mdfile_path, "tmp_metadata")
	if err != nil {
		return err
	}
	err = os.Rename(tmpfile.Name(), mdfile_path + "metadata.json")
	return err
}

func WriteToMetadata(task *api.Task, container *api.Container, data []byte) error {
	mdfile_path := GetMetadataFilePath(task, container) + "metadata.json"

	mdfile, err := os.OpenFile(mdfile_path, os.O_WRONLY | os.O_APPEND, 0644)
	defer mdfile.Close()
	if err != nil {
		return err
	}
	_, err = mdfile.Write(data)
	return err
}

func Clean(task *api.Task) error {
	mddir_path := ecsDataMount + "metadata/" + getIDfromArn(task.Arn)
	return os.RemoveAll(mddir_path)
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

