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
	"sync"
	"time"

	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs"
	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"
	"github.com/aws/amazon-ecs-agent/ecs-agent/async"
	"github.com/aws/amazon-ecs-agent/ecs-agent/config"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	"github.com/aws/amazon-ecs-agent/ecs-agent/httpclient"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	ecsservice "github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/smithy-go"
	"github.com/docker/docker/pkg/meminfo"
)

const (
	ecsMaxImageDigestLength     = 255
	ecsMaxContainerReasonLength = 1024
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
	// discoverPollEndpointTimeout is the maximum permitted time a single ECSClient.DiscoverPollEndpoint call can take.
	// The SDK client uses the default retryer which gives a max retry count of 3, we combine this with the timeout for the underlying httpclient's RoundtripTimeout.
	discoverPollEndpointTimeout = 3 * RoundtripTimeout
	// Below constants are used for RegisterContainerInstance retry with exponential backoff when receiving non-terminal errors.
	// To ensure parity in all regions and on all launch types, we should not set any time limit on the RCI timeout.
	// Thus, setting the max RCI retry timeout allowed to 1 hour, and capping max retry backoff at 192 seconds (3 * 2^6).
	rciMaxRetryTimeAllowed = 1 * time.Hour
	rciMinBackoff          = 3 * time.Second
	rciMaxBackoff          = 192 * time.Second
	rciRetryJitter         = 0.2
	rciRetryMultiple       = 2.0
)

var nonRetriableErrors = []smithy.APIError{
	new(types.AccessDeniedException),
	new(types.InvalidParameterException),
	new(types.ClientException),
}

// ecsClient implements ECSClient interface.
type ecsClient struct {
	credentialsCache                 *aws.CredentialsCache
	configAccessor                   config.AgentConfigAccessor
	standardClient                   ecs.ECSStandardSDK
	submitStateChangeClient          ecs.ECSSubmitStateSDK
	ec2metadata                      ec2.EC2MetadataClient
	httpClient                       *http.Client
	pollEndpointCache                async.TTLCache
	pollEndpointLock                 sync.Mutex
	isFIPSDetected                   bool
	shouldExcludeIPv4PortBinding     bool
	shouldExcludeIPv6PortBinding     bool
	sascCustomRetryBackoff           func(func() error) error
	stscAttachmentCustomRetryBackoff func(func() error) error
	metricsFactory                   metrics.EntryFactory
	rciRetryBackoff                  *retry.ExponentialBackoff
	availableMemoryProvider          func() int32
	isDualStackEnabled               bool
}

// NewECSClient creates a new ECSClient interface object.
func NewECSClient(
	credentialsCache *aws.CredentialsCache,
	configAccessor config.AgentConfigAccessor,
	ec2MetadataClient ec2.EC2MetadataClient,
	agentVer string,
	options ...ECSClientOption) (ecs.ECSClient, error) {
	client := &ecsClient{
		credentialsCache:  credentialsCache,
		configAccessor:    configAccessor,
		ec2metadata:       ec2MetadataClient,
		httpClient:        httpclient.New(RoundtripTimeout, configAccessor.AcceptInsecureCert(), agentVer, configAccessor.OSType(), configAccessor.OSFamily()),
		pollEndpointCache: async.NewTTLCache(&async.TTL{Duration: defaultPollEndpointCacheTTL}),
	}

	// Apply options to configure/override ECS client values.
	for _, opt := range options {
		opt(client)
	}

	ecsConfig, err := newECSConfig(client.credentialsCache, configAccessor, client.httpClient, client.isFIPSDetected, client.isDualStackEnabled)
	if err != nil {
		return nil, err
	}

	if client.standardClient == nil {
		client.standardClient = ecsservice.NewFromConfig(ecsConfig)
	}
	if client.submitStateChangeClient == nil {
		client.submitStateChangeClient = newSubmitStateChangeClient(ecsConfig)
	}
	if client.metricsFactory == nil {
		client.metricsFactory = metrics.NewNopEntryFactory()
	}
	if client.rciRetryBackoff == nil {
		client.rciRetryBackoff = retry.NewExponentialBackoff(rciMinBackoff, rciMaxBackoff, rciRetryJitter, rciRetryMultiple)
	}
	if client.availableMemoryProvider == nil {
		client.availableMemoryProvider = getHostMemoryInMiB
	}

	return client, nil
}

