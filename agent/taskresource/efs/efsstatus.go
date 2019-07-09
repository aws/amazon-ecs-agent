// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package efs

import (
	"strings"

	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/pkg/errors"
)

type EFSStatus resourcestatus.ResourceStatus

const (
	// is the zero state of a task resource
	EFSStatusNone EFSStatus = iota
	// represents a task resource which has been created
	EFSCreated
	// represents a task resource which has been cleaned up
	EFSRemoved
)

var efsStatusMap = map[string]EFSStatus{
	"NONE":    EFSStatusNone,
	"CREATED": EFSCreated,
	"REMOVED": EFSRemoved,
}

// StatusString returns a human readable string representation of this object
func (as EFSStatus) String() string {
	for k, v := range efsStatusMap {
		if v == as {
			return k
		}
	}
	return "NONE"
}

// MarshalJSON overrides the logic for JSON-encoding the ResourceStatus type
func (as *EFSStatus) MarshalJSON() ([]byte, error) {
	if as == nil {
		return nil, errors.New("efs resource status is nil")
	}
	return []byte(`"` + as.String() + `"`), nil
}

// UnmarshalJSON overrides the logic for parsing the JSON-encoded ResourceStatus data
func (as *EFSStatus) UnmarshalJSON(b []byte) error {
	if strings.ToLower(string(b)) == "null" {
		*as = EFSStatusNone
		return nil
	}

	if b[0] != '"' || b[len(b)-1] != '"' {
		*as = EFSStatusNone
		return errors.New("resource status unmarshal: status must be a string or null; Got " + string(b))
	}

	strStatus := string(b[1 : len(b)-1])
	stat, ok := efsStatusMap[strStatus]
	if !ok {
		*as = EFSStatusNone
		return errors.New("resource status unmarshal: unrecognized status")
	}
	*as = stat
	return nil
}
