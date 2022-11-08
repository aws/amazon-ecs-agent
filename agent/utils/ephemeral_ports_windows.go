//go:build windows
// +build windows

package utils

const (
	// Ref: https://learn.microsoft.com/en-US/troubleshoot/windows-server/networking/default-dynamic-port-range-tcpip-chang
	// defaultPortRangeStart indicates the first port in ephemeral port range
	defaultPortRangeStart = 49152
	// defaultPortRangeEnd indicates the last port in ephemeral port range
	defaultPortRangeEnd = 65535
)

// getDynamicHostPortRange returns the default ephemeral port range on Windows.
// TODO: instead of sticking to defaults, run netsh commands on the host to get the ranges.
func getDynamicHostPortRange() (start int, end int, err error) {
	return defaultPortRangeStart, defaultPortRangeEnd, nil
}
