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
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/docker/go-connections/nat"

	"github.com/aws/amazon-ecs-agent/agent/logger"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	"github.com/aws/amazon-ecs-agent/agent/async"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/httpclient"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/cihub/seelog"
	"github.com/docker/docker/pkg/system"
)

const (
	ecsMaxImageDigestLength     = 255
	ecsMaxContainerReasonLength = 255
	ecsMaxTaskReasonLength      = 1024
	ecsMaxRuntimeIDLength       = 255
	pollEndpointCacheTTL        = 12 * time.Hour
	azAttrName                  = "ecs.availability-zone"
	cpuArchAttrName             = "ecs.cpu-architecture"
	osTypeAttrName              = "ecs.os-type"
	osFamilyAttrName            = "ecs.os-family"
	RoundtripTimeout            = 5 * time.Second
	// NetworkModeAWSVPC specifies the awsvpc network mode.
	networkModeAWSVPC = "awsvpc"
	// NetworkModeHost specifies the host network mode.
	networkModeHost = "host"
	// ecsMaxNetworkBindingsLength is the maximum length of the ecs.NetworkBindings list sent as part of the
	// container state change payload. Currently, this is enforced only when containerPortRanges are requested.
	ecsMaxNetworkBindingsLength = 100
)

// APIECSClient implements ECSClient
type APIECSClient struct {
	credentialProvider      *credentials.Credentials
	config                  *config.Config
	standardClient          api.ECSSDK
	submitStateChangeClient api.ECSSubmitStateSDK
	ec2metadata             ec2.EC2MetadataClient
	pollEndpointCache       async.TTLCache
}

// NewECSClient creates a new ECSClient interface object
func NewECSClient(
	credentialProvider *credentials.Credentials,
	config *config.Config,
	ec2MetadataClient ec2.EC2MetadataClient) api.ECSClient {

	var ecsConfig aws.Config
	ecsConfig.Credentials = credentialProvider
	ecsConfig.Region = &config.AWSRegion
	ecsConfig.HTTPClient = httpclient.New(RoundtripTimeout, config.AcceptInsecureCert)
	if config.APIEndpoint != "" {
		ecsConfig.Endpoint = &config.APIEndpoint
	}
	standardClient := ecs.New(session.New(&ecsConfig))
	submitStateChangeClient := newSubmitStateChangeClient(&ecsConfig)
	return &APIECSClient{
		credentialProvider:      credentialProvider,
		config:                  config,
		standardClient:          standardClient,
		submitStateChangeClient: submitStateChangeClient,
		ec2metadata:             ec2MetadataClient,
		pollEndpointCache:       async.NewTTLCache(pollEndpointCacheTTL),
	}
}

// SetSDK overrides the SDK to the given one. This is useful for injecting a
// test implementation
func (client *APIECSClient) SetSDK(sdk api.ECSSDK) {
	client.standardClient = sdk
}

// SetSubmitStateChangeSDK overrides the SDK to the given one. This is useful
// for injecting a test implementation
func (client *APIECSClient) SetSubmitStateChangeSDK(sdk api.ECSSubmitStateSDK) {
	client.submitStateChangeClient = sdk
}

// CreateCluster creates a cluster from a given name and returns its arn
func (client *APIECSClient) CreateCluster(clusterName string) (string, error) {
	resp, err := client.standardClient.CreateCluster(&ecs.CreateClusterInput{ClusterName: &clusterName})
	if err != nil {
		seelog.Criticalf("Could not create cluster: %v", err)
		return "", err
	}
	seelog.Infof("Created a cluster named: %s", clusterName)
	return *resp.Cluster.ClusterName, nil
}

