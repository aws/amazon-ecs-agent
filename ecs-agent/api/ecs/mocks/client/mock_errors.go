// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

// Package mock_client provides mock error implementations for ECS client testing
package mock_client

import (
	"github.com/aws/aws-sdk-go-v2/aws/ratelimit"
	"github.com/aws/smithy-go"
)

// MockAPIError creates a mock error that implements smithy.APIError interface
type MockAPIError struct {
	code    string
	message string
}

// MockQuotaExceededError creates a mock error that implements ratelimit.QuotaExceededError interface
type MockQuotaExceededError struct {
	Available uint
	Requested uint
}

func (e *MockAPIError) Error() string {
	return e.message
}

func (e *MockAPIError) ErrorCode() string {
	return e.code
}

func (e *MockAPIError) ErrorMessage() string {
	return e.message
}

func (e *MockAPIError) ErrorFault() smithy.ErrorFault {
	return smithy.FaultUnknown
}

func (e *MockQuotaExceededError) Error() string {
	return "retry quota exceeded"
}

func NewThrottlingException() error {
	return &MockAPIError{code: "ThrottlingException", message: "Request was throttled"}
}

func NewServerException() error {
	return &MockAPIError{code: "ServerException", message: "Internal server error"}
}

func NewLimitExceededException() error {
	return &MockAPIError{code: "LimitExceededException", message: "Limit exceeded"}
}

func NewInvalidParameterException() error {
	return &MockAPIError{code: "InvalidParameterException", message: "Invalid parameter"}
}

func NewClientException() error {
	return &MockAPIError{code: "ClientException", message: "Client error"}
}

func NewAccessDeniedException() error {
	return &MockAPIError{code: "AccessDeniedException", message: "Access denied"}
}

func NewQuotaExceededError() error {
	return ratelimit.QuotaExceededError{Available: 0, Requested: 1}
}
