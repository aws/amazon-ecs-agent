package volume

import "os"

// TaskVolumeAccessor provides methods to access data in task EBS volume.
type TaskVolumeAccessor interface {
	CopyToVolume(taskID, src, dest string, mode os.FileMode) error
	DeleteAll(path string) error
}
