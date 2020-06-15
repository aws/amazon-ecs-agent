package testdata

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"runtime"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
)

func LoadTask(name string) *apitask.Task {
	_, filename, _, _ := runtime.Caller(0)
	filedata, err := ioutil.ReadFile(filepath.Join(filepath.Dir(filename), "test_tasks", name+".json"))
	if err != nil {
		panic(err)
	}
	t := &apitask.Task{}
	if err := json.Unmarshal(filedata, t); err != nil {
		panic(err)
	}
	return t
}
