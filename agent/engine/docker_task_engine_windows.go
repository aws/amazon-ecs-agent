// +build windows
// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package engine

// isParallelPullCompatible always returns true for windows as the Docker
// version with the fix for this (1.9.0) predates 1.12.0, in which windows
// support was added.
func (engine *DockerTaskEngine) isParallelPullCompatible() bool {
	return true
}
