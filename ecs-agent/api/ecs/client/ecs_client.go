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

package ecsclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs"
	ecsmodel "github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"
	"github.com/aws/amazon-ecs-agent/ecs-agent/async"
	"github.com/aws/amazon-ecs-agent/ecs-agent/config"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials/instancecreds"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	"github.com/aws/amazon-ecs-agent/ecs-agent/httpclient"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/docker/docker/pkg/meminfo"
)

const (
	ecsMaxImageDigestLength     = 255
	ecsMaxContainerReasonLength = 255
	ecsMaxTaskReasonLength      = 1024
	ecsMaxRuntimeIDLength       = 255
	defaultPollEndpointCacheTTL = 12 * time.Hour
	azAttrName                  = "ecs.availability-zone"
	cpuArchAttrName             = "ecs.cpu-architecture"
	osTypeAttrName              = "ecs.os-type"
	osFamilyAttrName            = "ecs.os-family"
	// RoundtripTimeout should only time out after dial and TLS handshake timeouts have elapsed.
	// Add additional 2 seconds to the sum of these 2 timeouts to be extra sure of this.
	RoundtripTimeout = httpclient.DefaultDialTimeout + httpclient.DefaultTLSHandshakeTimeout + 2*time.Second
	// Below constants are used for SetInstanceIdentity retry with exponential backoff.
	setInstanceIdRetryTimeOut         = 30 * time.Second
	setInstanceIdRetryBackoffMin      = 100 * time.Millisecond
	setInstanceIdRetryBackoffMax      = 5 * time.Second
	setInstanceIdRetryBackoffJitter   = 0.2
	setInstanceIdRetryBackoffMultiple = 2
)

// ecsClient implements ECSClient interface.
type ecsClient struct {
	credentialsProvider              *credentials.Credentials
	configAccessor                   config.AgentConfigAccessor
	standardClient                   ecs.ECSStandardSDK
	submitStateChangeClient          ecs.ECSSubmitStateSDK
	ec2metadata                      ec2.EC2MetadataClient
	httpClient                       *http.Client
	pollEndpointCache                async.TTLCache
	isFIPSDetected                   bool
	shouldExcludeIPv6PortBinding     bool
	sascCustomRetryBackoff           func(func() error) error
	stscAttachmentCustomRetryBackoff func(func() error) error
}

// NewECSClient creates a new ECSClient interface object.
func NewECSClient(
	credentialsProvider *credentials.Credentials,
	configAccessor config.AgentConfigAccessor,
	ec2MetadataClient ec2.EC2MetadataClient,
	agentVer string,
	options ...ECSClientOption) (ecs.ECSClient, error) {

	client := &ecsClient{
		credentialsProvider: credentialsProvider,
		configAccessor:      configAccessor,
		ec2metadata:         ec2MetadataClient,
		httpClient:          httpclient.New(RoundtripTimeout, configAccessor.AcceptInsecureCert(), agentVer, configAccessor.OSType()),
		pollEndpointCache:   async.NewTTLCache(&async.TTL{Duration: defaultPollEndpointCacheTTL}),
	}

	// Apply options to configure/override ECS client values.
	for _, opt := range options {
		opt(client)
	}

	ecsConfig := newECSConfig(credentialsProvider, configAccessor, client.httpClient, client.isFIPSDetected)
	s, err := session.NewSession(&ecsConfig)
	if err != nil {
		return nil, err
	}

	if client.standardClient == nil {
		client.standardClient = ecsmodel.New(s)
	}
	if client.submitStateChangeClient == nil {
		client.submitStateChangeClient = newSubmitStateChangeClient(&ecsConfig)
	}

	return client, nil
}

