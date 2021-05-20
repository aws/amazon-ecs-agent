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

//go:generate mockgen -destination=mocks/ecscni_mocks.go -copyright_file=../../scripts/copyright_file github.com/aws/amazon-ecs-agent/agent/ecscni CNIClient
//go:generate mockgen -destination=mocks/namespace_helper_mocks.go -copyright_file=../../scripts/copyright_file github.com/aws/amazon-ecs-agent/agent/ecscni NamespaceHelper
//go:generate mockgen -destination=mocks_libcni/libcni_mocks.go -copyright_file=../../scripts/copyright_file github.com/containernetworking/cni/libcni CNI
//go:generate mockgen -destination=mocks_cnitypes/result_mocks.go -copyright_file=../../scripts/copyright_file github.com/containernetworking/cni/pkg/types Result
