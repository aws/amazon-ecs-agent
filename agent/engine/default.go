package engine

import (
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

var log = logger.ForModule("TaskEngine")

// NewTaskEngine returns a default TaskEngine
func NewTaskEngine() (TaskEngine, error) {
	return InitDockerTaskEngine()
}

// MustTaskEngine tries to create a task engine until it succeeds without
// error. It will print a error message for the first error.
func MustTaskEngine() TaskEngine {
	errorOnce := sync.Once{}

	var taskEngine TaskEngine
	taskEngineConnectBackoff := utils.NewSimpleBackoff(200*time.Millisecond, 2*time.Second, 0.20, 1.5)
	utils.RetryWithBackoff(taskEngineConnectBackoff, func() error {
		var err error
		taskEngine, err = NewTaskEngine()
		if err != nil {
			errorOnce.Do(func() {
				log.Error("Could not connect to docker daemon", "err", err)
			})
		}
		return err
	})

	return taskEngine
}