func newECSConfig(
	credentialsProvider *credentials.Credentials,
	configAccessor config.AgentConfigAccessor,
	httpClient *http.Client,
	isFIPSEnabled bool) aws.Config {
	var ecsConfig aws.Config
	ecsConfig.HTTPClient = httpClient
	ecsConfig.Credentials = credentialsProvider
	ecsConfig.Region = aws.String(configAccessor.AWSRegion())
	// We should respect the endpoint given (if any) because it could be the Gamma or Zeta endpoint of ECS service which
	// don't have the corresponding FIPS endpoints. Otherwise, when the host has FIPS enabled, we should tell SDK to
	// pick the FIPS endpoint.
	if configAccessor.APIEndpoint() != "" {
		ecsConfig.Endpoint = aws.String(configAccessor.APIEndpoint())
	} else if isFIPSEnabled {
		ecsConfig.UseFIPSEndpoint = endpoints.FIPSEndpointStateEnabled
	}
	return ecsConfig
}

// CreateCluster creates a cluster from a given name and returns its ARN.
func (client *ecsClient) CreateCluster(clusterName string) (string, error) {
	resp, err := client.standardClient.CreateCluster(&ecsmodel.CreateClusterInput{ClusterName: &clusterName})
	if err != nil {
		logger.Critical("Could not create cluster", logger.Fields{
			field.Cluster: clusterName,
			field.Error:   err,
		})
		return "", err
	}
	logger.Info("Successfully created a cluster", logger.Fields{
		field.Cluster: clusterName,
	})
	return *resp.Cluster.ClusterName, nil
}

// RegisterContainerInstance calculates the appropriate resources, creates
// the default cluster if necessary, and returns the registered
// ContainerInstanceARN if successful. Supplying a non-empty container
// instance ARN allows a container instance to update its registered
// resources.
func (client *ecsClient) RegisterContainerInstance(containerInstanceArn string, attributes []*ecsmodel.Attribute,
	tags []*ecsmodel.Tag, registrationToken string, platformDevices []*ecsmodel.PlatformDevice,
	outpostARN string) (string, string, error) {

	clusterRef := client.configAccessor.Cluster()
	// If our clusterRef is empty, we should try to create the default.
	if clusterRef == "" {
		clusterRef = client.configAccessor.DefaultClusterName()
		defer client.configAccessor.UpdateCluster(clusterRef)
		// Attempt to register without checking existence of the cluster so that we don't require
		// excess permissions in the case where the cluster already exists and is active.
		containerInstanceArn, availabilityzone, err := client.registerContainerInstance(clusterRef,
			containerInstanceArn, attributes, tags, registrationToken, platformDevices, outpostARN)
		if err == nil {
			return containerInstanceArn, availabilityzone, nil
		}

		// If trying to register fails because the default cluster doesn't exist, try to create the cluster before
		// calling register again.
		if apierrors.IsClusterNotFoundError(err) {
			clusterRef, err = client.CreateCluster(clusterRef)
			if err != nil {
				return "", "", err
			}
		}
	}
	return client.registerContainerInstance(clusterRef, containerInstanceArn, attributes, tags, registrationToken,
		platformDevices, outpostARN)
}

