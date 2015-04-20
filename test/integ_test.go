package integ_test

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/handlers"
	"github.com/aws/amazon-ecs-agent/test/testhelper"
	"github.com/awslabs/aws-sdk-go/service/ecs"
)

func setup(t *testing.T) (client *ecs.ECS, cluster string, containerInstanceArn string) {
	var localMetadata handlers.MetadataResponse
	agentMetadataResp, err := http.Get("http://localhost:51678/v1/metadata")
	if err != nil {
		t.Fatal("Could not access agent metadata; is it running?", err)
	}
	metadata, err := ioutil.ReadAll(agentMetadataResp.Body)
	if err != nil {
		t.Fatal(err)
	}

	err = json.Unmarshal(metadata, &localMetadata)
	if err != nil {
		t.Fatal(err)
	}
	if localMetadata.ContainerInstanceArn == nil {
		t.Fatal("No container instance arn yet; Perhaps it had trouble registering")
	}

	client = ecs.New(nil)

	return client, localMetadata.Cluster, *localMetadata.ContainerInstanceArn
}

func TestTasks(t *testing.T) {
	// TestTaskTransitions verifies that each task in testdata/tasks/* will
	// follow the given definition.
	client, cluster, ciArn := setup(t)

	taskTests, err := filepath.Glob("./testdata/tasks/*")
	if err != nil {
		t.Fatal(err)
	}
	if len(taskTests) == 0 {
		t.Fatal("Could not find test task data; is your cwd correct?")
	}

	tester := testhelper.New(t, client, cluster, ciArn)

	for ndx, dir := range taskTests {
		if stat, err := os.Stat(dir); err != nil || !stat.IsDir() {
			if err != nil {
				t.Errorf("Error statting test directory: #%v, %v\n", ndx, dir)
			}
			continue
		}

		metadataFile, err1 := ioutil.ReadFile(filepath.Join(dir, "metadata.json"))
		taskdefinitionFile, err2 := ioutil.ReadFile(filepath.Join(dir, "task-definition.json"))
		if err1 != nil || err2 != nil {
			t.Error("Error reading files for a tests: #%v, %v, %v, %v\n", ndx, dir, err1, err2)
			continue
		}
		var taskMetadata testhelper.TaskTestMetadata
		err = json.Unmarshal(metadataFile, &taskMetadata)
		if err != nil {
			t.Errorf("Could not parse metadata file: #%v, %v, %v", ndx, dir, err)
			continue
		}

		tdArn, err := idempotentRegisterTD(client, taskdefinitionFile)
		if err != nil {
			t.Errorf("Could not register task definition: #%v, %v, %v", ndx, dir, err)
			continue
		}

		tester.RunTaskTest(&taskMetadata, tdArn)
	}
}

func idempotentRegisterTD(client *ecs.ECS, tdData []byte) (string, error) {
	registerRequest := &ecs.RegisterTaskDefinitionInput{}
	err := json.Unmarshal(tdData, registerRequest)
	if err != nil {
		return "", err
	}

	tdHash := fmt.Sprintf("%x", md5.Sum(tdData))
	idempotentFamily := *registerRequest.Family + "-" + tdHash

	existing, err := client.DescribeTaskDefinition(&ecs.DescribeTaskDefinitionInput{
		TaskDefinition: &idempotentFamily,
	})
	if err == nil {
		return fmt.Sprintf("%s:%d", *existing.TaskDefinition.Family, *existing.TaskDefinition.Revision), nil
	}

	registerRequest.Family = &idempotentFamily

	registered, err := client.RegisterTaskDefinition(registerRequest)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", *registered.TaskDefinition.Family, *registered.TaskDefinition.Revision), nil
}
