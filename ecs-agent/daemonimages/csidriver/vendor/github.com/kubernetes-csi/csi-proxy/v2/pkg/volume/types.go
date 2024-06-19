package volume

type ListVolumesOnDiskRequest struct {
	// Disk device number of the disk to query for volumes
	DiskNumber uint32

	// The partition number (optional), by default it uses the first partition of the disk
	PartitionNumber uint32
}

type ListVolumesOnDiskResponse struct {
	// Volume device IDs of volumes on the specified disk
	VolumeIDs []string
}

type MountVolumeRequest struct {
	// Volume device ID of the volume to mount
	VolumeID string

	// Path in the host's file system where the volume needs to be mounted
	TargetPath string
}

type MountVolumeResponse struct {
	// Intentionally empty
}

type IsVolumeFormattedRequest struct {
	// Volume device ID of the volume to check
	VolumeID string
}

type IsVolumeFormattedResponse struct {
	// Whether the volume is formatted with NTFS
	Formatted bool
}

type FormatVolumeRequest struct {
	// Volume device ID of the volume to format
	VolumeID string
}

type FormatVolumeResponse struct {
	// Intentionally empty
}

type WriteVolumeCacheRequest struct {
	// Volume device ID of the volume to flush the cache
	VolumeID string
}

type WriteVolumeCacheResponse struct {
	// Intentionally empty
}

type UnmountVolumeRequest struct {
	// Volume device ID of the volume to dismount
	VolumeID string

	// Path where the volume has been mounted
	TargetPath string
}

type UnmountVolumeResponse struct {
	// Intentionally empty
}

type ResizeVolumeRequest struct {
	// Volume device ID of the volume to resize
	VolumeID string

	// New size in bytes of the volume
	SizeBytes int64
}

type ResizeVolumeResponse struct {
	// Intentionally empty
}

type GetVolumeStatsRequest struct {
	// Volume device Id of the volume to get the stats for
	VolumeID string
}

type GetVolumeStatsResponse struct {
	// Total bytes
	TotalBytes int64

	// Used bytes
	UsedBytes int64
}

type GetDiskNumberFromVolumeIDRequest struct {
	// Volume device ID of the volume to get the disk number for
	VolumeID string
}

type GetDiskNumberFromVolumeIDResponse struct {
	// Corresponding disk number
	DiskNumber uint32
}

type GetVolumeIDFromTargetPathRequest struct {
	// The target path
	TargetPath string
}

type GetVolumeIDFromTargetPathResponse struct {
	// The volume device ID
	VolumeID string
}

type GetClosestVolumeIDFromTargetPathRequest struct {
	// The target path
	TargetPath string
}

type GetClosestVolumeIDFromTargetPathResponse struct {
	// The volume device ID
	VolumeID string
}
