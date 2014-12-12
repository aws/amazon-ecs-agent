package api

var taskStatusMap = map[string]TaskStatus{
	"NONE":    TaskStatusNone,
	"UNKNOWN": TaskStatusUnknown,
	"CREATED": TaskCreated,
	"RUNNING": TaskRunning,
	"STOPPED": TaskStopped,
	"DEAD":    TaskDead,
}

func (ts *TaskStatus) String() string {
	for k, v := range taskStatusMap {
		if v == *ts {
			return k
		}
	}
	return "UNKNOWN"
}

var containerStatusMap = map[string]ContainerStatus{
	"NONE":    ContainerStatusNone,
	"UNKNOWN": ContainerStatusUnknown,
	"PULLED":  ContainerPulled,
	"CREATED": ContainerCreated,
	"RUNNING": ContainerRunning,
	"STOPPED": ContainerStopped,
	"DEAD":    ContainerDead,
}

func (cs *ContainerStatus) String() string {
	for k, v := range containerStatusMap {
		if v == *cs {
			return k
		}
	}
	return "UNKNOWN"
}

func (ts *TaskStatus) ContainerStatus() ContainerStatus {
	switch *ts {
	case TaskStatusNone:
		return ContainerStatusNone
	case TaskCreated:
		return ContainerCreated
	case TaskRunning:
		return ContainerRunning
	case TaskStopped:
		return ContainerStopped
	case TaskDead:
		return ContainerDead
	}
	return ContainerStatusUnknown
}
