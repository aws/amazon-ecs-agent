package eventhandler

import (
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/logger"
)

var log = logger.ForModule("eventhandler")

func HandleEngineEvents(taskEngine engine.TaskEngine, client api.ECSClient) {
	for {
		task_events, task_errc := taskEngine.TaskEvents()

		for task_events != nil {
			select {
			case event, open := <-task_events:
				if !open {
					task_events = nil
					log.Error("Task events closed; this should not happen")
					break
				}

				go AddTaskEvent(event, client)
			case err, _ := <-task_errc:
				log.Error("Error with task events", "err", err)
			}
		}
	}
}
