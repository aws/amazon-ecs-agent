package status

// NetworkStatus represents the status of a network resource.
type NetworkStatus string

const (
	// NetworkNone is the initial status of the ENI.
	NetworkNone NetworkStatus = "NONE"
	// NetworkReadyPull indicates that the ENI is ready for downloading resources associated with
	// the execution role. This includes container images, task secrets and configs.
	NetworkReadyPull NetworkStatus = "READY_PULL"
	// NetworkReady indicates that the ENI is ready for use by containers in the task.
	NetworkReady NetworkStatus = "READY"
	// NetworkDeleted indicates that the ENI is deleted.
	NetworkDeleted NetworkStatus = "DELETED"
)

var (
	eniStatusOrder = map[NetworkStatus]int{
		NetworkNone:      0,
		NetworkReadyPull: 1,
		NetworkReady:     2,
		NetworkDeleted:   3,
	}
)

func (es NetworkStatus) String() string {
	return string(es)
}

func (es NetworkStatus) ENIStatusBackwards(es2 NetworkStatus) bool {
	return eniStatusOrder[es] < eniStatusOrder[es2]
}

func GetAllENIStatuses() []NetworkStatus {
	return []NetworkStatus{
		NetworkNone,
		NetworkReadyPull,
		NetworkReady,
		NetworkDeleted,
	}
}
