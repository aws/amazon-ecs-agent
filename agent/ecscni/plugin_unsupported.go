//go:build !linux && !windows
// +build !linux,!windows

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
	"context"
	"errors"
	"time"

	"github.com/containernetworking/cni/pkg/types/current"
)

const (
	// vpcCNIPluginPath is the path of the cni plugin's log file.
	vpcCNIPluginPath = "/log/vpc-branch-eni.log"
)

// newCNIGuard returns a new instance of CNI guard for the CNI client.
// It is supported only on Windows.
func newCNIGuard() cniGuard {
	return &guard{
		mutex: nil,
	}
}

type cniPluginVersion struct{}

// setupNS is the called by SetupNS to setup the task namespace by invoking ADD for given CNI configurations
// On unsupported platforms, we will return an error
func (client *cniClient) setupNS(ctx context.Context, cfg *Config) (*current.Result, error) {
	return nil, errors.New("unsupported platform")
}

// ReleaseIPResource marks the ip available in the ipam db
// On unsupported platforms, we will return an error
func (client *cniClient) ReleaseIPResource(ctx context.Context, cfg *Config, timeout time.Duration) error {
	return errors.New("unsupported platform")
}

// str generates a string version of the CNI plugin version
// On unsupported platforms, we will return an empty string
func (version *cniPluginVersion) str() string {
	return ""
}
