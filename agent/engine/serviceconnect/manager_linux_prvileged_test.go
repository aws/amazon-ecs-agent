//go:build linux && sudo_unit
// +build linux,sudo_unit

package serviceconnect

import "testing"

func TestAgentContainerModificationsForServiceConnect_Privileged(t *testing.T) {
	testAgentContainerModificationsForServiceConnect(t, true)
}
