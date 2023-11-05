package volume

import "os"

type VolumeAccessor interface {
	CopyToVolume(src string, dest string, mode os.FileMode) error
	DeleteAll(path string) error
}
