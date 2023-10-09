package volume

type VolumeAccessor interface {
	CopyToVolume(src string, dest string) error
	DeleteAll(path string) error
}
