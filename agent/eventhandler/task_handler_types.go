package eventhandler

import (
	"container/list"
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

// Maximum number of tasks that may be handled at once by the taskHandler
const CONCURRENT_EVENT_CALLS = 3

// a state change that may have a container and, optionally, a task event to
// send
type sendableEvent struct {
	containerSent bool
	taskSent      bool

	api.ContainerStateChange
}

func newSendableEvent(event api.ContainerStateChange) *sendableEvent {
	return &sendableEvent{
		containerSent:        false,
		taskSent:             false,
		ContainerStateChange: event,
	}
}

type eventList struct {
	sending    bool // whether the list is already being handled
	sync.Mutex      // Locks both the list and sending bool
	*list.List      // list of *sendableEvents
}

type taskHandler struct {
	submitSemaphore utils.Semaphore       // Semaphore on the number of tasks that may be handled at once
	taskMap         map[string]*eventList // arn:*eventList map so events may be serialized per task

	sync.RWMutex // Lock for the taskMap
}

func newTaskHandler() taskHandler {
	taskMap := make(map[string]*eventList)
	submitSemaphore := utils.NewSemaphore(CONCURRENT_EVENT_CALLS)

	return taskHandler{
		taskMap:         taskMap,
		submitSemaphore: submitSemaphore,
	}
}
