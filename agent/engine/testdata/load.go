package testdata

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"strings"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/golang/mock/gomock"
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

type dockerNameSubstr struct {
	values []string
}

func (m dockerNameSubstr) Matches(arg interface{}) bool {
	sarg := arg.(string)
	for _, s := range m.values {
		if !strings.Contains(sarg, s) {
			return false
		}
	}
	return true
}

// Not used here, but satisfies the Matcher interface.
func (m dockerNameSubstr) String() string {
	return strings.Join(m.values, ", ")
}

func DockerNameSubstr(values ...string) gomock.Matcher {
	return dockerNameSubstr{values: values}
}
