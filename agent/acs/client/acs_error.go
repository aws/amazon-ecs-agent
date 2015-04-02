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

import (
	"reflect"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
)

// ACSError wraps all the typed errors that ACS may return
// This will not be needed once the aws-sdk-go generation handles error types
// more cleanly
type acsError struct {
	errObj interface{}
}

// NewACSError returns an error corresponding to a typed error returned from
// ACS. It is expected that the passed in interface{} is really a struct which
// has a 'Message' field of type *string. In that case, the Message will be
// conveyed as part of the Error string as well as the type. It is safe to pass
// anything into this constructor and it will also work reasonably well with
// anything fulfilling the 'error' interface.
func NewACSError(err interface{}) *acsError {
	return &acsError{err}
}

// These errors are all fatal and there's nothing we can do about them.
// AccessDeniedException is actually potentially fixable because you can change
// credentials at runtime, but still close to unretriable.
var unretriableErrors = []interface{}{
	&ecsacs.InvalidInstanceException{},
	&ecsacs.InvalidClusterException{},
	&ecsacs.InactiveInstanceException{},
	&ecsacs.AccessDeniedException{},
}

// Error returns an error string
func (err *acsError) Error() string {
	val := reflect.ValueOf(err.errObj)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	var typeStr = "Unknown type"
	if val.IsValid() {
		typeStr = val.Type().Name()
		msg := val.FieldByName("Message")
		if msg.IsValid() && msg.CanInterface() {
			str, ok := msg.Interface().(*string)
			if ok {
				if str == nil {
					return typeStr + ": null"
				}
				return typeStr + ": " + *str
			}
		}
	}

	if asErr, ok := err.errObj.(error); ok {
		return "ACSError: " + asErr.Error()
	}
	return "ACSError: Unknown error (" + typeStr + ")"
}

// Retry returns true if this error should be considered retriable
func (err *acsError) Retry() bool {
	for _, unretriable := range unretriableErrors {
		if reflect.TypeOf(err.errObj) == reflect.TypeOf(unretriable) {
			return false
		}
	}
	return true
}
