// +build linux

package udev

import (
	"errors"
	"strings"
)

// UEvent is a kernel UDev event
type UEvent struct {
	Action  string
	Devpath string
	Env     map[string]string
}

func (event *UEvent) String() (str string) {
	str = event.Action + "@" + event.Devpath
	for k, v := range event.Env {
		str += "\n" + k + "=" + v
	}
	return
}

// GetEnv returns the variable for Key
func (event *UEvent) GetEnv(Key string) string {
	return event.Env[strings.ToUpper(Key)]
}

// Modalias returns the module alias information, if present
func (event *UEvent) Modalias() (*ModAlias, error) {
	return ParseModAlias(event.GetEnv("MODALIAS"))
}

// ParseUEvent parses a byte sequence into a UEvent object
func ParseUEvent(packet []byte) (event *UEvent, err error) {
	lines := strings.Split(string(packet), "\000")
	header := strings.Split(lines[0], "@")
	if 2 != len(header) {
		return nil, errors.New("Invalid action/devpath line")
	}

	event = &UEvent{
		Action:  header[0],
		Devpath: header[1],
		Env:     make(map[string]string),
	}

	for _, line := range lines[1 : len(lines)-2] {
		bits := strings.SplitN(line, "=", 2)
		// Check well-formedness of lines.
		if len(bits) == 2 {
			event.Env[bits[0]] = bits[1]
		} else {
			return nil, errors.New("Invalid environment line")
		}
	}

	return
}
