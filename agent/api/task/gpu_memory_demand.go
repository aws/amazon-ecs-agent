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

package task

// GPUMemoryDemand is a task's per-GPU memory claim, keyed by GPU UUID.
type GPUMemoryDemand struct {
	// MiB is the memory a task's MPS containers reserve on the GPU (the sum of
	// their per-container caps). Meaningful only when WholeGPU is false.
	MiB int64
	// WholeGPU is true when a non-MPS container claims the GPU exclusively.
	WholeGPU bool
}