func newECSConfig(
	credentialsCache *aws.CredentialsCache,
	configAccessor config.AgentConfigAccessor,
	httpClient *http.Client,
	isFIPSEnabled, isDualStackEnabled bool,
) (aws.Config, error) {
	// We should respect the endpoint given (if any) because it could be the Gamma or Zeta endpoint of ECS service which
	// don't have the corresponding FIPS endpoints. Otherwise, when the host has FIPS enabled, we should tell SDK to
	// pick the FIPS endpoint.
	var endpointFn = func(_ *awsconfig.LoadOptions) error {
		return nil
	}
	fipsEndpointState := aws.FIPSEndpointStateUnset
	dualStackEndpointState := aws.DualStackEndpointStateUnset
	if configAccessor.APIEndpoint() != "" {
		endpointFn = awsconfig.WithBaseEndpoint(utils.AddScheme(configAccessor.APIEndpoint()))
	} else {
		if isFIPSEnabled {
			fipsEndpointState = aws.FIPSEndpointStateEnabled
		}
		if isDualStackEnabled {
			dualStackEndpointState = aws.DualStackEndpointStateEnabled
		}
	}

	ecsConfig, err := awsconfig.LoadDefaultConfig(
		context.TODO(),
		awsconfig.WithHTTPClient(httpClient),
		awsconfig.WithRegion(configAccessor.AWSRegion()),
		awsconfig.WithCredentialsProvider(credentialsCache),
		endpointFn,
		awsconfig.WithUseFIPSEndpoint(fipsEndpointState),
		awsconfig.WithUseDualStackEndpoint(dualStackEndpointState),
	)
	if err != nil {
		return aws.Config{}, err
	}
	return ecsConfig, nil
}

