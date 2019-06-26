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

package logrouter

import (
	"errors"
	"strings"

	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
)

type LogRouterStatus resourcestatus.ResourceStatus

const (
	// LogRouterStatusNone is the zero state of a logrouter task resource.
	LogRouterStatusNone LogRouterStatus = iota
	// LogRouterCreated represents the status of a logrouter task resource which has been created.
	LogRouterCreated
	// LogRouterRemoved represents the status of a logrouter task resource which has been cleaned up.
	LogRouterRemoved
)

var logRouterStatusMap = map[string]LogRouterStatus{
	"NONE":    LogRouterStatusNone,
	"CREATED": LogRouterCreated,
	"REMOVED": LogRouterRemoved,
}

// String returns a human readable string representation of this object.
func (as LogRouterStatus) String() string {
	for k, v := range logRouterStatusMap {
		if v == as {
			return k
		}
	}
	return "NONE"
}

// MarshalJSON overrides the logic for JSON-encoding the ResourceStatus type.
func (as *LogRouterStatus) MarshalJSON() ([]byte, error) {
	if as == nil {
		return nil, errors.New("logrouter resource status is nil")
	}
	return []byte(`"` + as.String() + `"`), nil
}

// UnmarshalJSON overrides the logic for parsing the JSON-encoded ResourceStatus data.
func (as *LogRouterStatus) UnmarshalJSON(b []byte) error {
	if strings.ToLower(string(b)) == "null" {
		*as = LogRouterStatusNone
		return nil
	}

	if b[0] != '"' || b[len(b)-1] != '"' {
		*as = LogRouterStatusNone
		return errors.New("resource status unmarshal: status must be a string or null; Got " + string(b))
	}

	strStatus := string(b[1 : len(b)-1])
	stat, ok := logRouterStatusMap[strStatus]
	if !ok {
		*as = LogRouterStatusNone
		return errors.New("resource status unmarshal: unrecognized status")
	}
	*as = stat
	return nil
}
