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

package metrics

const (
	// MetadataServer
	metadataServerMetricNamespace  = "MetadataServer"
	GetCredentialsMetricName       = metadataServerMetricNamespace + ".GetCredentials"
	CrashErrorMetricName           = metadataServerMetricNamespace + ".Crash"
	ShutdownErrorMetricName        = metadataServerMetricNamespace + ".ShutdownError"
	InternalServerErrorMetricName  = metadataServerMetricNamespace + ".InternalServerError"
	GetTaskProtectionMetricName    = metadataServerMetricNamespace + ".GetTaskProtection"
	UpdateTaskProtectionMetricName = metadataServerMetricNamespace + ".UpdateTaskProtection"
	AuthConfigMetricName           = metadataServerMetricNamespace + ".AuthConfig"

	// IntrospectionServer
	introspectionServerMetricNamespace = "IntrospectionServer"
	IntrospectionCrash                 = introspectionServerMetricNamespace + ".Crash"
	IntrospectionNotFound              = introspectionServerMetricNamespace + ".NotFound"
	IntrospectionFetchFailure          = introspectionServerMetricNamespace + ".FetchFailure"
	IntrospectionInternalServerError   = introspectionServerMetricNamespace + ".InternalServerError"
	IntrospectionBadRequest            = introspectionServerMetricNamespace + ".BadRequest"

	// AttachResourceResponder
	attachResourceResponderNamespace = "ResourceAttachment"
	ResourceValidationMetricName     = attachResourceResponderNamespace + ".Validation"

	// TaskManifestResponder
	taskManifestResponderNamespace = "TaskManifestResponder"
	TaskManifestHandlingDuration   = taskManifestResponderNamespace + ".Duration"

	// TaskStopVerificationACKResponder
	taskStopVerificationACKResponderNamespace = "TaskStopVeificationACKResponder"
	TaskStoppedMetricName                     = taskStopVerificationACKResponderNamespace + ".TaskStopped"

	// Credentials Refresh
	credsRefreshNamespace     = "CredentialsRefresh"
	CredentialsRefreshFailure = credsRefreshNamespace + ".Failure"
	CredentialsRefreshSuccess = credsRefreshNamespace + ".Success"

	// Agent Availability
	agentAvailabilityNamespace     = "Availability"
	ACSDisconnectTimeoutMetricName = agentAvailabilityNamespace + ".ACSDisconnectTimeout"
	TCSDisconnectTimeoutMetricName = agentAvailabilityNamespace + ".TCSDisconnectTimeout"

	// ACS Session Metrics
	acsSessionNamespace        = "ACSSession"
	ACSSessionFailureCallName  = acsSessionNamespace + ".ACSConnectFailure"
	ACSSessionCallDurationName = acsSessionNamespace + ".ACSConnectDuration"

	// ECS Client Metrics
	ecsClientNamespace               = "ECSClient"
	DiscoverPollEndpointFailure      = ecsClientNamespace + ".DiscoverPollEndpointFailure"
	DiscoverPollEndpointTotal        = ecsClientNamespace + ".DiscoverPollEndpoint"
	DiscoverPollEndpointDurationName = ecsClientNamespace + ".DiscoverPollEndpointDuration"

	dbClientMetricNamespace                 = "Data"
	GetNetworkConfigurationByTaskMetricName = dbClientMetricNamespace + ".GetNetworkConfigurationByTask"
	SaveNetworkNamespaceMetricName          = dbClientMetricNamespace + ".SaveNetworkNamespace"
	GetNetworkNamespaceMetricName           = dbClientMetricNamespace + ".GetNetworkNamespace"
	DelNetworkNamespaceMetricName           = dbClientMetricNamespace + ".DeleteNetworkNamespace"
	AssignGeneveDstPortMetricName           = dbClientMetricNamespace + ".AssignGeneveDstPort"

	networkBuilderNamespace               = "NetworkBuilder"
	BuildNetworkNamespaceMetricName       = networkBuilderNamespace + ".BuildNetworkNamespace"
	DeleteNetworkNamespaceMetricName      = networkBuilderNamespace + ".DeleteNetworkNamespace"
	V2NDestinationPortExhaustedMetricName = networkBuilderNamespace + ".V2NDestinationPortExhausted"
	ReleaseGeneveDstPortMetricName        = dbClientMetricNamespace + ".ReleaseGeneveDstPort"
)
