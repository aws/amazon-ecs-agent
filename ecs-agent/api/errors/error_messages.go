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
// 6. If necessary, define validation details in the numOfArgsForFmtErrMsg function to assert the correct
//    number of passed arguments for formatting the error message.
//
// Usage:
// To augment an error message, call the AugmentMessage function with the original error message
// and additional arguments. If the error message matches a known error pattern, it will be augmented
// with extra information; otherwise, the original error message will be returned.
//
// Example:
//   augmentedMsg := apierrors.AugmentMessage("API error (404): repository not found", "execRole")
//

// DockerErrorType represents the type of Docker API error.
type DockerErrorType int

const (
	APIError404RepoNotFound DockerErrorType = iota
	// Add more error types as needed
)

type DockerError struct {
	Type      DockerErrorType
	RepoImage string // This field is optional, present only for specific error types
	// Add more fields as needed for other error types
}

// Defines the function signature for generating formatted error messages.
// Note that the function does not perform validation - is is up to the caller to ensure
// amount or passed arguments is correct.
type ErrorMessageFunc func(errorData DockerError, args ...string) string

// A map associating DockerErrorType with functions to form error messages.
var errorMessageFunctions = map[DockerErrorType]ErrorMessageFunc{
	APIError404RepoNotFound: formatAPIError404RepoNotFound,
	// Add more mappings for other error types
}

// A map associating DockerErrorType with parser functions.
var errorParsers = map[DockerErrorType]func(string) (DockerError, error){
	APIError404RepoNotFound: parse404RepoNotFound,
	// Add more mappings for other error types
}

const (
	// error message patterns used by parsers
	Pattern404RepoNotFound = `API error \((404)\): repository (.+) not found`
	// internal errors
	ErrorMessageDoesNotMatch = `ErrorMessageDoesNotMatch`
	NumOfArgsUnknownForErrTy = `NumOfArgsUnknownForErrTy`
)

// Parses a 404 Repository Not Found error message.
func parse404RepoNotFound(err string) (DockerError, error) {
	pattern := Pattern404RepoNotFound
	regex := regexp.MustCompile(pattern)

	matches := regex.FindStringSubmatch(err)
	if len(matches) != 3 {
		return DockerError{}, errors.New(ErrorMessageDoesNotMatch)
	}

	image := matches[2]
	return DockerError{Type: APIError404RepoNotFound, RepoImage: image}, nil
}

// Extend error messages with extra useful information.
// Tries to find known cases by matching `errMsg` against implemented parsers for known errors.
// If the error message pattern is unknown, returns the original error message.
//
// Possible failure scenarios:
// 1. Missing parser implementation.
// 2. Missing validation details to assert the correct number of passed args for formatted message.
// 3. Insufficient number of args passed to formatted message.
//
// Scenarios 1 and 2 can only occur if a new ErrorType was added incorrectly due to implementation oversight.
// Scenario 3 can happen if the caller passes the wrong number of arguments.
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

	// validate the number of arguments passed. If validation failed - return original.
	numArgs, err := numOfArgsForFmtErrMsg(errorData)
	if err != nil {
		return errMsg
	}
	if len(args) < numArgs {
		return errMsg
	}

	// Generate rich error message with provided arguments.
	return formattedErrorMessage(errorData, args...)
}

// Returns number of arguments required in formatting function for given error type. Used in validation.
func numOfArgsForFmtErrMsg(err DockerError) (int, error) {
	switch err.Type {
	case APIError404RepoNotFound:
		return 1, nil // taskRole
	default:
		return 0, errors.New(NumOfArgsUnknownForErrTy)
	}
}

// Generates an error message for APIError404RepoNotFound.
func formatAPIError404RepoNotFound(errorData DockerError, args ...string) string {
	image := errorData.RepoImage
	taskRole := args[0]

	return fmt.Sprintf(
		"The task can’t pull the image"+
			" '%s' from Amazon Elastic Container Registry using the task"+
			" execution role '%s'. To fix this, verify that the"+
			" image URI is correct. Also check that the task execution"+
			" role has the additional permissions to pull Amazon ECR images."+
			" Status Code: 404. Response: 'not found'", image, taskRole)
}
