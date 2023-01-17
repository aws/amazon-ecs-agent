// +build linux

package udev

import (
	"errors"
	"strings"
)

// ModAlias abstracts a MODALIAS identifier string
type ModAlias struct {
	Type  string
	Value string
}

// ParseModAlias parses a MODALIAS identifier
func ParseModAlias(input string) (*ModAlias, error) {
	bits := strings.SplitN(input, ":", 2)
	if len(bits) != 2 {
		return nil, errors.New("Invalid string prefix")
	}

	modalias := &ModAlias{
		Type:  bits[0],
		Value: bits[1],
	}

	return modalias, nil
}