func (client *ecsClient) registerContainerInstance(clusterRef string, containerInstanceArn string,
	attributes []*ecsmodel.Attribute, tags []*ecsmodel.Tag, registrationToken string,
	platformDevices []*ecsmodel.PlatformDevice, outpostARN string) (string, string, error) {

	registerRequest := ecsmodel.RegisterContainerInstanceInput{Cluster: &clusterRef}
	var registrationAttributes []*ecsmodel.Attribute
	if containerInstanceArn != "" {
		// We are re-connecting a previously registered instance, restored from snapshot.
		registerRequest.ContainerInstanceArn = &containerInstanceArn
	} else {
		// This is a new instance, not previously registered.
		// Custom attribute registration only happens on initial instance registration.
		for _, attribute := range client.getCustomAttributes() {
			logger.Debug("Added a new custom attribute", logger.Fields{
				field.AttributeName:  aws.StringValue(attribute.Name),
				field.AttributeValue: aws.StringValue(attribute.Value),
			})
			registrationAttributes = append(registrationAttributes, attribute)
		}
	}
	// Standard attributes are included with all registrations.
	registrationAttributes = append(registrationAttributes, attributes...)

	// Add additional attributes, such as the OS type.
	registrationAttributes = append(registrationAttributes, client.getAdditionalAttributes()...)
	registrationAttributes = append(registrationAttributes, client.getOutpostAttribute(outpostARN)...)

	registerRequest.Attributes = registrationAttributes
	if len(tags) > 0 {
		registerRequest.Tags = tags
	}
	registerRequest.PlatformDevices = platformDevices
	registerRequest = client.setInstanceIdentity(registerRequest)

	logger.Info("REGISTER PAYLOAD %v", registerRequest)
	logger.Info("REGISTER PLATFORM DEVICES %v", registerRequest.PlatformDevices)

	resources, err := client.getResources()
	if err != nil {
		return "", "", err
	}

	registerRequest.TotalResources = resources

	registerRequest.ClientToken = &registrationToken
	resp, err := client.standardClient.RegisterContainerInstance(&registerRequest)
	if err != nil {
		logger.Error("Unable to register as a container instance with ECS", logger.Fields{
			field.Error: err,
		})
		return "", "", err
	}

	var availabilityzone = ""
	if resp != nil {
		for _, attr := range resp.ContainerInstance.Attributes {
			if aws.StringValue(attr.Name) == azAttrName {
				availabilityzone = aws.StringValue(attr.Value)
				break
			}
		}
	}

	logger.Info("Registered container instance with cluster!")
	err = validateRegisteredAttributes(registerRequest.Attributes, resp.ContainerInstance.Attributes)
	return aws.StringValue(resp.ContainerInstance.ContainerInstanceArn), availabilityzone, err
}

func (client *ecsClient) setInstanceIdentity(
	registerRequest ecsmodel.RegisterContainerInstanceInput) ecsmodel.RegisterContainerInstanceInput {
	instanceIdentityDoc := ""
	instanceIdentitySignature := ""

	if client.configAccessor.NoInstanceIdentityDocument() {
		logger.Info("Fetching Instance ID Document has been disabled")
		registerRequest.InstanceIdentityDocument = &instanceIdentityDoc
		registerRequest.InstanceIdentityDocumentSignature = &instanceIdentitySignature
		return registerRequest
	}

	iidRetrieved := true
	backoff := retry.NewExponentialBackoff(setInstanceIdRetryBackoffMin, setInstanceIdRetryBackoffMax,
		setInstanceIdRetryBackoffJitter, setInstanceIdRetryBackoffMultiple)
	ctx, cancel := context.WithTimeout(context.Background(), setInstanceIdRetryTimeOut)
	defer cancel()
	err := retry.RetryWithBackoffCtx(ctx, backoff, func() error {
		var attemptErr error
		logger.Debug("Attempting to get Instance Identity Document")
		instanceIdentityDoc, attemptErr = client.ec2metadata.GetDynamicData(ec2.InstanceIdentityDocumentResource)
		if attemptErr != nil {
			logger.Debug("Unable to get instance identity document, retrying", logger.Fields{
				field.Error: attemptErr,
			})
			// Force credentials to expire in case they are stale but not expired.
			client.credentialsProvider.Expire()
			client.credentialsProvider = instancecreds.GetCredentials(client.configAccessor.External())
			return apierrors.NewRetriableError(apierrors.NewRetriable(true), attemptErr)
		}
		logger.Debug("Successfully retrieved Instance Identity Document")
		return nil
	})
	if err != nil {
		logger.Error("Unable to get instance identity document", logger.Fields{
			field.Error: err,
		})
		iidRetrieved = false
	}
	registerRequest.InstanceIdentityDocument = &instanceIdentityDoc

	if iidRetrieved {
		ctx, cancel = context.WithTimeout(context.Background(), setInstanceIdRetryTimeOut)
		defer cancel()
		err = retry.RetryWithBackoffCtx(ctx, backoff, func() error {
			var attemptErr error
			logger.Debug("Attempting to get Instance Identity Signature")
			instanceIdentitySignature, attemptErr = client.ec2metadata.
				GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource)
			if attemptErr != nil {
				logger.Debug("Unable to get instance identity signature, retrying", logger.Fields{
					field.Error: attemptErr,
				})
				return apierrors.NewRetriableError(apierrors.NewRetriable(true), attemptErr)
			}
			logger.Debug("Successfully retrieved Instance Identity Signature")
			return nil
		})
		if err != nil {
			logger.Error("Unable to get instance identity signature", logger.Fields{
				field.Error: err,
			})
		}
	}

	registerRequest.InstanceIdentityDocumentSignature = &instanceIdentitySignature
	return registerRequest
}

