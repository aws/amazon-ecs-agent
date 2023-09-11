package uuid

import (
	"fmt"

	"github.com/google/uuid"
)

const (
	DefaultSeparator = "-"
)

// GenerateWithPrefix returns a unique string with the provided prefix and separator.
func GenerateWithPrefix(prefix string, separator string) string {
	if len(prefix) > 0 {
		return fmt.Sprintf("%s%s%s", prefix, separator, uuid.New().String())
	}
	return uuid.New().String()
}
