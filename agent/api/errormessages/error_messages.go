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

package errormessages

import (
	"errors"
	"fmt"
	"regexp"

	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
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
// To augment an error message, call the AugmentNamedErrMsg or AugmentMessage function with the original error
// If the error message matches known error pattern, it will be augmented with extra information; otherwise,
// the original error message will be returned.
//
// Example:
//   augmentedMsg := apierrors.AugmentMessage("API error (404): repository not found")
//

// DockerErrorType represents the type of error returned by Docker client.
type DockerErrorType int

const (
	MissingECRBatchGetImageError DockerErrorType = iota
	ECRImageDoesNotExistError
	NetworkConfigurationError
)

type DockerError struct {
	Type     DockerErrorType
	RawError string
}

// Defines the function signature for generating formatted error messages.
type ErrorMessageFunc func(errorData DockerError) string

// A map associating DockerErrorType with functions to format error messages.
var errorMessageFunctions = map[DockerErrorType]ErrorMessageFunc{
	MissingECRBatchGetImageError: formatMissingECRBatchGetImageError,
	ECRImageDoesNotExistError:    formatImageDoesNotExistError,
	NetworkConfigurationError:    formatNetworkConfigurationError,
}

// A map associating DockerErrorType with parser functions.
var errorParsers = map[DockerErrorType]func(string) (DockerError, error){
	MissingECRBatchGetImageError: parseMissingPullImagePermsError,
	ECRImageDoesNotExistError:    parseImageDoesNotExistError,
	NetworkConfigurationError:    parseNetworkConfigurationError,
}

// An interface that provides means to reconstruct Named Errors. This is
// used during error message augmentation process.
type AugmentableNamedError interface {
	apierrors.NamedError
	WithAugmentedErrorMessage(string) apierrors.NamedError
}

const (
	// error message patterns used by parsers
	PatternECRBatchGetImageError     = `denied: User: (.+) is not authorized to perform: ecr:BatchGetImage on resource`
	PatternImageDoesNotExistError    = `denied: requested access to the resource is denied`
	PatternNetworkConfigurationError = `request canceled while waiting for connection`
	// internal errors
	ErrorMessageDoesNotMatch = `ErrorMessageDoesNotMatch`
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

func parseNetworkConfigurationError(err string) (DockerError, error) {
	matched, _ := regexp.MatchString(PatternNetworkConfigurationError, err)
	if matched {
		return DockerError{
			Type:     NetworkConfigurationError,
			RawError: err,
		}, nil
	}
	return DockerError{RawError: err}, errors.New(ErrorMessageDoesNotMatch)
}

// Extend error with extra useful information. Works with NamedErrors.
func AugmentNamedErrMsg(namedErr apierrors.NamedError) apierrors.NamedError {
	augmentableErr, ok := namedErr.(AugmentableNamedError)
	if !ok { // If namedErr is not a AugmentableNamedError, return as-is.
		return namedErr
	}

	// Augment the error message.
	augmentedMsg := AugmentMessage(namedErr.Error())
	// Reconstruct new NamedError.
	newError := augmentableErr.WithAugmentedErrorMessage(augmentedMsg)
	return newError
}

// Extend error messages with extra useful information.
// Tries to find known cases by matching `errMsg` against implemented parsers for known errors.
// If the error message pattern is unknown, returns the original error message.
//
// Possible failure scenarios:
//  1. Missing parser implementation - this can only occur if a new ErrorType was added incorrectly
//     due to implementation oversight.
//
// Currently, the function is set up in a safe manner - all failures are ignored, and
// the original error message string is returned.
func AugmentMessage(errMsg string) string {
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
		logger.Debug("AugmentMessage: None of the parsers match - return original error message")
		return errMsg
	}

	// lookup corresponding error formatter function. If not found - return original.
	formattedErrorMessage, ok := errorMessageFunctions[errorData.Type]
	if !ok {
		return errMsg
	}

	return formattedErrorMessage(errorData)
}

// Generates error message for MissingECRBatchGetImage error.
func formatMissingECRBatchGetImageError(errorData DockerError) string {
	rawError := errorData.RawError

	formattedMessage := fmt.Sprintf(
		"The task can’t pull the image. Check that the role has the permissions to pull images from the registry. %s",
		rawError)

	return formattedMessage
}

// Generates error message for ECRImageDoesNotExist error.
func formatImageDoesNotExistError(errorData DockerError) string {
	rawError := errorData.RawError

	formattedMessage := fmt.Sprintf(
		"The task can’t pull the image. Check whether the image exists. %s",
		rawError)

	return formattedMessage
}

// Generates error message for network misconfiguration errors.
func formatNetworkConfigurationError(errorData DockerError) string {
	rawError := errorData.RawError

	formattedMessage := fmt.Sprintf("The task can’t pull the image. Check your network configuration. %s",
		rawError)

	return formattedMessage
}
