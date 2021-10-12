//go:build !linux && !windows

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

	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/containernetworking/cni/pkg/types/current"
)

// ConfigureTaskNamespaceRouting executes the commands required for setting up appropriate routing inside task namespace.
// This is applicable only for Windows.
func (nsHelper *helper) ConfigureTaskNamespaceRouting(ctx context.Context, taskENI *apieni.ENI, config *Config, result *current.Result) error {
	return nil
}
