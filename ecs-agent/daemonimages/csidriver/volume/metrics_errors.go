// this file has been modified from its original found in:
// https://github.com/kubernetes-sigs/aws-ebs-csi-driver

/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
