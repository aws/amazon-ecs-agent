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
	ecs_metadata_dir = "/var/lib/ecs/data/metadata/"
)

func InitMetadataFile(task *api.Task, container *api.Container, path string) error {
/*	dir, err := os.Open(path)
	defer dir.Close()
	if err != nil {
		return err
	}
	err = dir.Chmod(0644)
	if err != nil {
		return err
	}*/
	tmpfile, err := ioutil.TempFile(path, "tmp_metadata")
	if err != nil {
		return err
	}
	test_msg := GetMetadataFilePath(task, container) + "metadata.json" //TODO: Remove later
	_, err = tmpfile.Write([]byte(test_msg))
	if err != nil {
		return err
	}
	err = os.Rename(tmpfile.Name(), path + "metadata.json")
	return err
/*	mdfile, err := os.Create(path + "meatdata.json")
	if err != nil {
		return err
	}
	defer mdfile.Close()
	_, err = mdfile.Write([]byte(test_msg))
	return err*/
}

func InjectDockerMetadata(task *api.Task, container *api.Container, dmd *DockerMetadata) error {
	mdfile_path := "metadata.json" //TODO: Change to correct file path later
	if dmd == nil {
		return nil //TODO: Write proper error handling
	}
	mdfile, err := os.OpenFile(mdfile_path, os.O_WRONLY | os.O_APPEND, 0644)
	defer mdfile.Close();
	if  err == nil {
		return nil //TODO: Write proper error handling 
	}
	_, err = mdfile.WriteString(dockerMetadataToJSON(dmd))
	return err
}

func GetMetadataFilePath(task *api.Task, container *api.Container) string {
	return ecs_metadata_dir + getIDfromArn(task.Arn) + "/" + container.Name + "/"
}

func getIDfromArn(taskarn string) string {
	taskArnRegex := regexp.MustCompile(taskArnRegexExpr)
	arnsplit := taskArnRegex.Split(taskarn, -1)
	return arnsplit[3]
}

func dockerMetadataToJSON(dmd *DockerMetadata) string {
	if dmd == nil {
		return ""
	}
	json := ""
	json += dmd.Status + "\n"
	json += dmd.ContainerID + "\n"
	json += dmd.ContainerName + "\n"
	json += dmd.ImageName + "\n"
	json += dmd.ImageID + "\n"
	json += networkMetadataToJSON(dmd.NetworkInfo) + "\n"
	return json
}

func networkMetadataToJSON(cnmd *ContainerNetworkMetadata) string {
	return "" //TODO: Parse later
}
