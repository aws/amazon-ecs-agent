package csiclient

// Metrics represents the used and capacity bytes of the Volume.
type Metrics struct {
	// Used represents the total bytes used by the Volume.
	Used int64

	// Capacity represents the total capacity (bytes) of the volume's underlying storage.
	Capacity int64
}