func attributesToMap(attributes []*ecsmodel.Attribute) map[string]string {
	attributeMap := make(map[string]string)
	attribs := attributes
	for _, attribute := range attribs {
		attributeMap[aws.StringValue(attribute.Name)] = aws.StringValue(attribute.Value)
	}
	return attributeMap
}

func findMissingAttributes(expectedAttributes, actualAttributes map[string]string) ([]string, error) {
	missingAttributes := make([]string, 0)
	var err error
	for key, val := range expectedAttributes {
		if actualAttributes[key] != val {
			missingAttributes = append(missingAttributes, key)
		} else {
			logger.Trace("Response contained expected value for attribute", logger.Fields{
				"key": key,
			})
		}
	}
	if len(missingAttributes) > 0 {
		err = apierrors.NewAttributeError("Attribute validation failed")
	}
	return missingAttributes, err
}

func (client *ecsClient) getResources() ([]*ecsmodel.Resource, error) {
	// Below are micro-optimizations - the pointers to integerStr and stringSetStr are used multiple times below.
	integerStr := "INTEGER"
	stringSetStr := "STRINGSET"

	cpu, mem := getCpuAndMemory()
	remainingMem := mem - int64(client.configAccessor.ReservedMemory())
	logger.Info("Remaining memory", logger.Fields{
		"remainingMemory": remainingMem,
	})
	if remainingMem < 0 {
		return nil, fmt.Errorf(
			"api register-container-instance: reserved memory is higher than available memory on the host, "+
				"total memory: %d, reserved: %d", mem, client.configAccessor.ReservedMemory())
	}

	cpuResource := ecsmodel.Resource{
		Name:         aws.String("CPU"),
		Type:         &integerStr,
		IntegerValue: &cpu,
	}
	memResource := ecsmodel.Resource{
		Name:         aws.String("MEMORY"),
		Type:         &integerStr,
		IntegerValue: &remainingMem,
	}
	portResource := ecsmodel.Resource{
		Name:           aws.String("PORTS"),
		Type:           &stringSetStr,
		StringSetValue: utils.Uint16SliceToStringSlice(client.configAccessor.ReservedPorts()),
	}
	udpPortResource := ecsmodel.Resource{
		Name:           aws.String("PORTS_UDP"),
		Type:           &stringSetStr,
		StringSetValue: utils.Uint16SliceToStringSlice(client.configAccessor.ReservedPortsUDP()),
	}

	return []*ecsmodel.Resource{&cpuResource, &memResource, &portResource, &udpPortResource}, nil
}

// GetHostResources calling getHostResources to get a list of CPU, MEMORY, PORTS and PORTS_UPD resources
// and return a resourceMap that map the resource name to each resource
func (client *ecsClient) GetHostResources() (map[string]*ecsmodel.Resource, error) {
	resources, err := client.getResources()
	if err != nil {
		return nil, err
	}
	resourceMap := make(map[string]*ecsmodel.Resource)
	for _, resource := range resources {
		if *resource.Name == "PORTS" {
			// Except for RCI, TCP Ports are named as PORTS_TCP in Agent for Host Resources purpose.
			resource.Name = aws.String("PORTS_TCP")
		}
		resourceMap[*resource.Name] = resource
	}
	return resourceMap, nil
}

