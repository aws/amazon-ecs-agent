package networkinterface

// Status represents the status of an ENI resource.
type Status string

const (
	// StatusNone is the initial staus of the ENI.
	StatusNone Status = "NONE"
	// StatusReadyPull indicates that the ENI is ready for downloading resources associated with
	// the execution role. This includes container images, task secrets and configs.
	StatusReadyPull Status = "READY_PULL"
	// StatusReady indicates that the ENI is ready for use by containers in the task.
	StatusReady Status = "READY"
	// StatusDeleted indicates that the ENI is deleted.
	StatusDeleted Status = "DELETED"
)

var (
	eniStatusOrder = map[Status]int{
		StatusNone:      0,
		StatusReadyPull: 1,
		StatusReady:     2,
		StatusDeleted:   3,
	}
)

func (es Status) String() string {
	return string(es)
}

func (es Status) StatusBackwards(es2 Status) bool {
	return eniStatusOrder[es] < eniStatusOrder[es2]
}

func GetAllStatuses() []Status {
	return []Status{
		StatusNone,
		StatusReadyPull,
		StatusReady,
		StatusDeleted,
	}
}
