package util

import (
	"crypto/sha256"
	"encoding/binary"
)

const (
	// GID range for mount point permissions
	minGID = 900000
	maxGID = 999999
)

// GenerateGIDFromPath generates a GID in range [900000, 999999] from a string
func GenerateGIDFromPath(path string) int {
	// Generate SHA256 hash of the path
	h := sha256.New()
	h.Write([]byte(path))
	hash := h.Sum(nil)

	// Use last 4 bytes of hash to generate number in our range
	num := binary.BigEndian.Uint32(hash[len(hash)-4:])

	// Map to our range
	diff := maxGID - minGID + 1
	gid := minGID + (int(num) % diff)

	return gid
}
