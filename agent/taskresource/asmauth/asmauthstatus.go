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

package asmauth

import (
	"errors"
	"strings"

	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
)

type ASMAuthStatus resourcestatus.ResourceStatus

const (
	// is the zero state of a task resource
	ASMAuthStatusNone ASMAuthStatus = iota
	// represents a task resource which has been created
	ASMAuthStatusCreated
	// represents a task resource which has been cleaned up
	ASMAuthStatusRemoved
)

var asmAuthStatusMap = map[string]ASMAuthStatus{
	"NONE":    ASMAuthStatusNone,
	"CREATED": ASMAuthStatusCreated,
	"REMOVED": ASMAuthStatusRemoved,
}

// StatusString returns a human readable string representation of this object
func (as ASMAuthStatus) String() string {
	for k, v := range asmAuthStatusMap {
		if v == as {
			return k
		}
	}
	return "NONE"
}

// MarshalJSON overrides the logic for JSON-encoding the ResourceStatus type
func (as *ASMAuthStatus) MarshalJSON() ([]byte, error) {
	if as == nil {
		return nil, errors.New("asm-auth resource status is nil")
	}
	return []byte(`"` + as.String() + `"`), nil
}

// UnmarshalJSON overrides the logic for parsing the JSON-encoded ResourceStatus data
func (as *ASMAuthStatus) UnmarshalJSON(b []byte) error {
	if strings.ToLower(string(b)) == "null" {
		*as = ASMAuthStatusNone
		return nil
	}

	if b[0] != '"' || b[len(b)-1] != '"' {
		*as = ASMAuthStatusNone
		return errors.New("resource status unmarshal: status must be a string or null; Got " + string(b))
	}

	strStatus := b[1 : len(b)-1]
	stat, ok := asmAuthStatusMap[string(strStatus)]
	if !ok {
		*as = ASMAuthStatusNone
		return errors.New("resource status unmarshal: unrecognized status")
	}
	*as = stat
	return nil
}
