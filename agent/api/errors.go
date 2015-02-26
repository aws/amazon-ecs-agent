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

package api

import "github.com/awslabs/aws-sdk-go/aws"

// Implements Error & Retriable
type APIError struct {
	err       error
	Retriable bool
}

func NewAPIError(err error) *APIError {
	if apierr, ok := err.(aws.APIError); ok {
		// ClientExceptions are not retriable
		if apierr.Code == "ClientException" {
			return &APIError{err, false}
		}
	}

	return &APIError{err, true}
}

func (apiErr *APIError) Retry() bool {
	return apiErr.Retriable
}

func (apiErr *APIError) Error() string {
	return apiErr.err.Error()
}
