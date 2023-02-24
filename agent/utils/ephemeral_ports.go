// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package utils

import (
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
)

// From https://www.kernel.org/doc/html/latest//networking/ip-sysctl.html#ip-variables
const (
	EphemeralPortMin         = 32768
	EphemeralPortMax         = 60999
	maxPortSelectionAttempts = 100
	portUnavailableMsg       = "Port %v is unavailable or an error occurred while listening on the local %v network"
	portNotFoundErrMsg       = "a host port is unavailable"
	portsNotFoundErrMsg      = "%v contiguous host ports are unavailable"
	portRangeErrMsg          = "The host port range: %s found by ECS Agent is not within the expected host port range: %s"
	portErrMsg               = "The host port: %s found by ECS Agent is not within the expected host port range: %s"
)

var (
	// Injection point for UTs
	randIntFunc         = rand.Intn
	isPortAvailableFunc = isPortAvailable
	// portLock is a mutex lock used to prevent two concurrent tasks to get the same host ports.
	portLock sync.Mutex
)

// GenerateEphemeralPortNumbers generates a list of n unique port numbers in the 32768-60999 range. The resulting port
// number list is guaranteed to not include any port number present in "reserved" parameter.
func GenerateEphemeralPortNumbers(n int, reserved []uint16) ([]uint16, error) {
	toExcludeSet := map[uint16]struct{}{}
	for _, e := range reserved {
		toExcludeSet[e] = struct{}{}
	}
	rand.Seed(time.Now().UnixNano())

	var result []uint16
	var portSelectionAttempts int
	for len(result) < n {
		// The intention of maxPortSelectionAttempts is to avoid a super highly unlikely case where we
		// keep getting ports that collide, thus creating an infinite loop.
		if portSelectionAttempts > maxPortSelectionAttempts {
			return nil, fmt.Errorf("maximum number of attempts to generate unique ports reached")
		}
		port := uint16(randIntFunc(EphemeralPortMax-EphemeralPortMin+1) + EphemeralPortMin)
		if _, ok := toExcludeSet[port]; ok {
			portSelectionAttempts++
			continue
		}
		toExcludeSet[port] = struct{}{}
		result = append(result, port)
	}
	return result, nil
}

// safePortTracker tracks the host port last assigned to a container port range and is safe to use concurrently.
// TODO: implement a port manager that does synchronization and integrates with a configurable option to modify ephemeral range
type safePortTracker struct {
	mu                   sync.Mutex
	lastAssignedHostPort int
}

// SetLastAssignedHostPort sets the last assigned host port
func (pt *safePortTracker) SetLastAssignedHostPort(port int) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.lastAssignedHostPort = port
}

// GetLastAssignedHostPort returns the last assigned host port
func (pt *safePortTracker) GetLastAssignedHostPort() int {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	return pt.lastAssignedHostPort
}

var tracker safePortTracker

// ResetTracker resets the last assigned host port to 0.
func ResetTracker() {
	tracker.SetLastAssignedHostPort(0)
}

// GetHostPortRange gets N contiguous host ports from the ephemeral host port range defined on the host.
// dynamicHostPortRange can be set by customers using ECS Agent environment variable ECS_DYNAMIC_HOST_PORT_RANGE;
// otherwise, ECS Agent will use the default value returned from GetDynamicHostPortRange() in the utils package.
func GetHostPortRange(numberOfPorts int, protocol string, dynamicHostPortRange string) (string, error) {
	portLock.Lock()
	defer portLock.Unlock()
	result, err := getNumOfHostPorts(numberOfPorts, protocol, dynamicHostPortRange)
	if err == nil {
		// Verify the found host port range is within the given dynamic host port range
		if isInRange := verifyPortsWithinRange(result, dynamicHostPortRange); !isInRange {
			errMsg := fmt.Errorf(portRangeErrMsg, result, dynamicHostPortRange)
			return "", errMsg
		}
	}
	return result, err
}

// GetHostPort gets 1 host port from the ephemeral host port range defined on the host.
// dynamicHostPortRange can be set by customers using ECS Agent environment variable ECS_DYNAMIC_HOST_PORT_RANGE;
// otherwise, ECS Agent will use the default value returned from GetDynamicHostPortRange() in the utils package.
func GetHostPort(protocol string, dynamicHostPortRange string) (string, error) {
	portLock.Lock()
	defer portLock.Unlock()
	numberOfPorts := 1
	result, err := getNumOfHostPorts(numberOfPorts, protocol, dynamicHostPortRange)
	foundHostPort := strings.Split(result, "-")[0]
	if err == nil {
		// Verify the found host port is within the given dynamic host port range
		if isInRange := verifyPortsWithinRange(result, dynamicHostPortRange); !isInRange {
			errMsg := fmt.Errorf(portErrMsg, foundHostPort, dynamicHostPortRange)
			return "", errMsg
		}
	}
	return foundHostPort, err
}

