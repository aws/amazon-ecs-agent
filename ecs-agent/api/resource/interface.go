package resource

type EBSDiscovery interface {
	ConfirmEBSVolumeIsAttached(deviceName, volumeID string) error
}

type GenericEBSAttachmentObject interface {
	GetAttachmentProperties(key string) string
	String() string
}
