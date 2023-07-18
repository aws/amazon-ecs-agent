//go:build unit
// +build unit

package utils

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

import (
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"

	"github.com/stretchr/testify/assert"
)

const (
	testTCPProtocol = "tcp"
	testUDPProtocol = "udp"
)

func TestGenerateEphemeralPortNumbers(t *testing.T) {
	// This number is just to "stress" this test by increasing the changes of collision, which should not be a problem
	// in prod, only around a dozen ports will be needed in the worst case.
	expectedPortsGenerated := 1000
	var reservedPorts []uint16
	rand.Seed(time.Now().UnixNano())
	// Since absolute max containers for a task is 20, let's exclude 40 ports at random, 2 per container
	// in order to make a somewhat realistic test
	for i := 0; i < 40; i++ {
		port := uint16(rand.Intn(EphemeralPortMax-EphemeralPortMin+1) + EphemeralPortMin)
		reservedPorts = append(reservedPorts, port)
	}
	toExcludeSet := map[uint16]struct{}{}
	for _, e := range reservedPorts {
		toExcludeSet[e] = struct{}{}
	}
	ports, err := GenerateEphemeralPortNumbers(expectedPortsGenerated, reservedPorts)
	assert.NoError(t, err)
	assert.Len(t, ports, expectedPortsGenerated, "Not enough ports generated")
	for _, port := range ports {
		_, ok := toExcludeSet[port]
		assert.False(t, ok, "Port collision detected")
		assert.Conditionf(t, func() (success bool) {
			return port >= EphemeralPortMin && port <= EphemeralPortMax
		}, "Port was generated outside the ephemeral range [%d-%d]", EphemeralPortMin, EphemeralPortMax)
	}
}

func TestGenerateEphemeralPortNumbers_CollisionError(t *testing.T) {
	randIntFuncTmp := randIntFunc
	defer func() {
		randIntFunc = randIntFuncTmp
	}()
	// Inject mock rand.Int that always returns the same number in order to test max attempts
	randIntFunc = func(n int) int {
		return EphemeralPortMin
	}
	ports, err := GenerateEphemeralPortNumbers(100, []uint16{})
	assert.Nil(t, ports)
	assert.Error(t, err)
	assert.Equal(t, "maximum number of attempts to generate unique ports reached", err.Error())
}

func TestGetHostPortRange(t *testing.T) {
	testCases := []struct {
		testName                 string
		numberOfPorts            int
		testDynamicHostPortRange string
		protocol                 string
		expectedLastAssignedPort []int
		isPortAvailableFunc      func(port int, protocol string) (bool, error)
		numberOfRequests         int
		expectedError            error
	}{
		{
			testName:                 "tcp protocol, contiguous hostPortRange found",
			numberOfPorts:            10,
			testDynamicHostPortRange: "40001-40080",
			protocol:                 testTCPProtocol,
			expectedLastAssignedPort: []int{40010},
			isPortAvailableFunc:      func(port int, protocol string) (bool, error) { return true, nil },
			numberOfRequests:         1,
			expectedError:            nil,
		},
		{
			testName:                 "udp protocol, contiguous hostPortRange found",
			numberOfPorts:            30,
			testDynamicHostPortRange: "40001-40080",
			protocol:                 testUDPProtocol,
			expectedLastAssignedPort: []int{40040},
			isPortAvailableFunc:      func(port int, protocol string) (bool, error) { return true, nil },
			numberOfRequests:         1,
			expectedError:            nil,
		},
		{
			testName:                 "2 requests for contiguous hostPortRange in succession, success",
			numberOfPorts:            20,
			testDynamicHostPortRange: "40001-40080",
			protocol:                 testTCPProtocol,
			expectedLastAssignedPort: []int{40060, 40000},
			isPortAvailableFunc:      func(port int, protocol string) (bool, error) { return true, nil },
			numberOfRequests:         2,
			expectedError:            nil,
		},
		{
			testName:                 "contiguous hostPortRange after looping back, success",
			numberOfPorts:            15,
			testDynamicHostPortRange: "40001-40080",
			protocol:                 testUDPProtocol,
			expectedLastAssignedPort: []int{40015},
			isPortAvailableFunc:      func(port int, protocol string) (bool, error) { return true, nil },
			numberOfRequests:         1,
			expectedError:            nil,
		},
		{
			testName:                 "contiguous hostPortRange not found, numberOfPorts more than available",
			numberOfPorts:            20,
			testDynamicHostPortRange: "40001-40005",
			protocol:                 testTCPProtocol,
			isPortAvailableFunc:      func(port int, protocol string) (bool, error) { return true, nil },
			numberOfRequests:         1,
			expectedError:            errors.New("20 contiguous host ports are unavailable"),
		},
		{
			testName:                 "contiguous hostPortRange not found, no ports available on the host",
			numberOfPorts:            5,
			testDynamicHostPortRange: "40001-40005",
			protocol:                 testTCPProtocol,
			isPortAvailableFunc:      func(port int, protocol string) (bool, error) { return false, nil },
			numberOfRequests:         1,
			expectedError:            errors.New("5 contiguous host ports are unavailable"),
		},
	}

	// mock isPortAvailable() for unit test
	// this ensures that the test doesn't rely on the runtime port availability on the host
	isPortAvailableFuncTmp := isPortAvailableFunc
	defer func() {
		isPortAvailableFunc = isPortAvailableFuncTmp
	}()

	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			for i := 0; i < tc.numberOfRequests; i++ {
				isPortAvailableFunc = tc.isPortAvailableFunc
				if tc.expectedError == nil {
					hostPortRange, err := GetHostPortRange(tc.numberOfPorts, tc.protocol, tc.testDynamicHostPortRange)
					assert.NoError(t, err)

					numberOfHostPorts, err := getPortRangeLength(hostPortRange)
					assert.NoError(t, err)
					assert.Equal(t, tc.numberOfPorts, numberOfHostPorts)

					actualLastAssignedHostPort := tracker.GetLastAssignedHostPort()
					assert.Equal(t, tc.expectedLastAssignedPort[i], actualLastAssignedHostPort)
				} else {
					// need to reset the tracker to avoid getting data from previous test cases
					ResetTracker()

					hostPortRange, err := GetHostPortRange(tc.numberOfPorts, tc.protocol, tc.testDynamicHostPortRange)
					assert.Equal(t, tc.expectedError, err)
					assert.Equal(t, "", hostPortRange)
				}
			}
		})
	}
}

