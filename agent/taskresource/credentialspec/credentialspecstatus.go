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

package credentialspec

import (
	"errors"
	"strings"

	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
)

type CredentialSpecStatus resourcestatus.ResourceStatus

const (
	// is the zero state of a task resource
	CredentialSpecStatusNone CredentialSpecStatus = iota
	// represents a task resource which has been created
	CredentialSpecCreated
	// represents a task resource which has been cleaned up
	CredentialSpecRemoved
)

var CredentialSpecStatusMap = map[string]CredentialSpecStatus{
	"NONE":    CredentialSpecStatusNone,
	"CREATED": CredentialSpecCreated,
	"REMOVED": CredentialSpecRemoved,
}

// StatusString returns a human readable string representation of this object
func (cs CredentialSpecStatus) String() string {
	for k, v := range CredentialSpecStatusMap {
		if v == cs {
			return k
		}
	}
	return "NONE"
}

// MarshalJSON overrides the logic for JSON-encoding the ResourceStatus type
func (cs *CredentialSpecStatus) MarshalJSON() ([]byte, error) {
	if cs == nil {
		return nil, errors.New("credentialspec resource status is nil")
	}
	return []byte(`"` + cs.String() + `"`), nil
}

// UnmarshalJSON overrides the logic for parsing the JSON-encoded ResourceStatus data
func (cs *CredentialSpecStatus) UnmarshalJSON(b []byte) error {
	if strings.ToLower(string(b)) == "null" {
		*cs = CredentialSpecStatusNone
		return nil
	}

	if b[0] != '"' || b[len(b)-1] != '"' {
		*cs = CredentialSpecStatusNone
		return errors.New("resource status unmarshal: status must be a string or null; Got " + string(b))
	}

	strStatus := b[1 : len(b)-1]
	stat, ok := CredentialSpecStatusMap[string(strStatus)]
	if !ok {
		*cs = CredentialSpecStatusNone
		return errors.New("resource status unmarshal: unrecognized status")
	}
	*cs = stat
	return nil
}