// RegisterContainerInstance calculates the appropriate resources, creates
// the default cluster if necessary, and returns the registered
// ContainerInstanceARN if successful. Supplying a non-empty container
// instance ARN allows a container instance to update its registered
// resources.
func (client *APIECSClient) RegisterContainerInstance(containerInstanceArn string, attributes []*ecs.Attribute,
	tags []*ecs.Tag, registrationToken string, platformDevices []*ecs.PlatformDevice,
	outpostARN string) (string, string, error) {

	clusterRef := client.config.Cluster
	// If our clusterRef is empty, we should try to create the default
	if clusterRef == "" {
		clusterRef = config.DefaultClusterName
		defer func() {
			// Update the config value to reflect the cluster we end up in
			client.config.Cluster = clusterRef
		}()
		// Attempt to register without checking existence of the cluster so we don't require
		// excess permissions in the case where the cluster already exists and is active
		containerInstanceArn, availabilityzone, err := client.registerContainerInstance(clusterRef,
			containerInstanceArn, attributes, tags, registrationToken, platformDevices, outpostARN)
		if err == nil {
			return containerInstanceArn, availabilityzone, nil
		}

		// If trying to register fails because the default cluster doesn't exist, try to create the cluster before calling
		// register again
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

func (client *APIECSClient) registerContainerInstance(clusterRef string, containerInstanceArn string,
	attributes []*ecs.Attribute, tags []*ecs.Tag, registrationToken string,
	platformDevices []*ecs.PlatformDevice, outpostARN string) (string, string, error) {

	registerRequest := ecs.RegisterContainerInstanceInput{Cluster: &clusterRef}
	var registrationAttributes []*ecs.Attribute
	if containerInstanceArn != "" {
		// We are re-connecting a previously registered instance, restored from snapshot.
		registerRequest.ContainerInstanceArn = &containerInstanceArn
	} else {
		// This is a new instance, not previously registered.
		// Custom attribute registration only happens on initial instance registration.
		for _, attribute := range client.getCustomAttributes() {
			seelog.Debugf("Added a new custom attribute %v=%v",
				aws.StringValue(attribute.Name),
				aws.StringValue(attribute.Value),
			)
			registrationAttributes = append(registrationAttributes, attribute)
		}
	}
	// Standard attributes are included with all registrations.
	registrationAttributes = append(registrationAttributes, attributes...)

	// Add additional attributes such as the os type
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
	resp, err := client.standardClient.RegisterContainerInstance(&registerRequest)
	if err != nil {
		seelog.Errorf("Unable to register as a container instance with ECS: %v", err)
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

	seelog.Info("Registered container instance with cluster!")
	err = validateRegisteredAttributes(registerRequest.Attributes, resp.ContainerInstance.Attributes)
	return aws.StringValue(resp.ContainerInstance.ContainerInstanceArn), availabilityzone, err
}

func (client *APIECSClient) setInstanceIdentity(registerRequest ecs.RegisterContainerInstanceInput) ecs.RegisterContainerInstanceInput {
	instanceIdentityDoc := ""
	instanceIdentitySignature := ""

	if client.config.NoIID {
		seelog.Info("Fetching Instance ID Document has been disabled")
		registerRequest.InstanceIdentityDocument = &instanceIdentityDoc
		registerRequest.InstanceIdentityDocumentSignature = &instanceIdentitySignature
		return registerRequest
	}

	iidRetrieved := true
	instanceIdentityDoc, err := client.ec2metadata.GetDynamicData(ec2.InstanceIdentityDocumentResource)
	if err != nil {
		seelog.Errorf("Unable to get instance identity document: %v", err)
		iidRetrieved = false
	}
	registerRequest.InstanceIdentityDocument = &instanceIdentityDoc

	if iidRetrieved {
		instanceIdentitySignature, err = client.ec2metadata.GetDynamicData(ec2.InstanceIdentityDocumentSignatureResource)
		if err != nil {
			seelog.Errorf("Unable to get instance identity signature: %v", err)
		}
	}

	registerRequest.InstanceIdentityDocumentSignature = &instanceIdentitySignature
	return registerRequest
}

func attributesToMap(attributes []*ecs.Attribute) map[string]string {
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
			seelog.Tracef("Response contained expected value for attribute %v", key)
		}
	}
	if len(missingAttributes) > 0 {
		err = apierrors.NewAttributeError("Attribute validation failed")
	}
	return missingAttributes, err
}

func (client *APIECSClient) getResources() ([]*ecs.Resource, error) {
	// Micro-optimization, the pointer to this is used multiple times below
	integerStr := "INTEGER"

	cpu, mem := getCpuAndMemory()
	remainingMem := mem - int64(client.config.ReservedMemory)
	seelog.Infof("Remaining mem: %d", remainingMem)
	if remainingMem < 0 {
		return nil, fmt.Errorf(
			"api register-container-instance: reserved memory is higher than available memory on the host, total memory: %d, reserved: %d",
			mem, client.config.ReservedMemory)
	}

	cpuResource := ecs.Resource{
		Name:         utils.Strptr("CPU"),
		Type:         &integerStr,
		IntegerValue: &cpu,
	}
	memResource := ecs.Resource{
		Name:         utils.Strptr("MEMORY"),
		Type:         &integerStr,
		IntegerValue: &remainingMem,
	}
	portResource := ecs.Resource{
		Name:           utils.Strptr("PORTS"),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: utils.Uint16SliceToStringSlice(client.config.ReservedPorts),
	}
	udpPortResource := ecs.Resource{
		Name:           utils.Strptr("PORTS_UDP"),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: utils.Uint16SliceToStringSlice(client.config.ReservedPortsUDP),
	}

	return []*ecs.Resource{&cpuResource, &memResource, &portResource, &udpPortResource}, nil
}

func getCpuAndMemory() (int64, int64) {
	memInfo, err := system.ReadMemInfo()
	mem := int64(0)
	if err == nil {
		mem = memInfo.MemTotal / 1024 / 1024 // MiB
	} else {
		seelog.Errorf("Unable to get memory info: %v", err)
	}

	cpu := runtime.NumCPU() * 1024

	return int64(cpu), mem
}

func validateRegisteredAttributes(expectedAttributes, actualAttributes []*ecs.Attribute) error {
	var err error
	expectedAttributesMap := attributesToMap(expectedAttributes)
	actualAttributesMap := attributesToMap(actualAttributes)
	missingAttributes, err := findMissingAttributes(expectedAttributesMap, actualAttributesMap)
	if err != nil {
		msg := strings.Join(missingAttributes, ",")
		seelog.Errorf("Error registering attributes: %v", msg)
	}
	return err
}

func (client *APIECSClient) getAdditionalAttributes() []*ecs.Attribute {
	attrs := []*ecs.Attribute{
		{
			Name:  aws.String(osTypeAttrName),
			Value: aws.String(config.OSType),
		},
		{
			Name:  aws.String(osFamilyAttrName),
			Value: aws.String(config.GetOSFamily()),
		},
	}
	// Send cpu arch attribute directly when running on external capacity. When running on EC2, this is not needed
	// since the cpu arch is reported via instance identity doc in that case.
	if client.config.External.Enabled() {
		attrs = append(attrs, &ecs.Attribute{
			Name:  aws.String(cpuArchAttrName),
			Value: aws.String(getCPUArch()),
		})
	}
	return attrs
}

func (client *APIECSClient) getOutpostAttribute(outpostARN string) []*ecs.Attribute {
	if len(outpostARN) > 0 {
		return []*ecs.Attribute{
			{
				Name:  aws.String("ecs.outpost-arn"),
				Value: aws.String(outpostARN),
			},
		}
	}
	return []*ecs.Attribute{}
}

func (client *APIECSClient) getCustomAttributes() []*ecs.Attribute {
	var attributes []*ecs.Attribute
	for attribute, value := range client.config.InstanceAttributes {
		attributes = append(attributes, &ecs.Attribute{
			Name:  aws.String(attribute),
			Value: aws.String(value),
		})
	}
	return attributes
}

func (client *APIECSClient) SubmitTaskStateChange(change api.TaskStateChange) error {
	// Submit attachment state change
	if change.Attachment != nil {
		var attachments []*ecs.AttachmentStateChange

		eniStatus := change.Attachment.Status.String()
		attachments = []*ecs.AttachmentStateChange{
			{
				AttachmentArn: aws.String(change.Attachment.AttachmentARN),
				Status:        aws.String(eniStatus),
			},
		}

		_, err := client.submitStateChangeClient.SubmitTaskStateChange(&ecs.SubmitTaskStateChangeInput{
			Cluster:     aws.String(client.config.Cluster),
			Task:        aws.String(change.TaskARN),
			Attachments: attachments,
		})
		if err != nil {
			seelog.Warnf("Could not submit an attachment state change: %v", err)
			return err
		}

		return nil
	}

	status := change.Status.BackendStatus()

	req := ecs.SubmitTaskStateChangeInput{
		Cluster:            aws.String(client.config.Cluster),
		Task:               aws.String(change.TaskARN),
		Status:             aws.String(status),
		Reason:             aws.String(trimString(change.Reason, ecsMaxTaskReasonLength)),
		PullStartedAt:      change.PullStartedAt,
		PullStoppedAt:      change.PullStoppedAt,
		ExecutionStoppedAt: change.ExecutionStoppedAt,
	}

	for _, managedAgentEvent := range change.ManagedAgents {
		if mgspl := client.buildManagedAgentStateChangePayload(managedAgentEvent); mgspl != nil {
			req.ManagedAgents = append(req.ManagedAgents, mgspl)
		}
	}

	containerEvents := make([]*ecs.ContainerStateChange, len(change.Containers))
	for i, containerEvent := range change.Containers {
		payload, err := client.buildContainerStateChangePayload(containerEvent, client.config.ShouldExcludeIPv6PortBinding.Enabled())
		if err != nil {
			seelog.Errorf("Could not submit task state change: [%s]: %v", change.String(), err)
			return err
		}
		containerEvents[i] = payload
	}

	req.Containers = containerEvents

	_, err := client.submitStateChangeClient.SubmitTaskStateChange(&req)
	if err != nil {
		seelog.Warnf("Could not submit task state change: [%s]: %v", change.String(), err)
		return err
	}

	return nil
}

func trimString(inputString string, maxLen int) string {
	if len(inputString) > maxLen {
		trimmed := inputString[0:maxLen]
		return trimmed
	} else {
		return inputString
	}
}

func (client *APIECSClient) buildManagedAgentStateChangePayload(change api.ManagedAgentStateChange) *ecs.ManagedAgentStateChange {
	if !change.Status.ShouldReportToBackend() {
		seelog.Warnf("Not submitting unsupported managed agent state %s for container %s in task %s",
			change.Status.String(), change.Container.Name, change.TaskArn)
		return nil
	}
	var trimmedReason *string
	if change.Reason != "" {
		trimmedReason = aws.String(trimString(change.Reason, ecsMaxContainerReasonLength))
	}
	return &ecs.ManagedAgentStateChange{
		ManagedAgentName: aws.String(change.Name),
		ContainerName:    aws.String(change.Container.Name),
		Status:           aws.String(change.Status.String()),
		Reason:           trimmedReason,
	}
}

func (client *APIECSClient) buildContainerStateChangePayload(change api.ContainerStateChange, shouldExcludeIPv6PortBinding bool) (*ecs.ContainerStateChange, error) {
	statechange := &ecs.ContainerStateChange{
		ContainerName: aws.String(change.ContainerName),
	}
	if change.RuntimeID != "" {
		trimmedRuntimeID := trimString(change.RuntimeID, ecsMaxRuntimeIDLength)
		statechange.RuntimeId = aws.String(trimmedRuntimeID)
	}
	if change.Reason != "" {
		trimmedReason := trimString(change.Reason, ecsMaxContainerReasonLength)
		statechange.Reason = aws.String(trimmedReason)
	}
	if change.ImageDigest != "" {
		trimmedImageDigest := trimString(change.ImageDigest, ecsMaxImageDigestLength)
		statechange.ImageDigest = aws.String(trimmedImageDigest)
	}
	status := change.Status

	if status != apicontainerstatus.ContainerStopped && status != apicontainerstatus.ContainerRunning {
		seelog.Warnf("Not submitting unsupported upstream container state %s for container %s in task %s",
			status.String(), change.ContainerName, change.TaskArn)
		return nil, nil
	}
	stat := change.Status.String()
	if stat == "DEAD" {
		stat = apicontainerstatus.ContainerStopped.String()
	}
	statechange.Status = aws.String(stat)

	if change.ExitCode != nil {
		exitCode := int64(aws.IntValue(change.ExitCode))
		statechange.ExitCode = aws.Int64(exitCode)
	}

	networkBindings := getNetworkBindings(change, shouldExcludeIPv6PortBinding)
	// we enforce a limit on the no.of network bindings for containers with at-least 1 port range requested.
	// this limit is enforced by ECS, and we fail early and don't call SubmitContainerStateChange.
	if change.Container.GetContainerHasPortRange() && len(networkBindings) > ecsMaxNetworkBindingsLength {
		return nil, fmt.Errorf("no. of network bindings %v is more than the maximum supported no. %v, "+
			"container: %s "+"task: %s", len(networkBindings), ecsMaxNetworkBindingsLength, change.ContainerName, change.TaskArn)
	}
	statechange.NetworkBindings = networkBindings

	return statechange, nil
}

// ProtocolBindIP used to store protocol and bindIP information associated to a particular host port
type ProtocolBindIP struct {
	protocol string
	bindIP   string
}

// getNetworkBindings returns the list of networkingBindings, sent to ECS as part of the container state change payload
func getNetworkBindings(change api.ContainerStateChange, shouldExcludeIPv6PortBinding bool) []*ecs.NetworkBinding {
	networkBindings := []*ecs.NetworkBinding{}
	// we do not return any network bindings for awsvpc and host network mode tasks.
	if change.Container.GetNetworkMode() == networkModeAWSVPC || change.Container.GetNetworkMode() == networkModeHost {
		return networkBindings
	}
	// hostPortToProtocolBindIPMap is a map to store protocol and bindIP information associated to host ports
	// that belong to a range. This is used in case when there are multiple protocol/bindIP combinations associated to a
	// port binding. example: when both IPv4 and IPv6 bindIPs are populated by docker and shouldExcludeIPv6PortBinding is false.
	hostPortToProtocolBindIPMap := map[int64][]ProtocolBindIP{}

	containerPortSet := change.Container.GetContainerPortSet()
	containerPortRangeMap := change.Container.GetContainerPortRangeMap()

	for _, binding := range change.PortBindings {
		if binding.BindIP == "::" && shouldExcludeIPv6PortBinding {
			seelog.Debugf("Exclude IPv6 port binding %v for container %s in task %s", binding, change.ContainerName, change.TaskArn)
			continue
		}

		hostPort := int64(binding.HostPort)
		containerPort := int64(aws.Uint16Value(binding.ContainerPort))
		bindIP := binding.BindIP
		protocol := binding.Protocol.String()

		// create network binding for each containerPort that exists in the singular ContainerPortSet
		// for container ports that belong to a range, we'll have 1 consolidated network binding for the range
		if _, ok := containerPortSet[int(containerPort)]; ok {
			networkBindings = append(networkBindings, &ecs.NetworkBinding{
				BindIP:        aws.String(bindIP),
				ContainerPort: aws.Int64(containerPort),
				HostPort:      aws.Int64(hostPort),
				Protocol:      aws.String(protocol),
			})
		} else {
			// populate hostPortToProtocolBindIPMap – this is used below when we construct network binding for ranges.
			hostPortToProtocolBindIPMap[hostPort] = append(hostPortToProtocolBindIPMap[hostPort],
				ProtocolBindIP{
					protocol: protocol,
					bindIP:   bindIP,
				})
		}
	}

	for containerPortRange, hostPortRange := range containerPortRangeMap {
		// we check for protocol and bindIP information associated to any one of the host ports from the hostPortRange,
		// all ports belonging to the same range share this information.
		hostPort, _, _ := nat.ParsePortRangeToInt(hostPortRange)
		if val, ok := hostPortToProtocolBindIPMap[int64(hostPort)]; ok {
			for _, v := range val {
				networkBindings = append(networkBindings, &ecs.NetworkBinding{
					BindIP:             aws.String(v.bindIP),
					ContainerPortRange: aws.String(containerPortRange),
					HostPortRange:      aws.String(hostPortRange),
					Protocol:           aws.String(v.protocol),
				})
			}
		}
	}

	return networkBindings
}

func (client *APIECSClient) SubmitContainerStateChange(change api.ContainerStateChange) error {
	pl, err := client.buildContainerStateChangePayload(change, client.config.ShouldExcludeIPv6PortBinding.Enabled())
	if err != nil {
		seelog.Errorf("Could not build container state change payload: [%s]: %v", change.String(), err)
		return err
	} else if pl == nil {
		return nil
	}

	_, err = client.submitStateChangeClient.SubmitContainerStateChange(&ecs.SubmitContainerStateChangeInput{
		Cluster:         aws.String(client.config.Cluster),
		ContainerName:   aws.String(change.ContainerName),
		ExitCode:        pl.ExitCode,
		ManagedAgents:   pl.ManagedAgents,
		NetworkBindings: pl.NetworkBindings,
		Reason:          pl.Reason,
		RuntimeId:       pl.RuntimeId,
		Status:          pl.Status,
		Task:            aws.String(change.TaskArn),
	})
	if err != nil {
		seelog.Warnf("Could not submit container state change: [%s]: %v", change.String(), err)
		return err
	}
	return nil
}

func (client *APIECSClient) SubmitAttachmentStateChange(change api.AttachmentStateChange) error {
	attachmentStatus := change.Attachment.Status.String()

	req := ecs.SubmitAttachmentStateChangesInput{
		Cluster: &client.config.Cluster,
		Attachments: []*ecs.AttachmentStateChange{
			{
				AttachmentArn: aws.String(change.Attachment.AttachmentARN),
				Status:        aws.String(attachmentStatus),
			},
		},
	}

	_, err := client.submitStateChangeClient.SubmitAttachmentStateChanges(&req)
	if err != nil {
		seelog.Warnf("Could not submit attachment state change [%s]: %v", change.String(), err)
		return err
	}

	return nil
}

func (client *APIECSClient) DiscoverPollEndpoint(containerInstanceArn string) (string, error) {
	resp, err := client.discoverPollEndpoint(containerInstanceArn)
	if err != nil {
		return "", err
	}

	return aws.StringValue(resp.Endpoint), nil
}

func (client *APIECSClient) DiscoverTelemetryEndpoint(containerInstanceArn string) (string, error) {
	resp, err := client.discoverPollEndpoint(containerInstanceArn)
	if err != nil {
		return "", err
	}
	if resp.TelemetryEndpoint == nil {
		return "", errors.New("No telemetry endpoint returned; nil")
	}

	return aws.StringValue(resp.TelemetryEndpoint), nil
}

func (client *APIECSClient) DiscoverServiceConnectEndpoint(containerInstanceArn string) (string, error) {
	resp, err := client.discoverPollEndpoint(containerInstanceArn)
	if err != nil {
		return "", err
	}
	if resp.ServiceConnectEndpoint == nil {
		return "", errors.New("No ServiceConnect endpoint returned; nil")
	}

	return aws.StringValue(resp.ServiceConnectEndpoint), nil
}

func (client *APIECSClient) discoverPollEndpoint(containerInstanceArn string) (*ecs.DiscoverPollEndpointOutput, error) {
	// Try getting an entry from the cache
	cachedEndpoint, expired, found := client.pollEndpointCache.Get(containerInstanceArn)
	if !expired && found {
		// Cache hit and not expired. Return the output.
		if output, ok := cachedEndpoint.(*ecs.DiscoverPollEndpointOutput); ok {
			logger.Info("Using cached DiscoverPollEndpoint", logger.Fields{
				"endpoint":               aws.StringValue(output.Endpoint),
				"telemetryEndpoint":      aws.StringValue(output.TelemetryEndpoint),
				"serviceConnectEndpoint": aws.StringValue(output.ServiceConnectEndpoint),
				"containerInstanceARN":   containerInstanceArn,
			})
			return output, nil
		}
	}

	// Cache miss or expired, invoke the ECS DiscoverPollEndpoint API.
	seelog.Debugf("Invoking DiscoverPollEndpoint for '%s'", containerInstanceArn)
	output, err := client.standardClient.DiscoverPollEndpoint(&ecs.DiscoverPollEndpointInput{
		ContainerInstance: &containerInstanceArn,
		Cluster:           &client.config.Cluster,
	})
	if err != nil {
		// if we got an error calling the API, fallback to an expired cached endpoint if
		// we have it.
		if expired {
			if output, ok := cachedEndpoint.(*ecs.DiscoverPollEndpointOutput); ok {
				logger.Info("Error calling DiscoverPollEndpoint. Using cached-but-expired endpoint as a fallback.", logger.Fields{
					"endpoint":               aws.StringValue(output.Endpoint),
					"telemetryEndpoint":      aws.StringValue(output.TelemetryEndpoint),
					"serviceConnectEndpoint": aws.StringValue(output.ServiceConnectEndpoint),
					"containerInstanceARN":   containerInstanceArn,
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

func (client *APIECSClient) GetResourceTags(resourceArn string) ([]*ecs.Tag, error) {
	output, err := client.standardClient.ListTagsForResource(&ecs.ListTagsForResourceInput{
		ResourceArn: &resourceArn,
	})
	if err != nil {
		return nil, err
	}
	return output.Tags, nil
}

func (client *APIECSClient) UpdateContainerInstancesState(instanceARN string, status string) error {
	seelog.Debugf("Invoking UpdateContainerInstancesState, status='%s' instanceARN='%s'", status, instanceARN)
	_, err := client.standardClient.UpdateContainerInstancesState(&ecs.UpdateContainerInstancesStateInput{
		ContainerInstances: []*string{aws.String(instanceARN)},
		Status:             aws.String(status),
		Cluster:            &client.config.Cluster,
	})
	return err
}
