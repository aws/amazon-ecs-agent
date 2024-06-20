package disk

type DiskLocation struct {
	Adapter string
	Bus     string
	Target  string
	LUNID   string
}

type ListDiskLocationsRequest struct {
	// Intentionally empty
}

type ListDiskLocationsResponse struct {
	// Map of disk device IDs and <adapter, bus, target, lun ID> associated with each disk device
	DiskLocations map[uint32]*DiskLocation
}

type PartitionDiskRequest struct {
	// Disk device ID of the disk to partition
	DiskNumber uint32
}

type PartitionDiskResponse struct {
	// Intentionally empty
}

type RescanRequest struct {
	// Intentionally empty
}

type RescanResponse struct {
	// Intentionally empty
}

type ListDiskIDsRequest struct {
	// Intentionally empty
}

type DiskIDs struct {
	// Map of Disk ID types and Disk ID values
	Page83 string

	// The disk serial number
	SerialNumber string
}

type ListDiskIDsResponse struct {
	// Map of disk device numbers and IDs associated with each disk device
	DiskIDs map[uint32]*DiskIDs
}

type GetDiskStatsRequest struct {
	// Disk device number of the disk to get the stats from
	DiskNumber uint32
}

type GetDiskStatsResponse struct {
	TotalBytes int64
}

type SetDiskStateRequest struct {
	// Disk device ID of the disk which state will change
	DiskNumber uint32

	// Online state to set for the disk. true for online, false for offline
	IsOnline bool
}

type SetDiskStateResponse struct {
}

type GetDiskStateRequest struct {
	// Disk device ID of the disk
	DiskNumber uint32
}

type GetDiskStateResponse struct {
	// Online state of the disk. true for online, false for offline
	IsOnline bool
}
