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

package envFiles

import (
	"errors"
	"strings"

	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
)

type EnvironmentFileStatus resourcestatus.ResourceStatus

const (
	// EnvFileStatusNone is the zero state of a task resource
	EnvFileStatusNone EnvironmentFileStatus = iota
	// EnvFileCreated means the task resource is created
	EnvFileCreated
	// EnvFileRemoved means the task resource is cleaned up
	EnvFileRemoved
)

var envfileStatusMap = map[string]EnvironmentFileStatus{
	"NONE":    EnvFileStatusNone,
	"CREATED": EnvFileCreated,
	"REMOVED": EnvFileRemoved,
}

func (envfileStatus EnvironmentFileStatus) String() string {
	for k, v := range envfileStatusMap {
		if v == envfileStatus {
			return k
		}
	}
	return "NONE"
}

// MarshalJSON overrides the logic for JSON-encoding the ResourceStatus type.
func (envfileStatus *EnvironmentFileStatus) MarshalJSON() ([]byte, error) {
	if envfileStatus == nil {
		return nil, errors.New("envfile resource status is nil")
	}
	return []byte(`"` + envfileStatus.String() + `"`), nil
}

// UnmarshalJSON overrides the logic for parsing the JSON-encoded ResourceStatus data.
func (envfileStatus *EnvironmentFileStatus) UnmarshalJSON(b []byte) error {
	if strings.ToLower(string(b)) == "null" {
		*envfileStatus = EnvFileStatusNone
		return nil
	}

	if b[0] != '"' || b[len(b)-1] != '"' {
		*envfileStatus = EnvFileStatusNone
		return errors.New("resource status unmarshal: status must be a string or null; Got " + string(b))
	}

	strStatus := b[1 : len(b)-1]
	stat, ok := envfileStatusMap[string(strStatus)]
	if !ok {
		*envfileStatus = EnvFileStatusNone
		return errors.New("resource status unmarshal: unrecognized status")
	}
	*envfileStatus = stat
	return nil
}
