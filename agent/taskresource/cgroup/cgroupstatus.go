// +build linux
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

package cgroup

import (
	"errors"
	"strings"

	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
)

// CgroupStatus defines resource statuses for cgroups
type CgroupStatus resourcestatus.ResourceStatus

const (
	// CgroupStatusNone is the zero state of a task resource
	CgroupStatusNone CgroupStatus = iota
	// CgroupCreated represents a task resource which has been created
	CgroupCreated
	// CgroupRemoved represents a task resource which has been cleaned up
	CgroupRemoved
)

var cgroupStatusMap = map[string]CgroupStatus{
	"NONE":    CgroupStatusNone,
	"CREATED": CgroupCreated,
	"REMOVED": CgroupRemoved,
}

// String returns a human readable string representation of this object
func (cs CgroupStatus) String() string {
	for k, v := range cgroupStatusMap {
		if v == cs {
			return k
		}
	}
	return "NONE"
}

// MarshalJSON overrides the logic for JSON-encoding the ResourceStatus type
func (cs *CgroupStatus) MarshalJSON() ([]byte, error) {
	if cs == nil {
		return nil, nil
	}
	return []byte(`"` + cs.String() + `"`), nil
}

// UnmarshalJSON overrides the logic for parsing the JSON-encoded ResourceStatus data
func (cs *CgroupStatus) UnmarshalJSON(b []byte) error {
	if strings.ToLower(string(b)) == "null" {
		*cs = CgroupStatusNone
		return nil
	}

	if b[0] != '"' || b[len(b)-1] != '"' {
		*cs = CgroupStatusNone
		return errors.New("resource status unmarshal: status must be a string or null; Got " + string(b))
	}

	strStatus := b[1 : len(b)-1]
	stat, ok := cgroupStatusMap[string(strStatus)]
	if !ok {
		*cs = CgroupStatusNone
		return errors.New("resource status unmarshal: unrecognized status")
	}
	*cs = stat
	return nil
}