// getNumOfHostPorts returns the requested number of host ports using the given dynamic host port range
// and protocol. If no host port(s) was/were found, an empty string along with the error message will be returned.
func getNumOfHostPorts(numberOfPorts int, protocol, dynamicHostPortRange string) (string, error) {
	// get ephemeral port range, either default or if custom-defined
	startHostPortRange, endHostPortRange, _ := nat.ParsePortRangeToInt(dynamicHostPortRange)
	start := startHostPortRange
	end := endHostPortRange

	// get the last assigned host port
	lastAssignedHostPort := tracker.GetLastAssignedHostPort()
	if lastAssignedHostPort != 0 {
		// this implies that this is not the first time we're searching for host ports
		// so start searching for new ports from the last tracked port
		start = lastAssignedHostPort + 1
	}

	result, lastCheckedPort, err := getHostPortRange(numberOfPorts, start, end, protocol)
	if err != nil {
		if lastAssignedHostPort != 0 {
			// this implies that there are no contiguous host ports available from lastAssignedHostPort to endHostPortRange
			// so, we need to loop back to the startHostPortRange and check for contiguous ports until lastCheckedPort
			start = startHostPortRange
			end = lastCheckedPort - 1
			result, lastCheckedPort, err = getHostPortRange(numberOfPorts, start, end, protocol)
		}
	}

	if lastCheckedPort == endHostPortRange {
		tracker.SetLastAssignedHostPort(startHostPortRange - 1)
	} else {
		tracker.SetLastAssignedHostPort(lastCheckedPort)
	}
	return result, err
}

func getHostPortRange(numberOfPorts, start, end int, protocol string) (string, int, error) {
	var resultStartPort, resultEndPort, n int
	for port := start; port <= end; port++ {
		isAvailable, err := isPortAvailableFunc(port, protocol)
		if !isAvailable || err != nil {
			// either port is unavailable or some error occurred while listening or closing the listener,
			// we proceed to the next port
			continue
		}
		// check if current port is contiguous relative to lastPort
		if port-resultEndPort != 1 {
			resultStartPort = port
			resultEndPort = port
			n = 1
		} else {
			resultEndPort = port
			n += 1
		}

		// we've got contiguous available ephemeral host ports to use, equal to the requested numberOfPorts
		if n == numberOfPorts {
			break
		}
	}

	if n != numberOfPorts {
		errMsg := fmt.Errorf(portNotFoundErrMsg)
		if numberOfPorts > 1 {
			errMsg = fmt.Errorf(portsNotFoundErrMsg, numberOfPorts)
		}
		return "", resultEndPort, errMsg
	}

	return fmt.Sprintf("%d-%d", resultStartPort, resultEndPort), resultEndPort, nil
}

// isPortAvailable checks if a port is available
func isPortAvailable(port int, protocol string) (bool, error) {
	portStr := strconv.Itoa(port)
	switch protocol {
	case "tcp":
		// net.Listen announces on the local tcp network
		ln, err := net.Listen(protocol, ":"+portStr)
		if err != nil {
			return false, err
		}
		// let's close the listener first
		err = ln.Close()
		if err != nil {
			return false, err
		}
		return true, nil
	case "udp":
		// net.ListenPacket announces on the local udp network
		ln, err := net.ListenPacket(protocol, ":"+portStr)
		if err != nil {
			return false, err
		}
		// let's close the listener first
		err = ln.Close()
		if err != nil {
			return false, err
		}
		return true, nil
	default:
		return false, errors.New("invalid protocol")
	}
}

// PortIsInRange returns true if the given port is within the start-end port range;
// otherwise, returns false.
func portIsInRange(port, start, end int) bool {
	if (port >= start) && (port <= end) {
		return true
	}
	return false
}

// VerifyPortsWithinRange returns true if the actualPortRange is within the expectedPortRange;
// otherwise, returns false.
func verifyPortsWithinRange(actualPortRange, expectedPortRange string) bool {
	// Get the actual start port and end port
	aStartPort, aEndPort, _ := nat.ParsePortRangeToInt(actualPortRange)
	// Get the expected start port and end port
	eStartPort, eEndPort, _ := nat.ParsePortRangeToInt(expectedPortRange)
	// Check the actual start port is in the expected range or not
	aStartIsInRange := portIsInRange(aStartPort, eStartPort, eEndPort)
	// Check the actual end port is in the expected range or not
	aEndIsInRange := portIsInRange(aEndPort, eStartPort, eEndPort)

	// Return true if both actual start port and end port are in the expected range
	if aStartIsInRange && aEndIsInRange {
		return true
	}

	return false
}
