package resource

type EBSDiscovery interface {
	ConfirmEBSVolumeIsAttached(deviceName, volumeID string) error
}
