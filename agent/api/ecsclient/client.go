// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"net/http"
	"runtime"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/async"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/cihub/seelog"
	"github.com/docker/docker/pkg/system"
)

const (
	ecsMaxReasonLength    = 255
	pollEndpointCacheSize = 1
	pollEndpointCacheTTL  = 20 * time.Minute
	RoundtripTimeout      = 5 * time.Second
)

var log = logger.ForModule("api client")

// APIECSClient implements ECSClient
type APIECSClient struct {
	credentialProvider      *credentials.Credentials
	config                  *config.Config
	standardClient          api.ECSSDK
	submitStateChangeClient api.ECSSubmitStateSDK
	ec2metadata             ec2.EC2MetadataClient
	pollEndpoinCache        async.Cache
}

func NewECSClient(credentialProvider *credentials.Credentials, config *config.Config, httpClient *http.Client, ec2MetadataClient ec2.EC2MetadataClient) api.ECSClient {
	var ecsConfig aws.Config
	ecsConfig.Credentials = credentialProvider
	ecsConfig.Region = &config.AWSRegion
	ecsConfig.HTTPClient = httpClient
	if config.APIEndpoint != "" {
		ecsConfig.Endpoint = &config.APIEndpoint
	}
	standardClient := ecs.New(session.New(&ecsConfig))
	submitStateChangeClient := newSubmitStateChangeClient(&ecsConfig)
	pollEndpoinCache := async.NewLRUCache(pollEndpointCacheSize, pollEndpointCacheTTL)
	return &APIECSClient{
		credentialProvider:      credentialProvider,
		config:                  config,
		standardClient:          standardClient,
		submitStateChangeClient: submitStateChangeClient,
		ec2metadata:             ec2MetadataClient,
		pollEndpoinCache:        pollEndpoinCache,
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
		log.Crit("Could not register", "err", err)
		return "", err
	}
	log.Info("Created a cluster!", "clusterName", clusterName)
	return *resp.Cluster.ClusterName, nil
}

func (client *APIECSClient) RegisterContainerInstance(containerInstanceArn string, attributes []string) (string, error) {
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
		containerInstanceArn, err := client.registerContainerInstance(clusterRef, containerInstanceArn, attributes)
		if err == nil {
			return containerInstanceArn, nil
		}
		// If trying to register fails, try to create the cluster before calling
		// register again
		clusterRef, err = client.CreateCluster(clusterRef)
		if err != nil {
			return "", err
		}
	}
	return client.registerContainerInstance(clusterRef, containerInstanceArn, attributes)
}

