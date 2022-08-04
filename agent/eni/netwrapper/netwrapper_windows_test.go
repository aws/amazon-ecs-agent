//go:build windows
// +build windows

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

package netwrapper

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	dummyIfAlias              = "Ethernet 2"
	dummyIfIndex              = 9
	dummyMAC                  = "06-34-BB-C8-64-C5"
	networkAdapterValidResult = `[
  {
    "ifAlias": "%s",
    "ifIndex": %d,
    "MacAddress": "%s",
    "ActiveMaximumTransmissionUnit": 1500
  }
]`
	networkAdapterExpectedValidResult []net.Interface
	networkAdapterInvalidResult       = ""
)

func init() {
	networkAdapterValidResult = fmt.Sprintf(networkAdapterValidResult, dummyIfAlias, dummyIfIndex, dummyMAC)

	parsedMAC, _ := net.ParseMAC(dummyMAC)
	networkAdapterExpectedValidResult = []net.Interface{
		{
			Index:        dummyIfIndex,
			MTU:          1500,
			Name:         dummyIfAlias,
			HardwareAddr: parsedMAC,
		},
	}

}

// Supporting methods for testing getAllNetworkInterfaces method.
// mockExecCommandForNetworkAdaptersSuccess returns a valid output from exec cmd.
func mockExecCommandForNetworkAdaptersSuccess(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestNetworkAdaptersProcessSuccess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

// TestNetworkAdaptersProcessSuccess is invoked from mockExecCommandForNetworkAdaptersSuccess
// to return the network adapter information.
func TestNetworkAdaptersProcessSuccess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprintf(os.Stdout, networkAdapterValidResult)
	os.Exit(0)
}

// mockExecCommandForNetworkAdaptersInvalidOutput returns an invalid output from exec cmd.
func mockExecCommandForNetworkAdaptersInvalidOutput(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestNetworkAdaptersProcessInvalidOutput", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

// TestNetworkAdaptersProcessInvalidOutput is invoked from mockExecCommandForNetworkAdaptersInvalidOutput
// to return invalid network adapter information.
func TestNetworkAdaptersProcessInvalidOutput(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprintf(os.Stdout, networkAdapterInvalidResult)
	os.Exit(0)
}

// mockExecCommandForNetworkAdaptersCommandError returns an error during command execution.
func mockExecCommandForNetworkAdaptersCommandError(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestNetworkAdaptersProcessCommandError", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

// TestNetworkAdaptersProcessCommandError is invoked from mockExecCommandForNetworkAdaptersCommandError
// to return an error during command execution.
func TestNetworkAdaptersProcessCommandError(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprintf(os.Stderr, "error during execution")
	os.Exit(1)
}

func TestGetAllWindowsNetworkInterfaces(t *testing.T) {
	var tests = []struct {
		name                string
		execCmd             func(string, ...string) *exec.Cmd
		expectedOutput      []net.Interface
		expectedErrorString string
	}{
		{
			name:                "TestGetAllWindowsNetworkInterfacesSuccess",
			execCmd:             mockExecCommandForNetworkAdaptersSuccess,
			expectedOutput:      networkAdapterExpectedValidResult,
			expectedErrorString: "",
		},
		{
			name:                "TestGetAllWindowsNetworkInterfacesInvalidOutput",
			execCmd:             mockExecCommandForNetworkAdaptersInvalidOutput,
			expectedOutput:      nil,
			expectedErrorString: "failed to unmarshal the json of network adapters: unexpected end of JSON input",
		},
		{
			name:                "TestGetAllWindowsNetworkInterfacesCommandError",
			execCmd:             mockExecCommandForNetworkAdaptersCommandError,
			expectedOutput:      nil,
			expectedErrorString: "failed to execute Get-NetAdapter for retrieving all interfaces: exit status 1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			execCommand = test.execCmd
			out, err := New().GetAllNetworkInterfaces()
			assert.Equal(t, test.expectedOutput, out)
			if test.expectedErrorString != "" {
				assert.EqualError(t, err, test.expectedErrorString)
			}
		})
	}
}