func getCpuAndMemory() (int64, int64) {
	memInfo, err := meminfo.Read()
	mem := int64(0)
	if err == nil {
		mem = memInfo.MemTotal / 1024 / 1024 // MiB
	} else {
		logger.Error("Unable to get memory info", logger.Fields{
			field.Error: err,
		})
	}

	cpu := utils.GetNumCPU() * 1024

	return int64(cpu), mem
}

func validateRegisteredAttributes(expectedAttributes, actualAttributes []*ecsmodel.Attribute) error {
	var err error
	expectedAttributesMap := attributesToMap(expectedAttributes)
	actualAttributesMap := attributesToMap(actualAttributes)
	missingAttributes, err := findMissingAttributes(expectedAttributesMap, actualAttributesMap)
	if err != nil {
		msg := strings.Join(missingAttributes, ",")
		logger.Error("Error registering attributes", logger.Fields{
			field.Error:         err,
			"missingAttributes": msg,
		})
	}
	return err
}

func (client *ecsClient) getAdditionalAttributes() []*ecsmodel.Attribute {
	var attrs []*ecsmodel.Attribute

	// Add a check to ensure only non-empty values are added
	// to API call.
	if client.configAccessor.OSType() != "" {
		attrs = append(attrs, &ecsmodel.Attribute{
			Name:  aws.String(osTypeAttrName),
			Value: aws.String(client.configAccessor.OSType()),
		})
	}

	// OSFamily should be treated as an optional field as it is not applicable for all agents
	// using ecs client shared library. Add a check to ensure only non-empty values are added
	// to API call.
	if client.configAccessor.OSFamily() != "" {
		attrs = append(attrs, &ecsmodel.Attribute{
			Name:  aws.String(osFamilyAttrName),
			Value: aws.String(client.configAccessor.OSFamily()),
		})
	}
	// Send CPU arch attribute directly when running on external capacity. When running on EC2 or Fargate launch type,
	// this is not needed since the CPU arch is reported via instance identity document in those cases.
	if client.configAccessor.External() {
		attrs = append(attrs, &ecsmodel.Attribute{
			Name:  aws.String(cpuArchAttrName),
			Value: aws.String(getCPUArch()),
		})
	}
	return attrs
}

func (client *ecsClient) getOutpostAttribute(outpostARN string) []*ecsmodel.Attribute {
	if len(outpostARN) > 0 {
		return []*ecsmodel.Attribute{
			{
				Name:  aws.String("ecs.outpost-arn"),
				Value: aws.String(outpostARN),
			},
		}
	}
	return []*ecsmodel.Attribute{}
}

func (client *ecsClient) getCustomAttributes() []*ecsmodel.Attribute {
	var attributes []*ecsmodel.Attribute
	for attribute, value := range client.configAccessor.InstanceAttributes() {
		attributes = append(attributes, &ecsmodel.Attribute{
			Name:  aws.String(attribute),
			Value: aws.String(value),
		})
	}
	return attributes
}

func (client *ecsClient) SubmitTaskStateChange(change ecs.TaskStateChange) error {
	if change.Attachment != nil && client.stscAttachmentCustomRetryBackoff != nil {
		retryFunc := func() error {
			err := client.submitTaskStateChange(change)
			if err == nil {
				return nil
			}
			return submitStateCustomRetriableError(err)
		}
		return client.stscAttachmentCustomRetryBackoff(retryFunc)
	}
	return client.submitTaskStateChange(change)
}

