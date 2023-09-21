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

package ecscni

import (
	"io"

	"github.com/containernetworking/cni/pkg/types"
)

const (
	PluginName = "testPlugin"
	CNIVersion = "testVersion"
	NetNS      = "testNetNS"
	IfName     = "testIfName"
)

type TestCNIConfig struct {
	CNIConfig
	NetworkInterfaceName string
}

func (tc *TestCNIConfig) InterfaceName() string {
	return tc.NetworkInterfaceName
}

func (tc *TestCNIConfig) NSPath() string {
	return tc.NetNSPath
}

func (tc *TestCNIConfig) PluginName() string {
	return tc.CNIPluginName
}

func (tc *TestCNIConfig) CNIVersion() string {
	return tc.CNISpecVersion
}

type TestResult struct {
	msg string
}

func (tr *TestResult) Version() string {
	return CNIVersion
}

func (tr *TestResult) GetAsVersion(version string) (types.Result, error) {
	return &TestResult{msg: version}, nil
}

func (tr *TestResult) Print() error {
	return nil
}

func (tr *TestResult) PrintTo(writer io.Writer) error {
	_, err := writer.Write([]byte(tr.msg))
	return err
}
