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
	"sync"
	"time"

	"github.com/cihub/seelog"
)

// From https://www.kernel.org/doc/html/latest//networking/ip-sysctl.html#ip-variables
const (
	EphemeralPortMin         = 32768
	EphemeralPortMax         = 60999
	maxPortSelectionAttempts = 100
)

var (
	// Injection point for UTs
	randIntFunc = rand.Intn
	// portLock is a mutex lock used to prevent two concurrent tasks to get the same host ports.
	// TODO: implement a port manager that tracks last assigned host port
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

var dynamicHostPortRange = getDynamicHostPortRange

// GetHostPortRange gets N contiguous host ports from the ephemeral host port range defined on the host.
func GetHostPortRange(numberOfPorts int, protocol string) (string, error) {
	portLock.Lock()
	defer portLock.Unlock()

	// get ephemeral port range, either default or if custom-defined
	startHostPortRange, endHostPortRange, err := dynamicHostPortRange()
	if err != nil {
		seelog.Warnf("Unable to read the ephemeral host port range, falling back to the default range: %v-%v",
			defaultPortRangeStart, defaultPortRangeEnd)
		startHostPortRange, endHostPortRange = defaultPortRangeStart, defaultPortRangeEnd
	}

	var startPort, lastPort, n int
	for port := startHostPortRange; port <= endHostPortRange; port++ {
		portStr := strconv.Itoa(port)
		// check if port is available
		if protocol == "tcp" {
			// net.Listen announces on the local tcp network
			ln, err := net.Listen(protocol, ":"+portStr)
			// either port is unavailable or some error occurred while listening, we proceed to the next port
			if err != nil {
				continue
			}
			// let's close the listener first
			err = ln.Close()
			if err != nil {
				continue
			}
		} else if protocol == "udp" {
			// net.ListenPacket announces on the local udp network
			ln, err := net.ListenPacket(protocol, ":"+portStr)
			// either port is unavailable or some error occurred while listening, we proceed to the next port
			if err != nil {
				continue
			}
			// let's close the listener first
			err = ln.Close()
			if err != nil {
				continue
			}
		}

		// check if current port is contiguous relative to lastPort
		if port-lastPort != 1 {
			startPort = port
			lastPort = port
			n = 1
		} else {
			lastPort = port
			n += 1
		}

		// we've got contiguous available ephemeral host ports to use, equal to the requested numberOfPorts
		if n == numberOfPorts {
			break
		}
	}
	if n != numberOfPorts {
		return "", fmt.Errorf("%v contiguous host ports unavailable", numberOfPorts)
	}
	return strconv.Itoa(startPort) + "-" + strconv.Itoa(lastPort), nil
}
