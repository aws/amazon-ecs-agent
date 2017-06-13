package metadataservice

import (
	"io/ioutil"
	"os"
	"regexp"

	"github.com/aws/amazon-ecs-agent/agent/api"
	//docker "github.com/fsouza/go-dockerclient"
)

const (
	taskArnRegexExpr = "(:task/)|(arn:aws:ecs:)|:"
	ecs_metadata_dir = "/var/lib/ecs/data/metadata/"
)

func InitMetadataFile(task *api.Task, container *api.Container, path string) error {
	test_msg := GetMetadataFilePath(task, container) + "/metadata.json" //TODO: Remove later
	return ioutil.WriteFile(path, []byte(test_msg), os.FileMode(0600))
}

func InjectDockerMetadata(task *api.Task, container *api.Container, dmd *DockerMetadata) error {
	mdfile_path := "metadata.json" //TODO: Change to correct file path later
	if dmd == nil {
		return nil //TODO: Write proper error handling
	}
	mdfile, err := os.OpenFile(mdfile_path, os.O_WRONLY | os.O_APPEND, 0644)
	defer mdfile.Close();
	if  err == nil {
		os.Stderr.WriteString("Unable to open file EDTEST")
		return nil //TODO: Write proper error handling 
	}
	_, err = mdfile.WriteString(dockerMetadataToJSON(dmd))
	return err
}

func GetMetadataFilePath(task *api.Task, container *api.Container) string {
	return ecs_metadata_dir + getIDfromArn(task.Arn) + "/" + container.Name
}

func getIDfromArn(taskarn string) string {
	taskArnRegex := regexp.MustCompile(taskArnRegexExpr)
	arnsplit := taskArnRegex.Split(taskarn, -1)
	return arnsplit[3]
}

func dockerMetadataToJSON(dmd *DockerMetadata) string {
	os.Stderr.WriteString("HELLO EDTEST")
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
	os.Stderr.WriteString(json + "EDTEST")
	return json
}

func networkMetadataToJSON(cnmd *ContainerNetworkMetadata) string {
	return "" //TODO: Parse later
}
