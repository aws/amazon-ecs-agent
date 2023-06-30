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

package volume

import "fmt"

const (
	// ErrCodeNotSupported code for NotSupported Errors.
	ErrCodeNotSupported int = iota + 1
	ErrCodeNoPathDefined
	ErrCodeFsInfoFailed
)

// MetricsError to distinguish different Metrics Errors.
type MetricsError struct {
	Code int
	Msg  string
}

// NewNoPathDefinedError creates a new MetricsError with code NoPathDefined.
func NewNoPathDefinedError() *MetricsError {
	return &MetricsError{
		Code: ErrCodeNoPathDefined,
		Msg:  "no path defined for disk usage metrics.",
	}
}

// NewFsInfoFailedError creates a new MetricsError with code FsInfoFailed.
func NewFsInfoFailedError(err error) *MetricsError {
	return &MetricsError{
		Code: ErrCodeFsInfoFailed,
		Msg:  fmt.Sprintf("failed to get FsInfo due to error %v", err),
	}
}

func (e *MetricsError) Error() string {
	return e.Msg
}
