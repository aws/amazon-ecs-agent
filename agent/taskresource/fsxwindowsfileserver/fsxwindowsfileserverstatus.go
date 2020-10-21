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

package fsxwindowsfileserver

import (
	"errors"
	"strings"

	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
)

// FSxWindowsFileServerVolumeStatus defines resource statuses for fsxwindowsfileserver resource
type FSxWindowsFileServerVolumeStatus resourcestatus.ResourceStatus

const (
	// FSxWindowsFileServerVolumeStatusNone is the zero state of a task resource
	FSxWindowsFileServerVolumeStatusNone FSxWindowsFileServerVolumeStatus = iota
	// FSxWindowsFileServerVolumeCreated represents a task resource which has been created
	FSxWindowsFileServerVolumeCreated
	// FSxWindowsFileServerVolumeRemoved represents a task resource which has been cleaned up
	FSxWindowsFileServerVolumeRemoved
)

var FSxWindowsFileServerVolumeStatusMap = map[string]FSxWindowsFileServerVolumeStatus{
	"NONE":    FSxWindowsFileServerVolumeStatusNone,
	"CREATED": FSxWindowsFileServerVolumeCreated,
	"REMOVED": FSxWindowsFileServerVolumeRemoved,
}

// StatusString returns a human readable string representation of this object
func (fs FSxWindowsFileServerVolumeStatus) String() string {
	for k, v := range FSxWindowsFileServerVolumeStatusMap {
		if v == fs {
			return k
		}
	}
	return "NONE"
}

// MarshalJSON overrides the logic for JSON-encoding the ResourceStatus type
func (fs *FSxWindowsFileServerVolumeStatus) MarshalJSON() ([]byte, error) {
	if fs == nil {
		return nil, nil
	}
	return []byte(`"` + fs.String() + `"`), nil
}

// UnmarshalJSON overrides the logic for parsing the JSON-encoded ResourceStatus data
func (fs *FSxWindowsFileServerVolumeStatus) UnmarshalJSON(b []byte) error {
	if strings.ToLower(string(b)) == "null" {
		*fs = FSxWindowsFileServerVolumeStatusNone
		return nil
	}

	if b[0] != '"' || b[len(b)-1] != '"' {
		*fs = FSxWindowsFileServerVolumeStatusNone
		return errors.New("resource status unmarshal: status must be a string or null; Got " + string(b))
	}

	strStatus := b[1 : len(b)-1]
	stat, ok := FSxWindowsFileServerVolumeStatusMap[string(strStatus)]
	if !ok {
		*fs = FSxWindowsFileServerVolumeStatusNone
		return errors.New("resource status unmarshal: unrecognized status")
	}
	*fs = stat
	return nil
}
