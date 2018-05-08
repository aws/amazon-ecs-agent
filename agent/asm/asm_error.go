// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package asm

// ASMAuthDataError is used to encapsulate the error returned...
type ASMAuthDataError struct {
	FromError error
}

// Error returns the error string
func (err ASMAuthDataError) Error() string {
	return err.FromError.Error()
}

// Retry fulfills the utils.Retrier interface and allows retries to be skipped by utils.Retry* functions
func (err ASMAuthDataError) Retry() bool {
	return false
}

// ErrorName returns name of the ASMAuthDataError.
func (err ASMAuthDataError) ErrorName() string {
	return "SecretsManagerRegistryAuthenticationError"
}
