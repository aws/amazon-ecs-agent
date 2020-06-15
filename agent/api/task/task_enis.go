// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package task

import (
	"encoding/json"

	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
)

// TaskENIs type enumerates the list of ENI objects as a type. It is used for
// implementing a custom unmarshaler. The unmarshaler is capable of unmarshaling
// both a list of ENI objects into the TaskENIs type (the new scheme) or
// a single ENI object into the TaskENIs type (the old scheme before this object
// was introduced).
//
// The 'task' package is deemed to be a better home for this than the 'eni' package
// since this is only required for unmarshaling 'Task' object. None of the
// functionality/types in the 'eni' package themselves have any dependencies on this
// type.
type TaskENIs []*apieni.ENI

func (taskENIs *TaskENIs) UnmarshalJSON(b []byte) error {
	var enis []*apieni.ENI
	// Try to unmarshal this as a list of ENI objects.
	err := json.Unmarshal(b, &enis)
	if err == nil {
		// Successfully decoded the byte slice as a list of ENI objects, we're done.
		*taskENIs = enis
		return nil
	}
	// There was an error unmarshaling the byte slice as a list of ENIs. This is the case
	// where we're restoring from a previous version of the state file, where the ENI is
	// being stored as a standalone object and not as a list.
	var eni apieni.ENI
	err = json.Unmarshal(b, &eni)
	if err != nil {
		return err
	}
	enis = []*apieni.ENI{&eni}
	*taskENIs = enis
	return nil
}