// CreateCluster creates a cluster from a given name and returns its ARN.
func (client *ecsClient) CreateCluster(clusterName string) (string, error) {
	resp, err := client.standardClient.CreateCluster(context.TODO(), &ecsservice.CreateClusterInput{ClusterName: &clusterName})
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
func (client *ecsClient) RegisterContainerInstance(containerInstanceArn string, attributes []types.Attribute,
	tags []types.Tag, registrationToken string, platformDevices []types.PlatformDevice,
	outpostARN string) (string, string, error) {

	clusterRef := client.configAccessor.Cluster()
	// If our clusterRef is empty, we should try to create the default.
	if clusterRef == "" {
		clusterRef = client.configAccessor.DefaultClusterName()
		defer client.configAccessor.UpdateCluster(clusterRef)
		// Attempt to register without checking existence of the cluster so that we don't require
		// excess permissions in the case where the cluster already exists and is active.
		containerInstanceArn, availabilityzone, err := client.registerContainerInstanceWithRetry(clusterRef,
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
	return client.registerContainerInstanceWithRetry(clusterRef, containerInstanceArn, attributes, tags, registrationToken,
		platformDevices, outpostARN)
}

// registerContainerInstanceWithRetry wraps around registerContainerInstance with exponential backoff retry implementation.
func (client *ecsClient) registerContainerInstanceWithRetry(clusterRef string, containerInstanceArn string,
	attributes []types.Attribute, tags []types.Tag, registrationToken string,
	platformDevices []types.PlatformDevice, outpostARN string) (string, string, error) {

	var containerInstanceARN, availabilityZone string
	var errFromRCI error
	ctx, cancel := context.WithTimeout(context.Background(), rciMaxRetryTimeAllowed)
	defer cancel()
	// Reset the backoff such that retries from past calls won't impact the current call.
	client.rciRetryBackoff.Reset()
	err := retry.RetryWithBackoffCtx(ctx, client.rciRetryBackoff,
		func() error {
			containerInstanceARN, availabilityZone, errFromRCI = client.registerContainerInstance(
				clusterRef, containerInstanceArn, attributes, tags, registrationToken, platformDevices, outpostARN)
			if errFromRCI != nil {
				if !isTransientError(errFromRCI) {
					logger.Error("Received terminal error from RegisterContainerInstance call, exiting", logger.Fields{
						field.Error: errFromRCI,
					})
					// Mark the error as non-retriable, to stop the retry loop in RetryWithBackoffCtx.
					return apierrors.NewRetriableError(apierrors.NewRetriable(false), errFromRCI)
				} else {
					logger.Error("Received non-terminal error from RegisterContainerInstance call, retrying with exponential backoff", logger.Fields{
						field.Error: errFromRCI,
					})
					// Mark non-terminal errors as retriable, to continue the retry loop in RetryWithBackoffCtx.
					return apierrors.NewRetriableError(apierrors.NewRetriable(true), errFromRCI)
				}
			}
			return nil
		})
	if err != nil {
		// return errFromRCI instead of err returned by the retry wrapper, as err wraps around the original error thrown by RCI.
		// errFromRCI has implementation to mark the exit code terminal, so that systemd won't restart the agent binary.
		return "", "", errFromRCI
	}
	return containerInstanceARN, availabilityZone, nil
}

func (client *ecsClient) registerContainerInstance(clusterRef string, containerInstanceArn string,
	attributes []types.Attribute, tags []types.Tag, registrationToken string,
	platformDevices []types.PlatformDevice, outpostARN string) (string, string, error) {

	registerRequest := ecsservice.RegisterContainerInstanceInput{Cluster: &clusterRef}
	var registrationAttributes []types.Attribute
	if containerInstanceArn != "" {
		// We are re-connecting a previously registered instance, restored from snapshot.
		registerRequest.ContainerInstanceArn = &containerInstanceArn
	} else {
		// This is a new instance, not previously registered.
		// Custom attribute registration only happens on initial instance registration.
		for _, attribute := range client.getCustomAttributes() {
			logger.Debug("Added a new custom attribute", logger.Fields{
				field.AttributeName:  aws.ToString(attribute.Name),
				field.AttributeValue: aws.ToString(attribute.Value),
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

	resources, err := client.getResources()
	if err != nil {
		return "", "", err
	}

	registerRequest.TotalResources = resources

	registerRequest.ClientToken = &registrationToken
	resp, err := client.standardClient.RegisterContainerInstance(context.TODO(), &registerRequest)
	if err != nil {
		logger.Error("Unable to register as a container instance with ECS", logger.Fields{
			field.Error: err,
		})
		return "", "", err
	}

	var availabilityzone = ""
	if resp != nil {
		for _, attr := range resp.ContainerInstance.Attributes {
			if aws.ToString(attr.Name) == azAttrName {
				availabilityzone = aws.ToString(attr.Value)
				break
			}
		}
	}

	logger.Info("Registered container instance with cluster!")
	err = validateRegisteredAttributes(registerRequest.Attributes, resp.ContainerInstance.Attributes)
	return aws.ToString(resp.ContainerInstance.ContainerInstanceArn), availabilityzone, err
}

func (client *ecsClient) setInstanceIdentity(
	registerRequest ecsservice.RegisterContainerInstanceInput) ecsservice.RegisterContainerInstanceInput {
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
			logger.Error("Unable to get instance identity document, retrying", logger.Fields{
				field.Error: attemptErr,
			})
			// Force credentials to expire in case they are stale but not expired.
			client.credentialsCache.Invalidate()
			if creds, err := client.credentialsCache.Retrieve(ctx); err != nil || !creds.HasKeys() {
				logger.Error("Unable to get valid credentials after invalidating credentials cache", logger.Fields{
					field.Error: err,
				})
			}
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

func attributesToMap(attributes []types.Attribute) map[string]string {
	attributeMap := make(map[string]string)
	attribs := attributes
	for _, attribute := range attribs {
		attributeMap[aws.ToString(attribute.Name)] = aws.ToString(attribute.Value)
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

func (client *ecsClient) getResources() ([]types.Resource, error) {
	// Below are micro-optimizations - the pointers to integerStr and stringSetStr are used multiple times below.
	integerStr := "INTEGER"
	stringSetStr := "STRINGSET"

	cpu := getCpu()
	mem := client.availableMemoryProvider()
	remainingMem := mem - int32(client.configAccessor.ReservedMemory())
	logger.Info("Remaining memory", logger.Fields{
		"remainingMemory": remainingMem,
	})
	if remainingMem < 0 {
		return nil, fmt.Errorf(
			"api register-container-instance: reserved memory is higher than available memory on the host, "+
				"total memory: %d, reserved: %d", mem, client.configAccessor.ReservedMemory())
	}

	cpuResource := types.Resource{
		Name:         aws.String("CPU"),
		Type:         &integerStr,
		IntegerValue: cpu,
	}
	memResource := types.Resource{
		Name:         aws.String("MEMORY"),
		Type:         &integerStr,
		IntegerValue: remainingMem,
	}
	portResource := types.Resource{
		Name:           aws.String("PORTS"),
		Type:           &stringSetStr,
		StringSetValue: utils.Uint16SliceToStringSlice(client.configAccessor.ReservedPorts()),
	}
	udpPortResource := types.Resource{
		Name:           aws.String("PORTS_UDP"),
		Type:           &stringSetStr,
		StringSetValue: utils.Uint16SliceToStringSlice(client.configAccessor.ReservedPortsUDP()),
	}

	return []types.Resource{cpuResource, memResource, portResource, udpPortResource}, nil
}

// GetHostResources calling getHostResources to get a list of CPU, MEMORY, PORTS and PORTS_UPD resources
// and return a resourceMap that map the resource name to each resource
func (client *ecsClient) GetHostResources() (map[string]types.Resource, error) {
	resources, err := client.getResources()
	if err != nil {
		return nil, err
	}
	resourceMap := make(map[string]types.Resource)
	for _, resource := range resources {
		if *resource.Name == "PORTS" {
			// Except for RCI, TCP Ports are named as PORTS_TCP in Agent for Host Resources purpose.
			resource.Name = aws.String("PORTS_TCP")
		}
		resourceMap[*resource.Name] = resource
	}
	return resourceMap, nil
}

func getCpu() int32 {
	cpu := utils.GetNumCPU() * 1024
	return int32(cpu)
}

func getHostMemoryInMiB() int32 {
	memInfo, err := meminfo.Read()
	mem := int32(0)
	if err == nil {
		mem = int32(memInfo.MemTotal / 1024 / 1024) // MiB
	} else {
		logger.Error("Unable to get memory info", logger.Fields{
			field.Error: err,
		})
	}

	return mem
}

func validateRegisteredAttributes(expectedAttributes, actualAttributes []types.Attribute) error {
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

func (client *ecsClient) getAdditionalAttributes() []types.Attribute {
	var attrs []types.Attribute

	// Add a check to ensure only non-empty values are added
	// to API call.
	if client.configAccessor.OSType() != "" {
		attrs = append(attrs, types.Attribute{
			Name:  aws.String(osTypeAttrName),
			Value: aws.String(client.configAccessor.OSType()),
		})
	}

	// OSFamily should be treated as an optional field as it is not applicable for all agents
	// using ecs client shared library. Add a check to ensure only non-empty values are added
	// to API call.
	if client.configAccessor.OSFamily() != "" {
		attrs = append(attrs, types.Attribute{
			Name:  aws.String(osFamilyAttrName),
			Value: aws.String(client.configAccessor.OSFamily()),
		})
	}
	// Send CPU arch attribute directly when running on external capacity. When running on EC2 or Fargate launch type,
	// this is not needed since the CPU arch is reported via instance identity document in those cases.
	if client.configAccessor.External() {
		attrs = append(attrs, types.Attribute{
			Name:  aws.String(cpuArchAttrName),
			Value: aws.String(getCPUArch()),
		})
	}
	return attrs
}

func (client *ecsClient) getOutpostAttribute(outpostARN string) []types.Attribute {
	if len(outpostARN) > 0 {
		return []types.Attribute{
			{
				Name:  aws.String("ecs.outpost-arn"),
				Value: aws.String(outpostARN),
			},
		}
	}
	return []types.Attribute{}
}

func (client *ecsClient) getCustomAttributes() []types.Attribute {
	var attributes []types.Attribute
	for attribute, value := range client.configAccessor.InstanceAttributes() {
		attributes = append(attributes, types.Attribute{
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

	clusterARN := client.configAccessor.Cluster()
	if len(change.ClusterARN) != 0 {
		clusterARN = change.ClusterARN
	}
	if change.Attachment != nil {
		// Confirm attachment by submitting attachment state change via SubmitTaskStateChange API (specifically in
		// the input's Attachments field).
		var attachments []types.AttachmentStateChange
		eniStatus := change.Attachment.Status.String()
		attachments = []types.AttachmentStateChange{
			{
				AttachmentArn: aws.String(change.Attachment.AttachmentARN),
				Status:        aws.String(eniStatus),
			},
		}

		_, err := client.submitStateChangeClient.SubmitTaskStateChange(context.TODO(), &ecsservice.SubmitTaskStateChangeInput{
			Cluster:     aws.String(clusterARN),
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

	req := ecsservice.SubmitTaskStateChangeInput{
		Cluster:            aws.String(clusterARN),
		Task:               aws.String(change.TaskARN),
		Status:             aws.String(change.Status.BackendStatus()),
		Reason:             aws.String(trimString(change.Reason, ecsMaxTaskReasonLength)),
		PullStartedAt:      change.PullStartedAt,
		PullStoppedAt:      change.PullStoppedAt,
		ExecutionStoppedAt: change.ExecutionStoppedAt,
		ManagedAgents:      formatManagedAgents(change.ManagedAgents),
		Containers: formatContainers(
			change.Containers,
			client.shouldExcludeIPv4PortBinding, client.shouldExcludeIPv6PortBinding,
			change.TaskARN),
	}

	_, err := client.submitStateChangeClient.SubmitTaskStateChange(context.TODO(), &req)
	if err != nil {
		logger.Error("Could not submit task state change", logger.Fields{
			field.Error:       err,
			"taskStateChange": change.String(),
		})
		return err
	}

	return nil
}

func (client *ecsClient) SubmitContainerStateChange(change ecs.ContainerStateChange) error {

	input := ecsservice.SubmitContainerStateChangeInput{
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
		exitCode := int32(aws.ToInt(change.ExitCode))
		input.ExitCode = aws.Int32(exitCode)
	}

	networkBindings := change.NetworkBindings
	if client.shouldExcludeIPv4PortBinding {
		networkBindings = excludeIPv4PortBindingFromNetworkBindings(networkBindings, change.ContainerName,
			change.TaskArn)
	}
	if client.shouldExcludeIPv6PortBinding {
		networkBindings = excludeIPv6PortBindingFromNetworkBindings(networkBindings, change.ContainerName,
			change.TaskArn)
	}
	input.NetworkBindings = networkBindings

	_, err := client.submitStateChangeClient.SubmitContainerStateChange(context.TODO(), &input)
	if err != nil {
		logger.Error("Could not submit container state change", logger.Fields{
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

	req := ecsservice.SubmitAttachmentStateChangesInput{
		Cluster: aws.String(client.configAccessor.Cluster()),
		Attachments: []types.AttachmentStateChange{
			{
				AttachmentArn: aws.String(change.Attachment.GetAttachmentARN()),
				Status:        aws.String(attachmentStatus.String()),
			},
		},
	}

	_, err := client.submitStateChangeClient.SubmitAttachmentStateChanges(context.TODO(), &req)
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
	for _, apiErr := range nonRetriableErrors {
		if errors.As(err, &apiErr) {
			retry = false
			break
		}
	}
	return apierrors.NewRetriableError(apierrors.NewRetriable(retry), err)
}

func (client *ecsClient) DiscoverPollEndpoint(containerInstanceArn string) (string, error) {
	resp, err := client.discoverPollEndpoint(containerInstanceArn, "")
	if err != nil {
		return "", err
	}
	if resp.Endpoint == nil {
		return "", errors.New("no endpoint returned; nil")
	}

	return aws.ToString(resp.Endpoint), nil
}

func (client *ecsClient) DiscoverTelemetryEndpoint(containerInstanceArn string) (string, error) {
	resp, err := client.discoverPollEndpoint(containerInstanceArn, "")
	if err != nil {
		return "", err
	}
	if resp.TelemetryEndpoint == nil {
		return "", errors.New("no telemetry endpoint returned; nil")
	}

	return aws.ToString(resp.TelemetryEndpoint), nil
}

func (client *ecsClient) DiscoverServiceConnectEndpoint(containerInstanceArn string) (string, error) {
	resp, err := client.discoverPollEndpoint(containerInstanceArn, "")
	if err != nil {
		return "", err
	}
	if resp.ServiceConnectEndpoint == nil {
		return "", errors.New("no ServiceConnect endpoint returned; nil")
	}

	return aws.ToString(resp.ServiceConnectEndpoint), nil
}

func (client *ecsClient) DiscoverSystemLogsEndpoint(containerInstanceArn string, availabilityZone string) (string,
	error) {
	resp, err := client.discoverPollEndpoint(containerInstanceArn, availabilityZone)
	if err != nil {
		return "", err
	}
	if resp.SystemLogsEndpoint == nil {
		return "", errors.New("no system logs endpoint returned; nil")
	}

	return aws.ToString(resp.SystemLogsEndpoint), nil
}

func (client *ecsClient) discoverPollEndpoint(containerInstanceArn string,
	availabilityZone string) (*ecsservice.DiscoverPollEndpointOutput, error) {
	client.pollEndpointLock.Lock()
	defer client.pollEndpointLock.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), discoverPollEndpointTimeout)
	defer cancel()
	// Try getting an entry from the cache.
	cachedEndpoint, expired, found := client.pollEndpointCache.Get(containerInstanceArn)
	if !expired && found {
		// Cache hit and not expired. Return the output.
		output, ok := cachedEndpoint.(*ecsservice.DiscoverPollEndpointOutput)
		systemLogsEndpoint := aws.ToString(output.SystemLogsEndpoint)
		if ok {
			// Presence of the system logs endpoint can be disregarded if the AZ was not provided,
			// but the cache hit must include a non-empty system logs endpoint if the AZ was provided.
			if availabilityZone == "" || (availabilityZone != "" && systemLogsEndpoint != "") {
				logger.Info("Using cached DiscoverPollEndpoint", logger.Fields{
					field.Endpoint:               aws.ToString(output.Endpoint),
					field.TelemetryEndpoint:      aws.ToString(output.TelemetryEndpoint),
					field.ServiceConnectEndpoint: aws.ToString(output.ServiceConnectEndpoint),
					field.SystemLogsEndpoint:     systemLogsEndpoint,
					field.ContainerInstanceARN:   containerInstanceArn,
				})
				return output, nil
			}
		}
	}
	discoverPollEndpointStartTime := time.Now()
	// Cache miss or expired, invoke the ECS DiscoverPollEndpoint API.
	logger.Debug("Invoking DiscoverPollEndpoint", logger.Fields{
		field.ContainerInstanceARN: containerInstanceArn,
		field.AvailabilityZone:     availabilityZone,
	})
	output, err := client.standardClient.DiscoverPollEndpoint(ctx, &ecsservice.DiscoverPollEndpointInput{
		ContainerInstance: &containerInstanceArn,
		Cluster:           aws.String(client.configAccessor.Cluster()),
		ZoneId:            aws.String(availabilityZone),
	})
	client.metricsFactory.New(metrics.DiscoverPollEndpointFailure).Done(err)
	client.metricsFactory.New(metrics.DiscoverPollEndpointTotal).Done(nil)
	if err != nil {
		// If we got an error calling the API, fallback to an expired cached endpoint if
		// we have it.
		if expired {
			if output, ok := cachedEndpoint.(*ecsservice.DiscoverPollEndpointOutput); ok {
				logger.Info("Error calling DiscoverPollEndpoint. Using cached-but-expired endpoint as a fallback.",
					logger.Fields{
						field.Endpoint:               aws.ToString(output.Endpoint),
						field.TelemetryEndpoint:      aws.ToString(output.TelemetryEndpoint),
						field.ServiceConnectEndpoint: aws.ToString(output.ServiceConnectEndpoint),
						field.SystemLogsEndpoint:     aws.ToString(output.SystemLogsEndpoint),
						field.ContainerInstanceARN:   containerInstanceArn,
					})
				return output, nil
			}
		}
		return nil, err
	}
	client.metricsFactory.New(metrics.DiscoverPollEndpointDurationName).WithGauge(time.Since(discoverPollEndpointStartTime).Milliseconds()).Done(nil)
	// Cache the response from ECS.
	client.pollEndpointCache.Set(containerInstanceArn, output)
	return output, nil
}

func (client *ecsClient) GetResourceTags(resourceArn string) ([]types.Tag, error) {
	output, err := client.standardClient.ListTagsForResource(context.TODO(), &ecsservice.ListTagsForResourceInput{
		ResourceArn: &resourceArn,
	})
	if err != nil {
		return nil, err
	}
	return output.Tags, nil
}

func (client *ecsClient) UpdateContainerInstancesState(instanceARN string, status types.ContainerInstanceStatus) error {
	logger.Debug("Invoking UpdateContainerInstancesState", logger.Fields{
		field.Status:               status,
		field.ContainerInstanceARN: instanceARN,
	})
	_, err := client.standardClient.UpdateContainerInstancesState(context.TODO(), &ecsservice.UpdateContainerInstancesStateInput{
		ContainerInstances: []string{instanceARN},
		Status:             status,
		Cluster:            aws.String(client.configAccessor.Cluster()),
	})
	return err
}

func formatManagedAgents(managedAgents []types.ManagedAgentStateChange) []types.ManagedAgentStateChange {
	var result []types.ManagedAgentStateChange
	for _, m := range managedAgents {
		if m.Reason != nil {
			m.Reason = trimStringPtr(m.Reason, ecsMaxContainerReasonLength)
		}
		result = append(result, m)
	}
	return result
}

func formatContainers(containers []types.ContainerStateChange,
	shouldExcludeIPv4PortBinding, shouldExcludeIPv6PortBinding bool, taskARN string,
) []types.ContainerStateChange {
	var result []types.ContainerStateChange
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
		if shouldExcludeIPv4PortBinding {
			c.NetworkBindings = excludeIPv4PortBindingFromNetworkBindings(c.NetworkBindings,
				aws.ToString(c.ContainerName), taskARN)
		}
		if shouldExcludeIPv6PortBinding {
			c.NetworkBindings = excludeIPv6PortBindingFromNetworkBindings(c.NetworkBindings,
				aws.ToString(c.ContainerName), taskARN)
		}
		result = append(result, c)
	}
	return result
}

func excludeIPv4PortBindingFromNetworkBindings(networkBindings []types.NetworkBinding, containerName,
	taskARN string) []types.NetworkBinding {
	return excludePortBindingFromNetworkBindings("0.0.0.0", networkBindings, containerName, taskARN)
}

func excludeIPv6PortBindingFromNetworkBindings(networkBindings []types.NetworkBinding, containerName,
	taskARN string) []types.NetworkBinding {
	return excludePortBindingFromNetworkBindings("::", networkBindings, containerName, taskARN)
}

func excludePortBindingFromNetworkBindings(
	bindIPToExclude string, networkBindings []types.NetworkBinding, containerName, taskARN string,
) []types.NetworkBinding {
	var result []types.NetworkBinding
	for _, binding := range networkBindings {
		if aws.ToString(binding.BindIP) == bindIPToExclude {
			logger.Debug("Exclude port binding", logger.Fields{
				"portBinding":       binding,
				"bindIP":            bindIPToExclude,
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
	return aws.String(trimString(aws.ToString(inputStringPtr), maxLen))
}

func trimString(inputString string, maxLen int) string {
	if len(inputString) > maxLen {
		trimmed := inputString[0:maxLen]
		return trimmed
	} else {
		return inputString
	}
}

func isTransientError(err error) bool {
	var apiErr smithy.APIError
	// Using errors.As to unwrap as opposed to errors.Is.
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case apierrors.ErrCodeServerException, apierrors.ErrCodeLimitExceededException:
			return true
		}
	}
	return false
}
