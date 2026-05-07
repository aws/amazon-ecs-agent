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
package field

const (
	TaskID                  = "task"
	TaskARN                 = "taskARN"
	TaskVersion             = "taskVersion"
	Container               = "container"
	DockerId                = "dockerId"
	ManagedAgent            = "managedAgent"
	KnownStatus             = "knownStatus"
	KnownSent               = "knownSent"
	DesiredStatus           = "desiredStatus"
	SentStatus              = "sentStatus"
	FailedStatus            = "failedStatus"
	Sequence                = "seqnum"
	Reason                  = "reason"
	Status                  = "status"
	RuntimeID               = "runtimeID"
	Elapsed                 = "elapsed"
	Resource                = "resource"
	Error                   = "error"
	Event                   = "event"
	Image                   = "image"
	Volume                  = "volume"
	NetworkMode             = "networkMode"
	Cluster                 = "cluster"
	ServiceName             = "ServiceName"
	TaskProtection          = "TaskProtection"
	ImageID                 = "imageID"
	ImageName               = "imageName"
	ImageRef                = "imageRef"
	ImageTARPath            = "imageTARPath"
	ImageNames              = "imageNames"
	ImageSizeBytes          = "imageSizeBytes"
	ImagePulledAt           = "imagePulledAt"
	ImageLastUsedAt         = "imageLastUsedAt"
	ImagePullSucceeded      = "imagePullSucceeded"
	ImageDigest             = "imageDigest"
	ImageMediaType          = "imageMediaType"
	ContainerName           = "containerName"
	ContainerImage          = "containerImage"
	ContainerExitCode       = "containerExitCode"
	TMDSEndpointContainerID = "tmdsEndpointContainerID"
	MessageID               = "messageID"
	RequestType             = "requestType"
	CredentialsID           = "credentialsID"
	ContainerInstanceARN    = "containerInstanceARN"
	AvailabilityZone        = "availabilityZone"
	AttributeName           = "attributeName"
	AttributeValue          = "attributeValue"
	Endpoint                = "endpoint"
	TelemetryEndpoint       = "telemetryEndpoint"
	ServiceConnectEndpoint  = "serviceConnectEndpoint"
	SystemLogsEndpoint      = "systemLogsEndpoint"
	Response                = "response"
	Request                 = "request"
	RoleType                = "roleType"
	RoleARN                 = "roleARN"
	CommandString           = "commandString"
	CommandOutput           = "commandOutput"
	RegistryID              = "registryID"
	ExecutionStoppedAt      = "executionStoppedAt"
	Region                  = "region"
	DockerVersion           = "dockerVersion"
	NetworkInterface        = "networkInterface"
)
