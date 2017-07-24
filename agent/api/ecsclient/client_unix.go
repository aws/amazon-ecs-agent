// +build linux

// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package ecsclient

import "os"

// initPID defines the process identifier for the init process
const initPID = 1

// isAgentAlsoTheInit returns true if the Agent process's pid is
// 1, which means it's running without an init system
func isAgentAlsoTheInit() (bool, error) {
	return os.Getpid() == initPID, nil
}
