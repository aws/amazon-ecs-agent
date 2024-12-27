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

package app

type terminalError struct {
	error
}

// isTerminal returns true if the error is already wrapped as an unrecoverable condition
// which will allow agent to exit terminally.
func isTerminal(err error) bool {
	// Check if the error is already wrapped as a terminalError
	_, terminal := err.(terminalError)
	return terminal
}

// clusterMismatchError represents a mismatch in cluster name between the
// state file and the config object
type clusterMismatchError struct {
	error
}
