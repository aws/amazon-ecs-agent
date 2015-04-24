package testhelper

import (
	"testing"
	"time"

	"github.com/awslabs/aws-sdk-go/service/ecs"
)

type TaskTestMetadata struct {
	Timeout   string
	ExitCodes map[string]int
}

const MaxTaskRunDuration = 20 * time.Minute
const DescribePollWait = 5 * time.Second

type Tester struct {
	Cluster           string
	ContainerInstance string
	t                 *testing.T
	client            *ecs.ECS
}

func New(t *testing.T, client *ecs.ECS, cluster, containerInstance string) *Tester {
	return &Tester{
		Cluster:           cluster,
		ContainerInstance: containerInstance,
		t:                 t,
		client:            client,
	}
}

func (tester *Tester) RunTaskTest(testMetadata *TaskTestMetadata, tdArn string) {
	t, client := tester.t, tester.client
	expectedTimeout, err := time.ParseDuration(testMetadata.Timeout)
	if err != nil {
		t.Errorf("Could not parse timeout: %v, %v", tdArn, err)
		return
	}

	// Setup done, now to actually run the test
	resp, err := client.StartTask(&ecs.StartTaskInput{
		Cluster:            &tester.Cluster,
		ContainerInstances: []*string{&tester.ContainerInstance},
		TaskDefinition:     &tdArn,
	})
	if err != nil {
		t.Errorf("Could not start a task: %v, %v", tdArn, err)
		return
	}
	if len(resp.Failures) != 0 || len(resp.Tasks) == 0 {
		t.Errorf("Could not start a task: %v, %+v", tdArn, *resp.Failures[0])
		return
	}
	task := resp.Tasks[0]
	// Now poll every 5s up to timeout for it to be started
	startTime := time.Now()
	for time.Since(startTime) < MaxTaskRunDuration {
		if time.Since(startTime) > expectedTimeout {
			t.Errorf("Task timed out: %v, %v", tdArn, task.TaskARN)
		}

		resp, err := client.DescribeTasks(&ecs.DescribeTasksInput{
			Cluster: &tester.Cluster,
			Tasks:   []*string{task.TaskARN},
		})
		if err != nil {
			t.Errorf("Could not describe a task: %v, %v", tdArn, err)
			break
		}

		if *resp.Tasks[0].LastStatus == "STOPPED" {
			// Verify all metadata is as expected
			exitMapMatches := 0
			for _, cont := range resp.Tasks[0].Containers {
				if expectedExit, ok := testMetadata.ExitCodes[*cont.Name]; ok {
					exitMapMatches++
					if cont.ExitCode != nil && expectedExit != int(*cont.ExitCode) {
						t.Errorf("Incorrect exit code: %v, %v expected %v got %v", tdArn, *cont.Name, expectedExit, *cont.ExitCode)
					} else {
						t.Logf("Exit code match: %v %v, %v == %v", *task.TaskARN, *cont.Name, expectedExit, *cont.ExitCode)
					}
				}
			}
			if exitMapMatches != len(testMetadata.ExitCodes) {
				t.Errorf("Not all specified exit codes in the metadata matched a container: %v", tdArn)
			}
			break
		}

		time.Sleep(DescribePollWait)
	}
}
