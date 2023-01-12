//go:build windows
// +build windows

package utils

const (
	// Ref: https://learn.microsoft.com/en-US/troubleshoot/windows-server/networking/default-dynamic-port-range-tcpip-chang
	// DefaultPortRangeStart indicates the first port in ephemeral port range
	DefaultPortRangeStart = 49152
	// DefaultPortRangeEnd indicates the last port in ephemeral port range
	DefaultPortRangeEnd = 65535
)

// GetDynamicHostPortRange returns the default ephemeral port range on Windows.
// TODO: instead of sticking to defaults, run netsh commands on the host to get the ranges.
func GetDynamicHostPortRange() (start int, end int, err error) {
	return DefaultPortRangeStart, DefaultPortRangeEnd, nil
}
