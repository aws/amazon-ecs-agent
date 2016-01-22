// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package api

import (
	"errors"
	"net/http"
	"runtime"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/docker/docker/pkg/system"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

var log = logger.ForModule("api client")

// ECSClient is an interface over the ECSSDK interface which abstracts away some
// details around constructing the request and reading the response down to the
// parts the agent cares about.
// For example, the ever-present 'Cluster' member is abstracted out so that it
// may be configured once and used throughout transparently.
type ECSClient interface {
	// RegisterContainerInstance calculates the appropriate resources, creates
	// the default cluster if necessary, and returns the registered
	// ContainerInstanceARN if successful. Supplying a non-empty container
	// instance ARN allows a container instance to update its registered
	// resources.
	RegisterContainerInstance(existingContainerInstanceArn string, attributes []string) (string, error)
	// SubmitTaskStateChange sends a state change and returns an error
	// indicating if it was submitted
	SubmitTaskStateChange(change TaskStateChange) error
	// SubmitContainerStateChange sends a state change and returns an error
	// indicating if it was submitted
	SubmitContainerStateChange(change ContainerStateChange) error
	// DiscoverPollEndpoint takes a ContainerInstanceARN and returns the
	// endpoint at which this Agent should contact ACS
	DiscoverPollEndpoint(containerInstanceArn string) (string, error)
	// DiscoverTelemetryEndpoint takes a ContainerInstanceARN and returns the
	// endpoint at which this Agent should contact Telemetry Service
	DiscoverTelemetryEndpoint(containerInstanceArn string) (string, error)
}

// ECSSDK is an interface that specifies the subset of the AWS Go SDK's ECS
// client that the Agent uses.  This interface is meant to allow injecting a
// mock for testing.
type ECSSDK interface {
	CreateCluster(*ecs.CreateClusterInput) (*ecs.CreateClusterOutput, error)
	RegisterContainerInstance(*ecs.RegisterContainerInstanceInput) (*ecs.RegisterContainerInstanceOutput, error)
	DiscoverPollEndpoint(*ecs.DiscoverPollEndpointInput) (*ecs.DiscoverPollEndpointOutput, error)
}

type ECSSubmitStateSDK interface {
	SubmitContainerStateChange(*ecs.SubmitContainerStateChangeInput) (*ecs.SubmitContainerStateChangeOutput, error)
	SubmitTaskStateChange(*ecs.SubmitTaskStateChangeInput) (*ecs.SubmitTaskStateChangeOutput, error)
}

// ApiECSClient implements ECSClient
type ApiECSClient struct {
	credentialProvider      *credentials.Credentials
	config                  *config.Config
	standardClient          ECSSDK
	submitStateChangeClient ECSSubmitStateSDK
	ec2metadata             ec2.EC2MetadataClient
}

// SetSDK overrides the SDK to the given one. This is useful for injecting a
// test implementation
func (client *ApiECSClient) SetSDK(sdk ECSSDK) {
	client.standardClient = sdk
}

// SetSubmitStateChangeSDK overrides the SDK to the given one. This is useful
// for injecting a test implementation
func (client *ApiECSClient) SetSubmitStateChangeSDK(sdk ECSSubmitStateSDK) {
	client.submitStateChangeClient = sdk
}

const (
	ECS_SERVICE = "ecs"

	EcsMaxReasonLength = 255

	RoundtripTimeout = 5 * time.Second
)

func NewECSClient(credentialProvider *credentials.Credentials, config *config.Config, httpClient *http.Client, ec2MetadataClient ec2.EC2MetadataClient) ECSClient {
	var ecsConfig aws.Config
	ecsConfig.Credentials = credentialProvider
	ecsConfig.Region = &config.AWSRegion
	ecsConfig.HTTPClient = httpClient
	if config.APIEndpoint != "" {
		ecsConfig.Endpoint = &config.APIEndpoint
	}
	standardClient := ecs.New(session.New(&ecsConfig))
	submitStateChangeClient := newSubmitStateChangeClient(&ecsConfig)
	return &ApiECSClient{
		credentialProvider:      credentialProvider,
		config:                  config,
		standardClient:          standardClient,
		submitStateChangeClient: submitStateChangeClient,
		ec2metadata:             ec2MetadataClient,
	}
}

func getCpuAndMemory() (int64, int64) {
	memInfo, err := system.ReadMemInfo()
	mem := memInfo.MemTotal / 1024 / 1024 // MiB
	if err != nil {
		log.Error("Unable to get memory info", "err", err)
		mem = 0
	}
	cpu := runtime.NumCPU() * 1024

	return int64(cpu), mem
}

// CreateCluster creates a cluster from a given name and returns its arn
func (client *ApiECSClient) CreateCluster(clusterName string) (string, error) {
	resp, err := client.standardClient.CreateCluster(&ecs.CreateClusterInput{ClusterName: &clusterName})
	if err != nil {
		log.Crit("Could not register", "err", err)
		return "", err
	}
	log.Info("Created a cluster!", "clusterName", clusterName)
	return *resp.Cluster.ClusterName, nil
}

func (client *ApiECSClient) RegisterContainerInstance(containerInstanceArn string, attributes []string) (string, error) {
	clusterRef := client.config.Cluster
	// If our clusterRef is empty, we should try to create the default
	if clusterRef == "" {
		clusterRef = config.DEFAULT_CLUSTER_NAME
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

func (client *ApiECSClient) registerContainerInstance(clusterRef string, containerInstanceArn string, attributes []string) (string, error) {
	registerRequest := ecs.RegisterContainerInstanceInput{Cluster: &clusterRef}
	if containerInstanceArn != "" {
		registerRequest.ContainerInstanceArn = &containerInstanceArn
	}

	for _, attribute := range attributes {
		registerRequest.Attributes = append(registerRequest.Attributes, &ecs.Attribute{
			Name: aws.String(attribute),
		})
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

func (client *ApiECSClient) SubmitTaskStateChange(change TaskStateChange) error {
	if change.Status == TaskStatusNone {
		log.Warn("SubmitTaskStateChange called with an invalid change", "change", change)
		return errors.New("SubmitTaskStateChange called with an invalid change")
	}

	if change.Status != TaskRunning && change.Status != TaskStopped {
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

func (client *ApiECSClient) SubmitContainerStateChange(change ContainerStateChange) error {
	req := ecs.SubmitContainerStateChangeInput{
		Cluster:       &client.config.Cluster,
		Task:          &change.TaskArn,
		ContainerName: &change.ContainerName,
	}
	if change.Reason != "" {
		if len(change.Reason) > EcsMaxReasonLength {
			trimmed := change.Reason[0:EcsMaxReasonLength]
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

func (client *ApiECSClient) DiscoverPollEndpoint(containerInstanceArn string) (string, error) {
	resp, err := client.standardClient.DiscoverPollEndpoint(&ecs.DiscoverPollEndpointInput{
		ContainerInstance: &containerInstanceArn,
		Cluster:           &client.config.Cluster,
	})
	if err != nil {
		return "", err
	}

	return *resp.Endpoint, nil
}

func (client *ApiECSClient) DiscoverTelemetryEndpoint(containerInstanceArn string) (string, error) {
	resp, err := client.standardClient.DiscoverPollEndpoint(&ecs.DiscoverPollEndpointInput{
		ContainerInstance: &containerInstanceArn,
		Cluster:           &client.config.Cluster,
	})
	if err != nil {
		return "", err
	}
	if resp.TelemetryEndpoint == nil {
		return "", errors.New("No telemetry endpoint returned; nil")
	}

	return *resp.TelemetryEndpoint, nil
}