func (client *ecsClient) submitTaskStateChange(change ecs.TaskStateChange) error {
	if change.Attachment != nil {
		// Confirm attachment by submitting attachment state change via SubmitTaskStateChange API (specifically in
		// the input's Attachments field).
		var attachments []*ecsmodel.AttachmentStateChange
		eniStatus := change.Attachment.Status.String()
		attachments = []*ecsmodel.AttachmentStateChange{
			{
				AttachmentArn: aws.String(change.Attachment.AttachmentARN),
				Status:        aws.String(eniStatus),
			},
		}

		_, err := client.submitStateChangeClient.SubmitTaskStateChange(&ecsmodel.SubmitTaskStateChangeInput{
			Cluster:     aws.String(client.configAccessor.Cluster()),
			Task:        aws.String(change.TaskARN),
			Attachments: attachments,
		})
		if err != nil {
			logger.Warn("Could not submit task state change associated with confirming attachment",
				logger.Fields{
					field.Error:     err,
					"attachmentARN": change.Attachment.AttachmentARN,
					field.Status:    eniStatus,
				})
			return err
		}

		return nil
	}

	req := ecsmodel.SubmitTaskStateChangeInput{
		Cluster:            aws.String(client.configAccessor.Cluster()),
		Task:               aws.String(change.TaskARN),
		Status:             aws.String(change.Status.BackendStatus()),
		Reason:             aws.String(trimString(change.Reason, ecsMaxTaskReasonLength)),
		PullStartedAt:      change.PullStartedAt,
		PullStoppedAt:      change.PullStoppedAt,
		ExecutionStoppedAt: change.ExecutionStoppedAt,
		ManagedAgents:      formatManagedAgents(change.ManagedAgents),
		Containers:         formatContainers(change.Containers, client.shouldExcludeIPv6PortBinding, change.TaskARN),
	}

	_, err := client.submitStateChangeClient.SubmitTaskStateChange(&req)
	if err != nil {
		logger.Warn("Could not submit task state change", logger.Fields{
			field.Error:       err,
			"taskStateChange": change.String(),
		})
		return err
	}

	return nil
}

func (client *ecsClient) SubmitContainerStateChange(change ecs.ContainerStateChange) error {
	input := ecsmodel.SubmitContainerStateChangeInput{
		Cluster:       aws.String(client.configAccessor.Cluster()),
		ContainerName: aws.String(change.ContainerName),
		Task:          aws.String(change.TaskArn),
	}

	if change.RuntimeID != "" {
		input.RuntimeId = aws.String(trimString(change.RuntimeID, ecsMaxRuntimeIDLength))
	}

	if change.Reason != "" {
		input.Reason = aws.String(trimString(change.Reason, ecsMaxContainerReasonLength))
	}

	stat := change.Status.String()
	if stat == "DEAD" {
		stat = apicontainerstatus.ContainerStopped.String()
	}
	if stat != apicontainerstatus.ContainerStopped.String() && stat != apicontainerstatus.ContainerRunning.String() {
		logger.Info("Not submitting unsupported upstream container state", logger.Fields{
			field.ContainerName: change.ContainerName,
			field.Status:        stat,
			field.TaskARN:       change.TaskArn,
		})
		return nil
	}
	input.Status = aws.String(stat)

	if change.ExitCode != nil {
		exitCode := int64(aws.IntValue(change.ExitCode))
		input.ExitCode = aws.Int64(exitCode)
	}

	networkBindings := change.NetworkBindings
	if client.shouldExcludeIPv6PortBinding {
		networkBindings = excludeIPv6PortBindingFromNetworkBindings(networkBindings, change.ContainerName,
			change.TaskArn)
	}
	input.NetworkBindings = networkBindings

	_, err := client.submitStateChangeClient.SubmitContainerStateChange(&input)
	if err != nil {
		logger.Warn("Could not submit container state change", logger.Fields{
			field.Error:            err,
			field.TaskARN:          change.TaskArn,
			"containerStateChange": change.String(),
		})
		return err
	}
	return nil
}

func (client *ecsClient) SubmitAttachmentStateChange(change ecs.AttachmentStateChange) error {
	if client.sascCustomRetryBackoff != nil {
		retryFunc := func() error {
			err := client.submitAttachmentStateChange(change)
			if err == nil {
				return nil
			}
			return submitStateCustomRetriableError(err)
		}
		return client.sascCustomRetryBackoff(retryFunc)
	}
	return client.submitAttachmentStateChange(change)
}

