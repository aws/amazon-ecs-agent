// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

// nonTerminalError represents a transient error when executing the ECS Agent
type nonTerminalError struct {
	error
}

// isNonTerminal returns true if the error is transient
func isNonTerminal(err error) bool {
	_, ok := err.(nonTerminalError)
	return ok
}
