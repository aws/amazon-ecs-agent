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

package state

import "fmt"

// Error to be returned when container or task lookup failed
type ErrorLookupFailure struct {
	externalReason string // Reason to be included in the response
}

func NewErrorLookupFailure(externalReason string) *ErrorLookupFailure {
	return &ErrorLookupFailure{externalReason: externalReason}
}

func (e *ErrorLookupFailure) ExternalReason() string {
	return e.externalReason
}

func (e *ErrorLookupFailure) Error() string {
	return fmt.Sprintf("container lookup failed: %s", e.externalReason)
}

// General "catch-all" error to be returned when container or task metadata could not be
// fetched for some reason
type ErrorMetadataFetchFailure struct {
	externalReason string // Reason to be included in the response
}

func NewErrorMetadataFetchFailure(externalReason string) *ErrorMetadataFetchFailure {
	return &ErrorMetadataFetchFailure{externalReason: externalReason}
}

func (e *ErrorMetadataFetchFailure) Error() string {
	return fmt.Sprintf("container lookup failed: %s", e.externalReason)
}

func (e *ErrorMetadataFetchFailure) ExternalReason() string {
	return e.externalReason
}

// Error to be returned when container or task stats lookup failed due to a lookup failure
type ErrorStatsLookupFailure struct {
	externalReason string // Reason to be returned in TMDS response
}

func NewErrorStatsLookupFailure(externalReason string) *ErrorStatsLookupFailure {
	return &ErrorStatsLookupFailure{externalReason}
}

func (e *ErrorStatsLookupFailure) ExternalReason() string {
	return e.externalReason
}

func (e *ErrorStatsLookupFailure) Error() string {
	return fmt.Sprintf("stats lookup failed: %s", e.externalReason)
}

// General "catch-all" error to be returned when container or task stats could not
// be fetched for some reason.
type ErrorStatsFetchFailure struct {
	externalReason string // Reason to be returned in TMDS response
	cause          error
}

func NewErrorStatsFetchFailure(externalReason string, cause error) *ErrorStatsFetchFailure {
	return &ErrorStatsFetchFailure{externalReason, cause}
}

func (e *ErrorStatsFetchFailure) ExternalReason() string {
	return e.externalReason
}

func (e *ErrorStatsFetchFailure) Error() string {
	return fmt.Sprintf("failed to get stats: %s: %v", e.externalReason, e.cause)
}

func (e *ErrorStatsFetchFailure) Unwrap() error {
	return e.cause
}

// Interface for interacting with Agent State relevant to TMDS
type AgentState interface {
	// Returns container metadata in v4 format for the container identified by the
	// provided endpointContinerID.
	// Returns ErrorLookupFailure if container lookup fails.
	// Returns ErrorMetadataFetchFailure if something else goes wrong.
	GetContainerMetadata(endpointContainerID string) (ContainerResponse, error)

	// Returns task metadata in v4 format for the task identified by the provided endpointContainerID.
	// Returns ErrorTaskLookupFailed if task lookup fails.
	// Returns ErrorMetadataFetchFailure if something else goes wrong.
	GetTaskMetadata(endpointContainerID string) (TaskResponse, error)

	// Returns task metadata including task and container instance tags (if applicable) in v4 format
	// for the task identified by the provided endpointContainerID.
	// Returns ErrorTaskLookupFailed if task lookup fails.
	// Returns ErrorMetadataFetchFailure if something else goes wrong.
	GetTaskMetadataWithTags(endpointContainerID string) (TaskResponse, error)

	// Returns container stats in v4 format for the container identified by the provided
	// endpointContainerID.
	// Returns ErrorStatsLookupFailure if container lookup fails.
	// Returns ErrorStatsFetchFailure if something else goes wrong.
	GetContainerStats(endpointContainerID string) (StatsResponse, error)

	// Returns task stats in v4 format for the task identified by the provided
	// endpointContainerID.
	// Returns ErrorStatsLookupFailure if container lookup fails.
	// Returns ErrorStatsFetchFailure if something else goes wrong.
	GetTaskStats(endpointContainerID string) (map[string]*StatsResponse, error)
}