func (client *ecsClient) submitAttachmentStateChange(change ecs.AttachmentStateChange) error {
	attachmentStatus := change.Attachment.GetAttachmentStatus()

	req := ecsmodel.SubmitAttachmentStateChangesInput{
		Cluster: aws.String(client.configAccessor.Cluster()),
		Attachments: []*ecsmodel.AttachmentStateChange{
			{
				AttachmentArn: aws.String(change.Attachment.GetAttachmentARN()),
				Status:        aws.String(attachmentStatus.String()),
			},
		},
	}

	_, err := client.submitStateChangeClient.SubmitAttachmentStateChanges(&req)
	if err != nil {
		logger.Warn("Could not submit attachment state change", logger.Fields{
			field.Error:             err,
			"attachmentStateChange": change.String(),
		})
		return err
	}

	return nil
}

func submitStateCustomRetriableError(err error) error {
	retry := true
	aerr, ok := err.(awserr.Error)
	if ok {
		switch aerr.Code() {
		case ecsmodel.ErrCodeInvalidParameterException:
			retry = false
		case ecsmodel.ErrCodeAccessDeniedException:
			retry = false
		case ecsmodel.ErrCodeClientException:
			retry = false
		}
	}
	return apierrors.NewRetriableError(apierrors.NewRetriable(retry), err)
}

func (client *ecsClient) DiscoverPollEndpoint(containerInstanceArn string) (string, error) {
	resp, err := client.discoverPollEndpoint(containerInstanceArn)
	if err != nil {
		return "", err
	}
	if resp.Endpoint == nil {
		return "", errors.New("no endpoint returned; nil")
	}

	return aws.StringValue(resp.Endpoint), nil
}

func (client *ecsClient) DiscoverTelemetryEndpoint(containerInstanceArn string) (string, error) {
	resp, err := client.discoverPollEndpoint(containerInstanceArn)
	if err != nil {
		return "", err
	}
	if resp.TelemetryEndpoint == nil {
		return "", errors.New("no telemetry endpoint returned; nil")
	}

	return aws.StringValue(resp.TelemetryEndpoint), nil
}

func (client *ecsClient) DiscoverServiceConnectEndpoint(containerInstanceArn string) (string, error) {
	resp, err := client.discoverPollEndpoint(containerInstanceArn)
	if err != nil {
		return "", err
	}
	if resp.ServiceConnectEndpoint == nil {
		return "", errors.New("no ServiceConnect endpoint returned; nil")
	}

	return aws.StringValue(resp.ServiceConnectEndpoint), nil
}

func (client *ecsClient) discoverPollEndpoint(containerInstanceArn string) (*ecsmodel.DiscoverPollEndpointOutput,
	error) {
	// Try getting an entry from the cache.
	cachedEndpoint, expired, found := client.pollEndpointCache.Get(containerInstanceArn)
	if !expired && found {
		// Cache hit and not expired. Return the output.
		if output, ok := cachedEndpoint.(*ecsmodel.DiscoverPollEndpointOutput); ok {
			logger.Info("Using cached DiscoverPollEndpoint", logger.Fields{
				field.Endpoint:               aws.StringValue(output.Endpoint),
				field.TelemetryEndpoint:      aws.StringValue(output.TelemetryEndpoint),
				field.ServiceConnectEndpoint: aws.StringValue(output.ServiceConnectEndpoint),
				field.ContainerInstanceARN:   containerInstanceArn,
			})
			return output, nil
		}
	}

	// Cache miss or expired, invoke the ECS DiscoverPollEndpoint API.
	logger.Debug("Invoking DiscoverPollEndpoint", logger.Fields{
		field.ContainerInstanceARN: containerInstanceArn,
	})
	output, err := client.standardClient.DiscoverPollEndpoint(&ecsmodel.DiscoverPollEndpointInput{
		ContainerInstance: &containerInstanceArn,
		Cluster:           aws.String(client.configAccessor.Cluster()),
	})
	if err != nil {
		// If we got an error calling the API, fallback to an expired cached endpoint if
		// we have it.
		if expired {
			if output, ok := cachedEndpoint.(*ecsmodel.DiscoverPollEndpointOutput); ok {
				logger.Info("Error calling DiscoverPollEndpoint. Using cached-but-expired endpoint as a fallback.",
					logger.Fields{
						field.Endpoint:               aws.StringValue(output.Endpoint),
						field.TelemetryEndpoint:      aws.StringValue(output.TelemetryEndpoint),
						field.ServiceConnectEndpoint: aws.StringValue(output.ServiceConnectEndpoint),
						field.ContainerInstanceARN:   containerInstanceArn,
					})
				return output, nil
			}
		}
		return nil, err
	}

	// Cache the response from ECS.
	client.pollEndpointCache.Set(containerInstanceArn, output)
	return output, nil
}

