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

package asmsecret

import (
	"errors"
	"strings"

	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
)

type ASMSecretStatus resourcestatus.ResourceStatus

const (
	// is the zero state of a task resource
	ASMSecretStatusNone ASMSecretStatus = iota
	// represents a task resource which has been created
	ASMSecretCreated
	// represents a task resource which has been cleaned up
	ASMSecretRemoved
)

var asmSecretStatusMap = map[string]ASMSecretStatus{
	"NONE":    ASMSecretStatusNone,
	"CREATED": ASMSecretCreated,
	"REMOVED": ASMSecretRemoved,
}

// StatusString returns a human readable string representation of this object
func (as ASMSecretStatus) String() string {
	for k, v := range asmSecretStatusMap {
		if v == as {
			return k
		}
	}
	return "NONE"
}

// MarshalJSON overrides the logic for JSON-encoding the ResourceStatus type
func (as *ASMSecretStatus) MarshalJSON() ([]byte, error) {
	if as == nil {
		return nil, errors.New("asmsecret resource status is nil")
	}
	return []byte(`"` + as.String() + `"`), nil
}

// UnmarshalJSON overrides the logic for parsing the JSON-encoded ResourceStatus data
func (as *ASMSecretStatus) UnmarshalJSON(b []byte) error {
	if strings.ToLower(string(b)) == "null" {
		*as = ASMSecretStatusNone
		return nil
	}

	if b[0] != '"' || b[len(b)-1] != '"' {
		*as = ASMSecretStatusNone
		return errors.New("resource status unmarshal: status must be a string or null; Got " + string(b))
	}

	strStatus := string(b[1 : len(b)-1])
	stat, ok := asmSecretStatusMap[strStatus]
	if !ok {
		*as = ASMSecretStatusNone
		return errors.New("resource status unmarshal: unrecognized status")
	}
	*as = stat
	return nil
}
