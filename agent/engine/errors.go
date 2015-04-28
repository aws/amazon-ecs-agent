// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

import "github.com/aws/amazon-ecs-agent/agent/api"

// impossibleTransitionError is an error that occurs when an event causes a
// container to try and transition to a state that it cannot be moved to
type impossibleTransitionError struct {
	state api.ContainerStatus
}

func (err *impossibleTransitionError) Error() string {
	return "Cannot transition to " + err.state.String()
}
func (err *impossibleTransitionError) Name() string { return "ImpossibleStateTransitionError" }
