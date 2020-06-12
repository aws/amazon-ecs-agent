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

package volume

import (
	"errors"
	"strings"

	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
)

// VolumeStatus defines resource statuses for docker volume
type VolumeStatus resourcestatus.ResourceStatus

const (
	// VolumeStatusNone is the zero state of a task resource
	VolumeStatusNone VolumeStatus = iota
	// VolumeCreated represents a task resource which has been created
	VolumeCreated
	// VolumeRemoved represents a task resource which has been Removed
	VolumeRemoved
)

var resourceStatusMap = map[string]VolumeStatus{
	"NONE":    VolumeStatusNone,
	"CREATED": VolumeCreated,
	"REMOVED": VolumeRemoved,
}

// StatusString returns a human readable string representation of this object
func (vs VolumeStatus) String() string {
	for k, v := range resourceStatusMap {
		if v == vs {
			return k
		}
	}
	return "NONE"
}

// MarshalJSON overrides the logic for JSON-encoding the ResourceStatus type
func (vs *VolumeStatus) MarshalJSON() ([]byte, error) {
	if vs == nil {
		return nil, nil
	}
	return []byte(`"` + vs.String() + `"`), nil
}

// UnmarshalJSON overrides the logic for parsing the JSON-encoded ResourceStatus data
func (vs *VolumeStatus) UnmarshalJSON(b []byte) error {
	if strings.ToLower(string(b)) == "null" {
		*vs = VolumeStatusNone
		return nil
	}

	if b[0] != '"' || b[len(b)-1] != '"' {
		*vs = VolumeStatusNone
		return errors.New("resource status unmarshal: status must be a string or null; Got " + string(b))
	}

	strStatus := b[1 : len(b)-1]
	stat, ok := resourceStatusMap[string(strStatus)]
	if !ok {
		*vs = VolumeStatusNone
		return errors.New("resource status unmarshal: unrecognized status")
	}
	*vs = stat
	return nil
}