func (client *ecsClient) GetResourceTags(resourceArn string) ([]*ecsmodel.Tag, error) {
	output, err := client.standardClient.ListTagsForResource(&ecsmodel.ListTagsForResourceInput{
		ResourceArn: &resourceArn,
	})
	if err != nil {
		return nil, err
	}
	return output.Tags, nil
}

func (client *ecsClient) UpdateContainerInstancesState(instanceARN string, status string) error {
	logger.Debug("Invoking UpdateContainerInstancesState", logger.Fields{
		field.Status:               status,
		field.ContainerInstanceARN: instanceARN,
	})
	_, err := client.standardClient.UpdateContainerInstancesState(&ecsmodel.UpdateContainerInstancesStateInput{
		ContainerInstances: []*string{aws.String(instanceARN)},
		Status:             aws.String(status),
		Cluster:            aws.String(client.configAccessor.Cluster()),
	})
	return err
}

func formatManagedAgents(managedAgents []*ecsmodel.ManagedAgentStateChange) []*ecsmodel.ManagedAgentStateChange {
	var result []*ecsmodel.ManagedAgentStateChange
	for _, m := range managedAgents {
		if m.Reason != nil {
			m.Reason = trimStringPtr(m.Reason, ecsMaxContainerReasonLength)
		}
		result = append(result, m)
	}
	return result
}

func formatContainers(containers []*ecsmodel.ContainerStateChange, shouldExcludeIPv6PortBinding bool,
	taskARN string) []*ecsmodel.ContainerStateChange {
	var result []*ecsmodel.ContainerStateChange
	for _, c := range containers {
		if c.RuntimeId != nil {
			c.RuntimeId = trimStringPtr(c.RuntimeId, ecsMaxRuntimeIDLength)
		}
		if c.Reason != nil {
			c.Reason = trimStringPtr(c.Reason, ecsMaxContainerReasonLength)
		}
		if c.ImageDigest != nil {
			c.ImageDigest = trimStringPtr(c.ImageDigest, ecsMaxImageDigestLength)
		}
		if shouldExcludeIPv6PortBinding {
			c.NetworkBindings = excludeIPv6PortBindingFromNetworkBindings(c.NetworkBindings,
				aws.StringValue(c.ContainerName), taskARN)
		}
		result = append(result, c)
	}
	return result
}

func excludeIPv6PortBindingFromNetworkBindings(networkBindings []*ecsmodel.NetworkBinding, containerName,
	taskARN string) []*ecsmodel.NetworkBinding {
	var result []*ecsmodel.NetworkBinding
	for _, binding := range networkBindings {
		if aws.StringValue(binding.BindIP) == "::" {
			logger.Debug("Exclude IPv6 port binding", logger.Fields{
				"portBinding":       binding,
				field.ContainerName: containerName,
				field.TaskARN:       taskARN,
			})
			continue
		}
		result = append(result, binding)
	}
	return result
}

func trimStringPtr(inputStringPtr *string, maxLen int) *string {
	if inputStringPtr == nil {
		return nil
	}
	return aws.String(trimString(aws.StringValue(inputStringPtr), maxLen))
}

func trimString(inputString string, maxLen int) string {
	if len(inputString) > maxLen {
		trimmed := inputString[0:maxLen]
		return trimmed
	} else {
		return inputString
	}
}