func (client *APIECSClient) registerContainerInstance(clusterRef string, containerInstanceArn string, attributes []string) (string, error) {
	registerRequest := ecs.RegisterContainerInstanceInput{Cluster: &clusterRef}
	if containerInstanceArn != "" {
		registerRequest.ContainerInstanceArn = &containerInstanceArn
	}

	for _, attribute := range attributes {
		registerRequest.Attributes = append(registerRequest.Attributes, &ecs.Attribute{
			Name: aws.String(attribute),
		})
	}

	additionalAttributes := getAdditionalAttributes()
	for _, attribute := range additionalAttributes {
		registerRequest.Attributes = append(registerRequest.Attributes, attribute)
	}

	instanceIdentityDoc, err := client.ec2metadata.ReadResource(ec2.INSTANCE_IDENTITY_DOCUMENT_RESOURCE)
	iidRetrieved := true
	if err != nil {
		log.Error("Unable to get instance identity document", "err", err)
		iidRetrieved = false
		instanceIdentityDoc = []byte{}
	}
	strIid := string(instanceIdentityDoc)
	registerRequest.InstanceIdentityDocument = &strIid

	instanceIdentitySignature := []byte{}
	if iidRetrieved {
		instanceIdentitySignature, err = client.ec2metadata.ReadResource(ec2.INSTANCE_IDENTITY_DOCUMENT_SIGNATURE_RESOURCE)
		if err != nil {
			log.Error("Unable to get instance identity signature", "err", err)
		}
	}

	strIidSig := string(instanceIdentitySignature)
	registerRequest.InstanceIdentityDocumentSignature = &strIidSig

	// Micro-optimization, the pointer to this is used multiple times below
	integerStr := "INTEGER"

	cpu, mem := getCpuAndMemory()
	mem = mem - int64(client.config.ReservedMemory)

	cpuResource := ecs.Resource{
		Name:         utils.Strptr("CPU"),
		Type:         &integerStr,
		IntegerValue: &cpu,
	}
	memResource := ecs.Resource{
		Name:         utils.Strptr("MEMORY"),
		Type:         &integerStr,
		IntegerValue: &mem,
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

	resources := []*ecs.Resource{&cpuResource, &memResource, &portResource, &udpPortResource}
	registerRequest.TotalResources = resources

	resp, err := client.standardClient.RegisterContainerInstance(&registerRequest)
	if err != nil {
		log.Error("Could not register", "err", err)
		return "", err
	}
	log.Info("Registered!")
	return *resp.ContainerInstance.ContainerInstanceArn, nil
}

func getCpuAndMemory() (int64, int64) {
	memInfo, err := system.ReadMemInfo()
	mem := int64(0)
	if err == nil {
		mem = memInfo.MemTotal / 1024 / 1024 // MiB
	} else {
		log.Error("Unable to get memory info", "err", err)
	}

	cpu := runtime.NumCPU() * 1024

	return int64(cpu), mem
}

func getAdditionalAttributes() []*ecs.Attribute {
	return []*ecs.Attribute{&ecs.Attribute{
		Name:  aws.String("ecs.os-type"),
		Value: aws.String(api.OSType),
	}}
}

func (client *APIECSClient) SubmitTaskStateChange(change api.TaskStateChange) error {
	if change.Status == api.TaskStatusNone {
		log.Warn("SubmitTaskStateChange called with an invalid change", "change", change)
		return errors.New("SubmitTaskStateChange called with an invalid change")
	}

	if change.Status != api.TaskRunning && change.Status != api.TaskStopped {
		log.Debug("Not submitting unsupported upstream task state", "state", change.Status.String())
		// Not really an error
		return nil
	}

	status := change.Status.String()
	_, err := client.submitStateChangeClient.SubmitTaskStateChange(&ecs.SubmitTaskStateChangeInput{
		Cluster: &client.config.Cluster,
		Task:    &change.TaskArn,
		Status:  &status,
		Reason:  &change.Reason,
	})
	if err != nil {
		log.Warn("Could not submit a task state change", "err", err)
		return err
	}
	return nil
}

func (client *APIECSClient) SubmitContainerStateChange(change api.ContainerStateChange) error {
	req := ecs.SubmitContainerStateChangeInput{
		Cluster:       &client.config.Cluster,
		Task:          &change.TaskArn,
		ContainerName: &change.ContainerName,
	}
	if change.Reason != "" {
		if len(change.Reason) > ecsMaxReasonLength {
			trimmed := change.Reason[0:ecsMaxReasonLength]
			req.Reason = &trimmed
		} else {
			req.Reason = &change.Reason
		}
	}
	stat := change.Status.String()
	if stat == "DEAD" {
		stat = "STOPPED"
	}
	if stat != "STOPPED" && stat != "RUNNING" {
		log.Info("Not submitting not supported upstream container state", "state", stat)
		return nil
	}
	req.Status = &stat
	if change.ExitCode != nil {
		exitCode := int64(*change.ExitCode)
		req.ExitCode = &exitCode
	}
	networkBindings := make([]*ecs.NetworkBinding, len(change.PortBindings))
	for i, binding := range change.PortBindings {
		hostPort := int64(binding.HostPort)
		containerPort := int64(binding.ContainerPort)
		bindIP := binding.BindIp
		protocol := binding.Protocol.String()
		networkBindings[i] = &ecs.NetworkBinding{
			BindIP:        &bindIP,
			ContainerPort: &containerPort,
			HostPort:      &hostPort,
			Protocol:      &protocol,
		}
	}
	req.NetworkBindings = networkBindings

	_, err := client.submitStateChangeClient.SubmitContainerStateChange(&req)
	if err != nil {
		log.Warn("Could not submit a container state change", "change", change, "err", err)
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

func (client *APIECSClient) discoverPollEndpoint(containerInstanceArn string) (*ecs.DiscoverPollEndpointOutput, error) {
	// Try getting an entry from the cache
	cachedEndpoint, found := client.pollEndpoinCache.Get(containerInstanceArn)
	if found {
		// Cache hit. Return the output.
		if output, ok := cachedEndpoint.(*ecs.DiscoverPollEndpointOutput); ok {
			return output, nil
		}
	}

	// Cache miss, invoke the ECS DiscoverPollEndpoint API.
	seelog.Debugf("Invoking DiscoverPollEndpoint for '%s'", containerInstanceArn)
	output, err := client.standardClient.DiscoverPollEndpoint(&ecs.DiscoverPollEndpointInput{
		ContainerInstance: &containerInstanceArn,
		Cluster:           &client.config.Cluster,
	})
	if err != nil {
		return nil, err
	}

	// Cache the response from ECS.
	client.pollEndpoinCache.Set(containerInstanceArn, output)
	return output, nil
}