func TestGetHostPort(t *testing.T) {
	numberOfPorts := 1
	testCases := []struct {
		testName                  string
		testDynamicHostPortRange  string
		protocol                  string
		numberOfRequests          int
		resetLastAssignedHostPort bool
	}{
		{
			testName:                  "tcp protocol, a host port found",
			testDynamicHostPortRange:  "40090-40099",
			protocol:                  testTCPProtocol,
			resetLastAssignedHostPort: true,
			numberOfRequests:          1,
		},
		{
			testName:                  "udp protocol, a host port found",
			testDynamicHostPortRange:  "40090-40099",
			protocol:                  testUDPProtocol,
			resetLastAssignedHostPort: true,
			numberOfRequests:          1,
		},
		{
			testName:                  "5 requests for host port in succession, success",
			testDynamicHostPortRange:  "50090-50099",
			protocol:                  testTCPProtocol,
			resetLastAssignedHostPort: true,
			numberOfRequests:          5,
		},
		{
			testName:                  "5 requests for host port in succession, success",
			testDynamicHostPortRange:  "50090-50099",
			protocol:                  testUDPProtocol,
			resetLastAssignedHostPort: false,
			numberOfRequests:          5,
		},
	}

	for _, tc := range testCases {
		if tc.resetLastAssignedHostPort {
			// need to reset the tracker to avoid getting data from previous test cases
			ResetTracker()
		}

		t.Run(tc.testName, func(t *testing.T) {
			for i := 0; i < tc.numberOfRequests; i++ {
				hostPortRange, err := GetHostPort(tc.protocol, tc.testDynamicHostPortRange)
				assert.NoError(t, err)
				numberOfHostPorts, err := getPortRangeLength(hostPortRange)
				assert.NoError(t, err)
				assert.Equal(t, numberOfPorts, numberOfHostPorts)
			}
		})
	}
}

func TestPortIsInRange(t *testing.T) {
	testCases := []struct {
		testName       string
		testPort       int
		testStartPort  int
		testEndPort    int
		expectedResult bool
	}{
		{
			testName:       "in the range",
			testPort:       100,
			testStartPort:  1,
			testEndPort:    9999,
			expectedResult: true,
		},
		{
			testName:       "not in the range",
			testPort:       10000,
			testStartPort:  1,
			testEndPort:    9999,
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			result := portIsInRange(tc.testPort, tc.testStartPort, tc.testEndPort)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestVerifyPortsWithinRange(t *testing.T) {
	testCases := []struct {
		testName          string
		testActualRange   string
		testExpectedRange string
		expectedResult    bool
	}{
		{
			testName:          "in the expected range",
			testActualRange:   "1000-1005",
			testExpectedRange: "900-2000",
			expectedResult:    true,
		},
		{
			testName:          "not in the expected range",
			testActualRange:   "1-2",
			testExpectedRange: "2-100",
			expectedResult:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			result := verifyPortsWithinRange(tc.testActualRange, tc.testExpectedRange)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestResetTracker(t *testing.T) {
	tracker.SetLastAssignedHostPort(100)
	ResetTracker()
	expectedResetVal := 0
	actualResult := tracker.GetLastAssignedHostPort()
	assert.Equal(t, expectedResetVal, actualResult)
}

func getPortRangeLength(portRange string) (int, error) {
	startPort, endPort, err := nat.ParsePortRangeToInt(portRange)
	if err != nil {
		return 0, err
	}
	return endPort - startPort + 1, nil
}
