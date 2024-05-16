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

package errors

import (
	"errors"
	"fmt"
	"regexp"
)

// This module provides functionality to extend error messages with extra information for
// known Docker client error types. It allows parsing error messages and augmenting them with
// additional details to provide more context to end user. This abstraction simplifies
// error handling and provides richer error messages to aid debugging and troubleshooting.
//
// How it Works:
// The module consists of three main components:
// 1. Error Type Enum: Defines types of Docker errors and associates each error type with a unique identifier.
// 2. Parsers: Functions responsible for parsing error messages and identifying known error types.
//    Each parser is associated with a specific error pattern and error type.
// 3. Error Message Formatters: Functions to generate rich error messages for each known error type.
//    These functions take parsed error data and additional arguments as input and produce augmented
//    error messages.
//
// Adding a New Error Type:
// To add a new error type, follow these steps:
// 1. Define a new constant in the DockerErrorType enum for the new error type.
// 2. Implement a parser function to identify the error pattern associated with the new error type.
//    Add the parser function in `AugmentMessage`.
// 3. Update `errorParsers` map to associate the new error type with its parser function.
// 4. Implement a formatter function to generate error messages for the new error type. The formatter
//    function should take parsed error data and additional arguments as input and produce an augmented
//    error message.
// 5. Update `errorMessageFunctions` map to associate the new error type with its formatter function.
//
// Usage:
// To augment an error message, call the AugmentMessage function with the original error message
// and additional arguments. If the error message matches a known error pattern, it will be augmented
// with extra information; otherwise, the original error message will be returned.
//
// Example:
//   augmentedMsg := apierrors.AugmentMessage("API error (404): repository not found", "execRole")
//

// DockerErrorType represents the type of error returned by Docker client.
type DockerErrorType int

const (
	MissingECRBatchGetImageError DockerErrorType = iota
	ECRImageDoesNotExistError
)

type DockerError struct {
	Type     DockerErrorType
	RawError string
}

// Defines the function signature for generating formatted error messages.
// 'args' can be used to pass extra information into formatting call.
type ErrorMessageFunc func(errorData DockerError, args ...string) string

// A map associating DockerErrorType with functions to form error messages.
var errorMessageFunctions = map[DockerErrorType]ErrorMessageFunc{
	MissingECRBatchGetImageError: formatMissingImageOrPullImageError,
	ECRImageDoesNotExistError:    formatMissingImageOrPullImageError,
}

// A map associating DockerErrorType with parser functions.
var errorParsers = map[DockerErrorType]func(string) (DockerError, error){
	MissingECRBatchGetImageError: parseMissingPullImagePermsError,
	ECRImageDoesNotExistError:    parseImageDoesNotExistError,
}

// An interface that provides means to reconstruct Named Errors. This is
// used during error message augmentation process.
type ConstructibleNamedError interface {
	NamedError
	Constructor() func(string) NamedError
}

const (
	// error message patterns used by parsers
	PatternECRBatchGetImageError  = `denied: User: (.+) is not authorized to perform: ecr:BatchGetImage`
	PatternImageDoesNotExistError = `denied: requested access to the resource is denied`
	// internal errors
	ErrorMessageDoesNotMatch = `ErrorMessageDoesNotMatch`
	NumOfArgsUnknownForErrTy = `NumOfArgsUnknownForErrTy`
)

// A series of CannotPullContainerError error message parsers. Example messages:
//
//	CannotPullContainerError: Error response from daemon: pull access denied for
//	123123123123.dkr.ecr.us-east-1.amazonaws.com/test_image, repository does not
//	exist or may require 'docker login': {details}
//	where "details" can be:
//	  * Missing pull image permissions:
//	    "denied: User: arn:aws:sts::123123123123:assumed-role/BrokenRole/xyz
//	    is not authorized to perform: ecr:BatchGetImage on resource:
//	    arn:aws:ecr:region:123123123123:repository/test_image
//	    because no identity-based policy allows the ecr:BatchGetImage action"
//	  * Image repository does not exist (or a type in repo URL):
//	    "denied: requested access to the resource is denied"
func parseMissingPullImagePermsError(err string) (DockerError, error) {
	matched, _ := regexp.MatchString(PatternECRBatchGetImageError, err)
	if matched {
		return DockerError{
			Type:     MissingECRBatchGetImageError,
			RawError: err,
		}, nil
	}
	return DockerError{RawError: err}, errors.New(ErrorMessageDoesNotMatch)
}

func parseImageDoesNotExistError(err string) (DockerError, error) {
	matched, _ := regexp.MatchString(PatternImageDoesNotExistError, err)
	if matched {
		return DockerError{
			Type:     ECRImageDoesNotExistError,
			RawError: err,
		}, nil
	}
	return DockerError{RawError: err}, errors.New(ErrorMessageDoesNotMatch)
}

func AugmentErrMsg(namedErr NamedError, args ...string) NamedError {

	constructibleErr, ok := namedErr.(ConstructibleNamedError)
	if !ok { // If namedErr is not a ConstructibleNamedError, return as-is.
		return namedErr
	}

	// Augment the error message.
	augmentedMsg := AugmentMessage(namedErr.Error(), args...)
	// Reconstruct new NamedError.
	newError := constructibleErr.Constructor()(augmentedMsg)
	return newError
}

// Extend error messages with extra useful information.
// Tries to find known cases by matching `errMsg` against implemented parsers for known errors.
// If the error message pattern is unknown, returns the original error message.
//
// Possible failure scenarios:
// 1. Missing parser implementation.
//
// This can only occur if a new ErrorType was added incorrectly due to implementation oversight.
//
// Currently, the function is set up in a safe manner - all failures are ignored, and
// the original error message string is returned.
func AugmentMessage(errMsg string, args ...string) string {
	var errorData DockerError
	var err error

	// Try parsing each error type until a match is found.
	for _, parser := range errorParsers {
		errorData, err = parser(errMsg)
		if err == nil {
			// Parser found a match, break out of loop
			break
		}
	}

	// none of the parsers match - return original untouched error message
	if err != nil {
		return errMsg
	}

	// lookup corresponding error formatter function. If not found - return original.
	formattedErrorMessage, ok := errorMessageFunctions[errorData.Type]
	if !ok {
		return errMsg
	}

	return formattedErrorMessage(errorData, args...)
}

// Generates error message for MissingECRBatchGetImage and ECRImageDoesNotExist errors.
func formatMissingImageOrPullImageError(errorData DockerError, args ...string) string {
	rawError := errorData.RawError
	roleName := ""

	if len(args) > 0 && args[0] != "" {
		roleName = "'" + args[0] + "' "
	}

	formattedMessage := fmt.Sprintf(
		"Check if image exists and role %shas permissions to pull images from Amazon ECR. %s",
		roleName, rawError)

	return formattedMessage
}
