// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package acsclient

// UnrecognizedACSRequestType specifies that a given type is not recognized.
// This error is not retriable.
type UnrecognizedACSRequestType struct {
	Type string
}

// Error implements error
func (u *UnrecognizedACSRequestType) Error() string {
	return "Could not recognize given argument as a valid type for ACS: " + u.Type
}

// Retry implements Retriable
func (u *UnrecognizedACSRequestType) Retry() bool {
	return false
}

// NotMarshallableACSRequest represents that the given request input could not be
// marshalled
type NotMarshallableACSRequest struct {
	Type string

	err error
}

// Retry implementes Retriable
func (u *NotMarshallableACSRequest) Retry() bool {
	return false
}

// Error implements error
func (u *NotMarshallableACSRequest) Error() string {
	ret := "Could not marshal ACS Request"
	if u.Type != "" {
		ret += " (" + u.Type + ")"
	}
	return ret + ": " + u.err.Error()
}

// UndecodableMessage indicates that a message from ACS could not be decoded
type UndecodableMessage struct {
	msg string
}

func (u *UndecodableMessage) Error() string {
	return "Could not decode ACS message into any expected format: " + u.msg
}
