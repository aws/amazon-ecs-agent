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

package frontend

import (
	__model__ "github.com/aws/amazon-ecs-agent/agent/ecs_client/model/model"
	__reflect__ "reflect"
)

type AmazonEC2ContainerServiceV20141113 interface {
	DescribeClusters(DescribeClustersRequest) (DescribeClustersResponse, error)
	DeregisterContainerInstance(DeregisterContainerInstanceRequest) (DeregisterContainerInstanceResponse, error)
	DeregisterTaskDefinition(DeregisterTaskDefinitionRequest) (DeregisterTaskDefinitionResponse, error)
	ListTasks(ListTasksRequest) (ListTasksResponse, error)
	SubmitTaskStateChange(SubmitTaskStateChangeRequest) (SubmitTaskStateChangeResponse, error)
	CreateCluster(CreateClusterRequest) (CreateClusterResponse, error)
	DeleteCluster(DeleteClusterRequest) (DeleteClusterResponse, error)
	RegisterContainerInstance(RegisterContainerInstanceRequest) (RegisterContainerInstanceResponse, error)
	DiscoverPollEndpoint(DiscoverPollEndpointRequest) (DiscoverPollEndpointResponse, error)
	ListClusters(ListClustersRequest) (ListClustersResponse, error)
	RunTask(RunTaskRequest) (RunTaskResponse, error)
	SubmitContainerStateChange(SubmitContainerStateChangeRequest) (SubmitContainerStateChangeResponse, error)
	DescribeContainerInstances(DescribeContainerInstancesRequest) (DescribeContainerInstancesResponse, error)
	RegisterTaskDefinition(RegisterTaskDefinitionRequest) (RegisterTaskDefinitionResponse, error)
	DescribeTaskDefinition(DescribeTaskDefinitionRequest) (DescribeTaskDefinitionResponse, error)
	ListTaskDefinitions(ListTaskDefinitionsRequest) (ListTaskDefinitionsResponse, error)
	StartTask(StartTaskRequest) (StartTaskResponse, error)
	DescribeTasks(DescribeTasksRequest) (DescribeTasksResponse, error)
	StopTask(StopTaskRequest) (StopTaskResponse, error)
	ListContainerInstances(ListContainerInstancesRequest) (ListContainerInstancesResponse, error)
}
type ClientException interface {
	error
	SetMessage(s *string)
	Message() *string
}
type _ClientException struct {
	Message_ *string `awsjson:"message"`
}

func (this *_ClientException) Error() string {
	return __model__.ErrorMessage(this)
}
func (this *_ClientException) Message() *string {
	return this.Message_
}
func (this *_ClientException) SetMessage(s *string) {
	this.Message_ = s
}
func NewClientException() ClientException {
	return &_ClientException{}
}
func init() {
	var val ClientException
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("ClientException", t, func() interface{} {
		return NewClientException()
	})
}

type Cluster interface {
	SetClusterArn(s *string)
	ClusterArn() *string
	SetClusterName(s *string)
	ClusterName() *string
	SetStatus(s *string)
	Status() *string
}
type _Cluster struct {
	Status_      *string `awsjson:"status"`
	ClusterArn_  *string `awsjson:"clusterArn"`
	ClusterName_ *string `awsjson:"clusterName"`
}

func (this *_Cluster) ClusterArn() *string {
	return this.ClusterArn_
}
func (this *_Cluster) SetClusterArn(s *string) {
	this.ClusterArn_ = s
}
func (this *_Cluster) ClusterName() *string {
	return this.ClusterName_
}
func (this *_Cluster) SetClusterName(s *string) {
	this.ClusterName_ = s
}
func (this *_Cluster) Status() *string {
	return this.Status_
}
func (this *_Cluster) SetStatus(s *string) {
	this.Status_ = s
}
func NewCluster() Cluster {
	return &_Cluster{}
}
func init() {
	var val Cluster
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("Cluster", t, func() interface{} {
		return NewCluster()
	})
}

type Container interface {
	SetExitCode(b *int32)
	ExitCode() *int32
	SetReason(s *string)
	Reason() *string
	SetNetworkBindings(n []NetworkBinding)
	NetworkBindings() []NetworkBinding
	SetContainerArn(s *string)
	ContainerArn() *string
	SetTaskArn(s *string)
	TaskArn() *string
	SetName(s *string)
	Name() *string
	SetLastStatus(s *string)
	LastStatus() *string
}
type _Container struct {
	ContainerArn_    *string          `awsjson:"containerArn"`
	TaskArn_         *string          `awsjson:"taskArn"`
	Name_            *string          `awsjson:"name"`
	LastStatus_      *string          `awsjson:"lastStatus"`
	ExitCode_        *int32           `awsjson:"exitCode"`
	Reason_          *string          `awsjson:"reason"`
	NetworkBindings_ []NetworkBinding `awsjson:"networkBindings"`
}

func (this *_Container) ContainerArn() *string {
	return this.ContainerArn_
}
func (this *_Container) SetContainerArn(s *string) {
	this.ContainerArn_ = s
}
func (this *_Container) ExitCode() *int32 {
	return this.ExitCode_
}
func (this *_Container) SetExitCode(b *int32) {
	this.ExitCode_ = b
}
func (this *_Container) LastStatus() *string {
	return this.LastStatus_
}
func (this *_Container) SetLastStatus(s *string) {
	this.LastStatus_ = s
}
func (this *_Container) Name() *string {
	return this.Name_
}
func (this *_Container) SetName(s *string) {
	this.Name_ = s
}
func (this *_Container) NetworkBindings() []NetworkBinding {
	return this.NetworkBindings_
}
func (this *_Container) SetNetworkBindings(n []NetworkBinding) {
	this.NetworkBindings_ = n
}
func (this *_Container) Reason() *string {
	return this.Reason_
}
func (this *_Container) SetReason(s *string) {
	this.Reason_ = s
}
func (this *_Container) TaskArn() *string {
	return this.TaskArn_
}
func (this *_Container) SetTaskArn(s *string) {
	this.TaskArn_ = s
}
func NewContainer() Container {
	return &_Container{}
}
func init() {
	var val Container
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("Container", t, func() interface{} {
		return NewContainer()
	})
}

type ContainerDefinition interface {
	SetEssential(b *bool)
	Essential() *bool
	SetEntryPoint(s []*string)
	EntryPoint() []*string
	SetCpu(i *int32)
	Cpu() *int32
	SetPortMappings(p []PortMapping)
	PortMappings() []PortMapping
	SetMemory(i *int32)
	Memory() *int32
	SetLinks(s []*string)
	Links() []*string
	SetCommand(s []*string)
	Command() []*string
	SetEnvironment(e []KeyValuePair)
	Environment() []KeyValuePair
	SetName(s *string)
	Name() *string
	SetImage(s *string)
	Image() *string
}
type _ContainerDefinition struct {
	Cpu_          *int32         `awsjson:"cpu"`
	PortMappings_ []PortMapping  `awsjson:"portMappings"`
	Essential_    *bool          `awsjson:"essential"`
	EntryPoint_   []*string      `awsjson:"entryPoint"`
	Command_      []*string      `awsjson:"command"`
	Environment_  []KeyValuePair `awsjson:"environment"`
	Name_         *string        `awsjson:"name"`
	Image_        *string        `awsjson:"image"`
	Memory_       *int32         `awsjson:"memory"`
	Links_        []*string      `awsjson:"links"`
}

func (this *_ContainerDefinition) Command() []*string {
	return this.Command_
}
func (this *_ContainerDefinition) SetCommand(s []*string) {
	this.Command_ = s
}
func (this *_ContainerDefinition) Cpu() *int32 {
	return this.Cpu_
}
func (this *_ContainerDefinition) SetCpu(i *int32) {
	this.Cpu_ = i
}
func (this *_ContainerDefinition) EntryPoint() []*string {
	return this.EntryPoint_
}
func (this *_ContainerDefinition) SetEntryPoint(s []*string) {
	this.EntryPoint_ = s
}
func (this *_ContainerDefinition) Environment() []KeyValuePair {
	return this.Environment_
}
func (this *_ContainerDefinition) SetEnvironment(e []KeyValuePair) {
	this.Environment_ = e
}
func (this *_ContainerDefinition) Essential() *bool {
	return this.Essential_
}
func (this *_ContainerDefinition) SetEssential(b *bool) {
	this.Essential_ = b
}
func (this *_ContainerDefinition) Image() *string {
	return this.Image_
}
func (this *_ContainerDefinition) SetImage(s *string) {
	this.Image_ = s
}
func (this *_ContainerDefinition) Links() []*string {
	return this.Links_
}
func (this *_ContainerDefinition) SetLinks(s []*string) {
	this.Links_ = s
}
func (this *_ContainerDefinition) Memory() *int32 {
	return this.Memory_
}
func (this *_ContainerDefinition) SetMemory(i *int32) {
	this.Memory_ = i
}
func (this *_ContainerDefinition) Name() *string {
	return this.Name_
}
func (this *_ContainerDefinition) SetName(s *string) {
	this.Name_ = s
}
func (this *_ContainerDefinition) PortMappings() []PortMapping {
	return this.PortMappings_
}
func (this *_ContainerDefinition) SetPortMappings(p []PortMapping) {
	this.PortMappings_ = p
}
func NewContainerDefinition() ContainerDefinition {
	return &_ContainerDefinition{}
}
func init() {
	var val ContainerDefinition
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("ContainerDefinition", t, func() interface{} {
		return NewContainerDefinition()
	})
}

type ContainerInstance interface {
	SetRemainingResources(r []Resource)
	RemainingResources() []Resource
	SetRegisteredResources(r []Resource)
	RegisteredResources() []Resource
	SetStatus(s *string)
	Status() *string
	SetAgentConnected(b *bool)
	AgentConnected() *bool
	SetContainerInstanceArn(s *string)
	ContainerInstanceArn() *string
	SetEc2InstanceId(s *string)
	Ec2InstanceId() *string
}
type _ContainerInstance struct {
	RegisteredResources_  []Resource `awsjson:"registeredResources"`
	Status_               *string    `awsjson:"status"`
	AgentConnected_       *bool      `awsjson:"agentConnected"`
	ContainerInstanceArn_ *string    `awsjson:"containerInstanceArn"`
	Ec2InstanceId_        *string    `awsjson:"ec2InstanceId"`
	RemainingResources_   []Resource `awsjson:"remainingResources"`
}

func (this *_ContainerInstance) AgentConnected() *bool {
	return this.AgentConnected_
}
func (this *_ContainerInstance) SetAgentConnected(b *bool) {
	this.AgentConnected_ = b
}
func (this *_ContainerInstance) ContainerInstanceArn() *string {
	return this.ContainerInstanceArn_
}
func (this *_ContainerInstance) SetContainerInstanceArn(s *string) {
	this.ContainerInstanceArn_ = s
}
func (this *_ContainerInstance) Ec2InstanceId() *string {
	return this.Ec2InstanceId_
}
func (this *_ContainerInstance) SetEc2InstanceId(s *string) {
	this.Ec2InstanceId_ = s
}
func (this *_ContainerInstance) RegisteredResources() []Resource {
	return this.RegisteredResources_
}
func (this *_ContainerInstance) SetRegisteredResources(r []Resource) {
	this.RegisteredResources_ = r
}
func (this *_ContainerInstance) RemainingResources() []Resource {
	return this.RemainingResources_
}
func (this *_ContainerInstance) SetRemainingResources(r []Resource) {
	this.RemainingResources_ = r
}
func (this *_ContainerInstance) Status() *string {
	return this.Status_
}
func (this *_ContainerInstance) SetStatus(s *string) {
	this.Status_ = s
}
func NewContainerInstance() ContainerInstance {
	return &_ContainerInstance{}
}
func init() {
	var val ContainerInstance
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("ContainerInstance", t, func() interface{} {
		return NewContainerInstance()
	})
}

type ContainerOverride interface {
	SetName(s *string)
	Name() *string
	SetCommand(s []*string)
	Command() []*string
}
type _ContainerOverride struct {
	Command_ []*string `awsjson:"command"`
	Name_    *string   `awsjson:"name"`
}

func (this *_ContainerOverride) Command() []*string {
	return this.Command_
}
func (this *_ContainerOverride) SetCommand(s []*string) {
	this.Command_ = s
}
func (this *_ContainerOverride) Name() *string {
	return this.Name_
}
func (this *_ContainerOverride) SetName(s *string) {
	this.Name_ = s
}
func NewContainerOverride() ContainerOverride {
	return &_ContainerOverride{}
}
func init() {
	var val ContainerOverride
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("ContainerOverride", t, func() interface{} {
		return NewContainerOverride()
	})
}

type CreateClusterRequest interface {
	SetClusterName(s *string)
	ClusterName() *string
}
type _CreateClusterRequest struct {
	ClusterName_ *string `awsjson:"clusterName"`
}

func (this *_CreateClusterRequest) ClusterName() *string {
	return this.ClusterName_
}
func (this *_CreateClusterRequest) SetClusterName(s *string) {
	this.ClusterName_ = s
}
func NewCreateClusterRequest() CreateClusterRequest {
	return &_CreateClusterRequest{}
}
func init() {
	var val CreateClusterRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("CreateClusterRequest", t, func() interface{} {
		return NewCreateClusterRequest()
	})
}

type CreateClusterResponse interface {
	SetCluster(c Cluster)
	Cluster() Cluster
}
type _CreateClusterResponse struct {
	Cluster_ Cluster `awsjson:"cluster"`
}

func (this *_CreateClusterResponse) Cluster() Cluster {
	return this.Cluster_
}
func (this *_CreateClusterResponse) SetCluster(c Cluster) {
	this.Cluster_ = c
}
func NewCreateClusterResponse() CreateClusterResponse {
	return &_CreateClusterResponse{}
}
func init() {
	var val CreateClusterResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("CreateClusterResponse", t, func() interface{} {
		return NewCreateClusterResponse()
	})
}

type DeleteClusterRequest interface {
	SetCluster(s *string)
	Cluster() *string
}
type _DeleteClusterRequest struct {
	Cluster_ *string `awsjson:"cluster"`
}

func (this *_DeleteClusterRequest) Cluster() *string {
	return this.Cluster_
}
func (this *_DeleteClusterRequest) SetCluster(s *string) {
	this.Cluster_ = s
}
func NewDeleteClusterRequest() DeleteClusterRequest {
	return &_DeleteClusterRequest{}
}
func init() {
	var val DeleteClusterRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("DeleteClusterRequest", t, func() interface{} {
		return NewDeleteClusterRequest()
	})
}

type DeleteClusterResponse interface {
	SetCluster(c Cluster)
	Cluster() Cluster
}
type _DeleteClusterResponse struct {
	Cluster_ Cluster `awsjson:"cluster"`
}

func (this *_DeleteClusterResponse) Cluster() Cluster {
	return this.Cluster_
}
func (this *_DeleteClusterResponse) SetCluster(c Cluster) {
	this.Cluster_ = c
}
func NewDeleteClusterResponse() DeleteClusterResponse {
	return &_DeleteClusterResponse{}
}
func init() {
	var val DeleteClusterResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("DeleteClusterResponse", t, func() interface{} {
		return NewDeleteClusterResponse()
	})
}

type DeregisterContainerInstanceRequest interface {
	SetCluster(s *string)
	Cluster() *string
	SetContainerInstance(s *string)
	ContainerInstance() *string
	SetForce(b *bool)
	Force() *bool
}
type _DeregisterContainerInstanceRequest struct {
	Force_             *bool   `awsjson:"force"`
	Cluster_           *string `awsjson:"cluster"`
	ContainerInstance_ *string `awsjson:"containerInstance"`
}

func (this *_DeregisterContainerInstanceRequest) Cluster() *string {
	return this.Cluster_
}
func (this *_DeregisterContainerInstanceRequest) SetCluster(s *string) {
	this.Cluster_ = s
}
func (this *_DeregisterContainerInstanceRequest) ContainerInstance() *string {
	return this.ContainerInstance_
}
func (this *_DeregisterContainerInstanceRequest) SetContainerInstance(s *string) {
	this.ContainerInstance_ = s
}
func (this *_DeregisterContainerInstanceRequest) Force() *bool {
	return this.Force_
}
func (this *_DeregisterContainerInstanceRequest) SetForce(b *bool) {
	this.Force_ = b
}
func NewDeregisterContainerInstanceRequest() DeregisterContainerInstanceRequest {
	return &_DeregisterContainerInstanceRequest{}
}
func init() {
	var val DeregisterContainerInstanceRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("DeregisterContainerInstanceRequest", t, func() interface{} {
		return NewDeregisterContainerInstanceRequest()
	})
}

type DeregisterContainerInstanceResponse interface {
	SetContainerInstance(c ContainerInstance)
	ContainerInstance() ContainerInstance
}
type _DeregisterContainerInstanceResponse struct {
	ContainerInstance_ ContainerInstance `awsjson:"containerInstance"`
}

func (this *_DeregisterContainerInstanceResponse) ContainerInstance() ContainerInstance {
	return this.ContainerInstance_
}
func (this *_DeregisterContainerInstanceResponse) SetContainerInstance(c ContainerInstance) {
	this.ContainerInstance_ = c
}
func NewDeregisterContainerInstanceResponse() DeregisterContainerInstanceResponse {
	return &_DeregisterContainerInstanceResponse{}
}
func init() {
	var val DeregisterContainerInstanceResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("DeregisterContainerInstanceResponse", t, func() interface{} {
		return NewDeregisterContainerInstanceResponse()
	})
}

type DeregisterTaskDefinitionRequest interface {
	SetTaskDefinition(s *string)
	TaskDefinition() *string
}
type _DeregisterTaskDefinitionRequest struct {
	TaskDefinition_ *string `awsjson:"taskDefinition"`
}

func (this *_DeregisterTaskDefinitionRequest) TaskDefinition() *string {
	return this.TaskDefinition_
}
func (this *_DeregisterTaskDefinitionRequest) SetTaskDefinition(s *string) {
	this.TaskDefinition_ = s
}
func NewDeregisterTaskDefinitionRequest() DeregisterTaskDefinitionRequest {
	return &_DeregisterTaskDefinitionRequest{}
}
func init() {
	var val DeregisterTaskDefinitionRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("DeregisterTaskDefinitionRequest", t, func() interface{} {
		return NewDeregisterTaskDefinitionRequest()
	})
}

type DeregisterTaskDefinitionResponse interface {
	SetTaskDefinition(t TaskDefinition)
	TaskDefinition() TaskDefinition
}
type _DeregisterTaskDefinitionResponse struct {
	TaskDefinition_ TaskDefinition `awsjson:"taskDefinition"`
}

func (this *_DeregisterTaskDefinitionResponse) TaskDefinition() TaskDefinition {
	return this.TaskDefinition_
}
func (this *_DeregisterTaskDefinitionResponse) SetTaskDefinition(t TaskDefinition) {
	this.TaskDefinition_ = t
}
func NewDeregisterTaskDefinitionResponse() DeregisterTaskDefinitionResponse {
	return &_DeregisterTaskDefinitionResponse{}
}
func init() {
	var val DeregisterTaskDefinitionResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("DeregisterTaskDefinitionResponse", t, func() interface{} {
		return NewDeregisterTaskDefinitionResponse()
	})
}

type DescribeClustersRequest interface {
	SetClusters(s []*string)
	Clusters() []*string
}
type _DescribeClustersRequest struct {
	Clusters_ []*string `awsjson:"clusters"`
}

func (this *_DescribeClustersRequest) Clusters() []*string {
	return this.Clusters_
}
func (this *_DescribeClustersRequest) SetClusters(s []*string) {
	this.Clusters_ = s
}
func NewDescribeClustersRequest() DescribeClustersRequest {
	return &_DescribeClustersRequest{}
}
func init() {
	var val DescribeClustersRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("DescribeClustersRequest", t, func() interface{} {
		return NewDescribeClustersRequest()
	})
}

type DescribeClustersResponse interface {
	SetClusters(c []Cluster)
	Clusters() []Cluster
	SetFailures(f []Failure)
	Failures() []Failure
}
type _DescribeClustersResponse struct {
	Clusters_ []Cluster `awsjson:"clusters"`
	Failures_ []Failure `awsjson:"failures"`
}

func (this *_DescribeClustersResponse) Clusters() []Cluster {
	return this.Clusters_
}
func (this *_DescribeClustersResponse) SetClusters(c []Cluster) {
	this.Clusters_ = c
}
func (this *_DescribeClustersResponse) Failures() []Failure {
	return this.Failures_
}
func (this *_DescribeClustersResponse) SetFailures(f []Failure) {
	this.Failures_ = f
}
func NewDescribeClustersResponse() DescribeClustersResponse {
	return &_DescribeClustersResponse{}
}
func init() {
	var val DescribeClustersResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("DescribeClustersResponse", t, func() interface{} {
		return NewDescribeClustersResponse()
	})
}

type DescribeContainerInstancesRequest interface {
	SetCluster(s *string)
	Cluster() *string
	SetContainerInstances(s []*string)
	ContainerInstances() []*string
}
type _DescribeContainerInstancesRequest struct {
	Cluster_            *string   `awsjson:"cluster"`
	ContainerInstances_ []*string `awsjson:"containerInstances"`
}

func (this *_DescribeContainerInstancesRequest) Cluster() *string {
	return this.Cluster_
}
func (this *_DescribeContainerInstancesRequest) SetCluster(s *string) {
	this.Cluster_ = s
}
func (this *_DescribeContainerInstancesRequest) ContainerInstances() []*string {
	return this.ContainerInstances_
}
func (this *_DescribeContainerInstancesRequest) SetContainerInstances(s []*string) {
	this.ContainerInstances_ = s
}
func NewDescribeContainerInstancesRequest() DescribeContainerInstancesRequest {
	return &_DescribeContainerInstancesRequest{}
}
func init() {
	var val DescribeContainerInstancesRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("DescribeContainerInstancesRequest", t, func() interface{} {
		return NewDescribeContainerInstancesRequest()
	})
}

type DescribeContainerInstancesResponse interface {
	SetContainerInstances(c []ContainerInstance)
	ContainerInstances() []ContainerInstance
	SetFailures(f []Failure)
	Failures() []Failure
}
type _DescribeContainerInstancesResponse struct {
	ContainerInstances_ []ContainerInstance `awsjson:"containerInstances"`
	Failures_           []Failure           `awsjson:"failures"`
}

func (this *_DescribeContainerInstancesResponse) ContainerInstances() []ContainerInstance {
	return this.ContainerInstances_
}
func (this *_DescribeContainerInstancesResponse) SetContainerInstances(c []ContainerInstance) {
	this.ContainerInstances_ = c
}
func (this *_DescribeContainerInstancesResponse) Failures() []Failure {
	return this.Failures_
}
func (this *_DescribeContainerInstancesResponse) SetFailures(f []Failure) {
	this.Failures_ = f
}
func NewDescribeContainerInstancesResponse() DescribeContainerInstancesResponse {
	return &_DescribeContainerInstancesResponse{}
}
func init() {
	var val DescribeContainerInstancesResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("DescribeContainerInstancesResponse", t, func() interface{} {
		return NewDescribeContainerInstancesResponse()
	})
}

type DescribeTaskDefinitionRequest interface {
	SetTaskDefinition(s *string)
	TaskDefinition() *string
}
type _DescribeTaskDefinitionRequest struct {
	TaskDefinition_ *string `awsjson:"taskDefinition"`
}

func (this *_DescribeTaskDefinitionRequest) TaskDefinition() *string {
	return this.TaskDefinition_
}
func (this *_DescribeTaskDefinitionRequest) SetTaskDefinition(s *string) {
	this.TaskDefinition_ = s
}
func NewDescribeTaskDefinitionRequest() DescribeTaskDefinitionRequest {
	return &_DescribeTaskDefinitionRequest{}
}
func init() {
	var val DescribeTaskDefinitionRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("DescribeTaskDefinitionRequest", t, func() interface{} {
		return NewDescribeTaskDefinitionRequest()
	})
}

type DescribeTaskDefinitionResponse interface {
	SetTaskDefinition(t TaskDefinition)
	TaskDefinition() TaskDefinition
}
type _DescribeTaskDefinitionResponse struct {
	TaskDefinition_ TaskDefinition `awsjson:"taskDefinition"`
}

func (this *_DescribeTaskDefinitionResponse) TaskDefinition() TaskDefinition {
	return this.TaskDefinition_
}
func (this *_DescribeTaskDefinitionResponse) SetTaskDefinition(t TaskDefinition) {
	this.TaskDefinition_ = t
}
func NewDescribeTaskDefinitionResponse() DescribeTaskDefinitionResponse {
	return &_DescribeTaskDefinitionResponse{}
}
func init() {
	var val DescribeTaskDefinitionResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("DescribeTaskDefinitionResponse", t, func() interface{} {
		return NewDescribeTaskDefinitionResponse()
	})
}

type DescribeTasksRequest interface {
	SetTasks(s []*string)
	Tasks() []*string
	SetCluster(s *string)
	Cluster() *string
}
type _DescribeTasksRequest struct {
	Cluster_ *string   `awsjson:"cluster"`
	Tasks_   []*string `awsjson:"tasks"`
}

func (this *_DescribeTasksRequest) Cluster() *string {
	return this.Cluster_
}
func (this *_DescribeTasksRequest) SetCluster(s *string) {
	this.Cluster_ = s
}
func (this *_DescribeTasksRequest) Tasks() []*string {
	return this.Tasks_
}
func (this *_DescribeTasksRequest) SetTasks(s []*string) {
	this.Tasks_ = s
}
func NewDescribeTasksRequest() DescribeTasksRequest {
	return &_DescribeTasksRequest{}
}
func init() {
	var val DescribeTasksRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("DescribeTasksRequest", t, func() interface{} {
		return NewDescribeTasksRequest()
	})
}

type DescribeTasksResponse interface {
	SetTasks(t []Task)
	Tasks() []Task
	SetFailures(f []Failure)
	Failures() []Failure
}
type _DescribeTasksResponse struct {
	Tasks_    []Task    `awsjson:"tasks"`
	Failures_ []Failure `awsjson:"failures"`
}

func (this *_DescribeTasksResponse) Failures() []Failure {
	return this.Failures_
}
func (this *_DescribeTasksResponse) SetFailures(f []Failure) {
	this.Failures_ = f
}
func (this *_DescribeTasksResponse) Tasks() []Task {
	return this.Tasks_
}
func (this *_DescribeTasksResponse) SetTasks(t []Task) {
	this.Tasks_ = t
}
func NewDescribeTasksResponse() DescribeTasksResponse {
	return &_DescribeTasksResponse{}
}
func init() {
	var val DescribeTasksResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("DescribeTasksResponse", t, func() interface{} {
		return NewDescribeTasksResponse()
	})
}

type DiscoverPollEndpointRequest interface {
	SetContainerInstance(s *string)
	ContainerInstance() *string
	SetCluster(s *string)
	Cluster() *string
}
type _DiscoverPollEndpointRequest struct {
	ContainerInstance_ *string `awsjson:"containerInstance"`
	Cluster_           *string `awsjson:"cluster"`
}

func (this *_DiscoverPollEndpointRequest) Cluster() *string {
	return this.Cluster_
}
func (this *_DiscoverPollEndpointRequest) SetCluster(s *string) {
	this.Cluster_ = s
}
func (this *_DiscoverPollEndpointRequest) ContainerInstance() *string {
	return this.ContainerInstance_
}
func (this *_DiscoverPollEndpointRequest) SetContainerInstance(s *string) {
	this.ContainerInstance_ = s
}
func NewDiscoverPollEndpointRequest() DiscoverPollEndpointRequest {
	return &_DiscoverPollEndpointRequest{}
}
func init() {
	var val DiscoverPollEndpointRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("DiscoverPollEndpointRequest", t, func() interface{} {
		return NewDiscoverPollEndpointRequest()
	})
}

type DiscoverPollEndpointResponse interface {
	SetEndpoint(s *string)
	Endpoint() *string
}
type _DiscoverPollEndpointResponse struct {
	Endpoint_ *string `awsjson:"endpoint"`
}

func (this *_DiscoverPollEndpointResponse) Endpoint() *string {
	return this.Endpoint_
}
func (this *_DiscoverPollEndpointResponse) SetEndpoint(s *string) {
	this.Endpoint_ = s
}
func NewDiscoverPollEndpointResponse() DiscoverPollEndpointResponse {
	return &_DiscoverPollEndpointResponse{}
}
func init() {
	var val DiscoverPollEndpointResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("DiscoverPollEndpointResponse", t, func() interface{} {
		return NewDiscoverPollEndpointResponse()
	})
}

type Failure interface {
	SetArn(s *string)
	Arn() *string
	SetReason(s *string)
	Reason() *string
}
type _Failure struct {
	Arn_    *string `awsjson:"arn"`
	Reason_ *string `awsjson:"reason"`
}

func (this *_Failure) Arn() *string {
	return this.Arn_
}
func (this *_Failure) SetArn(s *string) {
	this.Arn_ = s
}
func (this *_Failure) Reason() *string {
	return this.Reason_
}
func (this *_Failure) SetReason(s *string) {
	this.Reason_ = s
}
func NewFailure() Failure {
	return &_Failure{}
}
func init() {
	var val Failure
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("Failure", t, func() interface{} {
		return NewFailure()
	})
}

type KeyValuePair interface {
	SetName(s *string)
	Name() *string
	SetValue(s *string)
	Value() *string
}
type _KeyValuePair struct {
	Name_  *string `awsjson:"name"`
	Value_ *string `awsjson:"value"`
}

func (this *_KeyValuePair) Name() *string {
	return this.Name_
}
func (this *_KeyValuePair) SetName(s *string) {
	this.Name_ = s
}
func (this *_KeyValuePair) Value() *string {
	return this.Value_
}
func (this *_KeyValuePair) SetValue(s *string) {
	this.Value_ = s
}
func NewKeyValuePair() KeyValuePair {
	return &_KeyValuePair{}
}
func init() {
	var val KeyValuePair
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("KeyValuePair", t, func() interface{} {
		return NewKeyValuePair()
	})
}

type ListClustersRequest interface {
	SetNextToken(s *string)
	NextToken() *string
	SetMaxResults(b *int32)
	MaxResults() *int32
}
type _ListClustersRequest struct {
	NextToken_  *string `awsjson:"nextToken"`
	MaxResults_ *int32  `awsjson:"maxResults"`
}

func (this *_ListClustersRequest) MaxResults() *int32 {
	return this.MaxResults_
}
func (this *_ListClustersRequest) SetMaxResults(b *int32) {
	this.MaxResults_ = b
}
func (this *_ListClustersRequest) NextToken() *string {
	return this.NextToken_
}
func (this *_ListClustersRequest) SetNextToken(s *string) {
	this.NextToken_ = s
}
func NewListClustersRequest() ListClustersRequest {
	return &_ListClustersRequest{}
}
func init() {
	var val ListClustersRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("ListClustersRequest", t, func() interface{} {
		return NewListClustersRequest()
	})
}

type ListClustersResponse interface {
	SetNextToken(s *string)
	NextToken() *string
	SetClusterArns(s []*string)
	ClusterArns() []*string
}
type _ListClustersResponse struct {
	ClusterArns_ []*string `awsjson:"clusterArns"`
	NextToken_   *string   `awsjson:"nextToken"`
}

func (this *_ListClustersResponse) ClusterArns() []*string {
	return this.ClusterArns_
}
func (this *_ListClustersResponse) SetClusterArns(s []*string) {
	this.ClusterArns_ = s
}
func (this *_ListClustersResponse) NextToken() *string {
	return this.NextToken_
}
func (this *_ListClustersResponse) SetNextToken(s *string) {
	this.NextToken_ = s
}
func NewListClustersResponse() ListClustersResponse {
	return &_ListClustersResponse{}
}
func init() {
	var val ListClustersResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("ListClustersResponse", t, func() interface{} {
		return NewListClustersResponse()
	})
}

type ListContainerInstancesRequest interface {
	SetCluster(s *string)
	Cluster() *string
	SetNextToken(s *string)
	NextToken() *string
	SetMaxResults(b *int32)
	MaxResults() *int32
}
type _ListContainerInstancesRequest struct {
	NextToken_  *string `awsjson:"nextToken"`
	MaxResults_ *int32  `awsjson:"maxResults"`
	Cluster_    *string `awsjson:"cluster"`
}

func (this *_ListContainerInstancesRequest) Cluster() *string {
	return this.Cluster_
}
func (this *_ListContainerInstancesRequest) SetCluster(s *string) {
	this.Cluster_ = s
}
func (this *_ListContainerInstancesRequest) MaxResults() *int32 {
	return this.MaxResults_
}
func (this *_ListContainerInstancesRequest) SetMaxResults(b *int32) {
	this.MaxResults_ = b
}
func (this *_ListContainerInstancesRequest) NextToken() *string {
	return this.NextToken_
}
func (this *_ListContainerInstancesRequest) SetNextToken(s *string) {
	this.NextToken_ = s
}
func NewListContainerInstancesRequest() ListContainerInstancesRequest {
	return &_ListContainerInstancesRequest{}
}
func init() {
	var val ListContainerInstancesRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("ListContainerInstancesRequest", t, func() interface{} {
		return NewListContainerInstancesRequest()
	})
}

type ListContainerInstancesResponse interface {
	SetContainerInstanceArns(s []*string)
	ContainerInstanceArns() []*string
	SetNextToken(s *string)
	NextToken() *string
}
type _ListContainerInstancesResponse struct {
	ContainerInstanceArns_ []*string `awsjson:"containerInstanceArns"`
	NextToken_             *string   `awsjson:"nextToken"`
}

func (this *_ListContainerInstancesResponse) ContainerInstanceArns() []*string {
	return this.ContainerInstanceArns_
}
func (this *_ListContainerInstancesResponse) SetContainerInstanceArns(s []*string) {
	this.ContainerInstanceArns_ = s
}
func (this *_ListContainerInstancesResponse) NextToken() *string {
	return this.NextToken_
}
func (this *_ListContainerInstancesResponse) SetNextToken(s *string) {
	this.NextToken_ = s
}
func NewListContainerInstancesResponse() ListContainerInstancesResponse {
	return &_ListContainerInstancesResponse{}
}
func init() {
	var val ListContainerInstancesResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("ListContainerInstancesResponse", t, func() interface{} {
		return NewListContainerInstancesResponse()
	})
}

type ListTaskDefinitionsRequest interface {
	SetFamilyPrefix(s *string)
	FamilyPrefix() *string
	SetNextToken(s *string)
	NextToken() *string
	SetMaxResults(b *int32)
	MaxResults() *int32
}
type _ListTaskDefinitionsRequest struct {
	FamilyPrefix_ *string `awsjson:"familyPrefix"`
	NextToken_    *string `awsjson:"nextToken"`
	MaxResults_   *int32  `awsjson:"maxResults"`
}

func (this *_ListTaskDefinitionsRequest) FamilyPrefix() *string {
	return this.FamilyPrefix_
}
func (this *_ListTaskDefinitionsRequest) SetFamilyPrefix(s *string) {
	this.FamilyPrefix_ = s
}
func (this *_ListTaskDefinitionsRequest) MaxResults() *int32 {
	return this.MaxResults_
}
func (this *_ListTaskDefinitionsRequest) SetMaxResults(b *int32) {
	this.MaxResults_ = b
}
func (this *_ListTaskDefinitionsRequest) NextToken() *string {
	return this.NextToken_
}
func (this *_ListTaskDefinitionsRequest) SetNextToken(s *string) {
	this.NextToken_ = s
}
func NewListTaskDefinitionsRequest() ListTaskDefinitionsRequest {
	return &_ListTaskDefinitionsRequest{}
}
func init() {
	var val ListTaskDefinitionsRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("ListTaskDefinitionsRequest", t, func() interface{} {
		return NewListTaskDefinitionsRequest()
	})
}

type ListTaskDefinitionsResponse interface {
	SetTaskDefinitionArns(s []*string)
	TaskDefinitionArns() []*string
	SetNextToken(s *string)
	NextToken() *string
}
type _ListTaskDefinitionsResponse struct {
	TaskDefinitionArns_ []*string `awsjson:"taskDefinitionArns"`
	NextToken_          *string   `awsjson:"nextToken"`
}

func (this *_ListTaskDefinitionsResponse) NextToken() *string {
	return this.NextToken_
}
func (this *_ListTaskDefinitionsResponse) SetNextToken(s *string) {
	this.NextToken_ = s
}
func (this *_ListTaskDefinitionsResponse) TaskDefinitionArns() []*string {
	return this.TaskDefinitionArns_
}
func (this *_ListTaskDefinitionsResponse) SetTaskDefinitionArns(s []*string) {
	this.TaskDefinitionArns_ = s
}
func NewListTaskDefinitionsResponse() ListTaskDefinitionsResponse {
	return &_ListTaskDefinitionsResponse{}
}
func init() {
	var val ListTaskDefinitionsResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("ListTaskDefinitionsResponse", t, func() interface{} {
		return NewListTaskDefinitionsResponse()
	})
}

type ListTasksRequest interface {
	SetNextToken(s *string)
	NextToken() *string
	SetMaxResults(b *int32)
	MaxResults() *int32
	SetCluster(s *string)
	Cluster() *string
	SetContainerInstance(s *string)
	ContainerInstance() *string
	SetFamily(s *string)
	Family() *string
}
type _ListTasksRequest struct {
	Cluster_           *string `awsjson:"cluster"`
	ContainerInstance_ *string `awsjson:"containerInstance"`
	Family_            *string `awsjson:"family"`
	NextToken_         *string `awsjson:"nextToken"`
	MaxResults_        *int32  `awsjson:"maxResults"`
}

func (this *_ListTasksRequest) Cluster() *string {
	return this.Cluster_
}
func (this *_ListTasksRequest) SetCluster(s *string) {
	this.Cluster_ = s
}
func (this *_ListTasksRequest) ContainerInstance() *string {
	return this.ContainerInstance_
}
func (this *_ListTasksRequest) SetContainerInstance(s *string) {
	this.ContainerInstance_ = s
}
func (this *_ListTasksRequest) Family() *string {
	return this.Family_
}
func (this *_ListTasksRequest) SetFamily(s *string) {
	this.Family_ = s
}
func (this *_ListTasksRequest) MaxResults() *int32 {
	return this.MaxResults_
}
func (this *_ListTasksRequest) SetMaxResults(b *int32) {
	this.MaxResults_ = b
}
func (this *_ListTasksRequest) NextToken() *string {
	return this.NextToken_
}
func (this *_ListTasksRequest) SetNextToken(s *string) {
	this.NextToken_ = s
}
func NewListTasksRequest() ListTasksRequest {
	return &_ListTasksRequest{}
}
func init() {
	var val ListTasksRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("ListTasksRequest", t, func() interface{} {
		return NewListTasksRequest()
	})
}

type ListTasksResponse interface {
	SetTaskArns(s []*string)
	TaskArns() []*string
	SetNextToken(s *string)
	NextToken() *string
}
type _ListTasksResponse struct {
	TaskArns_  []*string `awsjson:"taskArns"`
	NextToken_ *string   `awsjson:"nextToken"`
}

func (this *_ListTasksResponse) NextToken() *string {
	return this.NextToken_
}
func (this *_ListTasksResponse) SetNextToken(s *string) {
	this.NextToken_ = s
}
func (this *_ListTasksResponse) TaskArns() []*string {
	return this.TaskArns_
}
func (this *_ListTasksResponse) SetTaskArns(s []*string) {
	this.TaskArns_ = s
}
func NewListTasksResponse() ListTasksResponse {
	return &_ListTasksResponse{}
}
func init() {
	var val ListTasksResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("ListTasksResponse", t, func() interface{} {
		return NewListTasksResponse()
	})
}

type NetworkBinding interface {
	SetHostPort(b *int32)
	HostPort() *int32
	SetBindIP(s *string)
	BindIP() *string
	SetContainerPort(b *int32)
	ContainerPort() *int32
}
type _NetworkBinding struct {
	BindIP_        *string `awsjson:"bindIP"`
	ContainerPort_ *int32  `awsjson:"containerPort"`
	HostPort_      *int32  `awsjson:"hostPort"`
}

func (this *_NetworkBinding) BindIP() *string {
	return this.BindIP_
}
func (this *_NetworkBinding) SetBindIP(s *string) {
	this.BindIP_ = s
}
func (this *_NetworkBinding) ContainerPort() *int32 {
	return this.ContainerPort_
}
func (this *_NetworkBinding) SetContainerPort(b *int32) {
	this.ContainerPort_ = b
}
func (this *_NetworkBinding) HostPort() *int32 {
	return this.HostPort_
}
func (this *_NetworkBinding) SetHostPort(b *int32) {
	this.HostPort_ = b
}
func NewNetworkBinding() NetworkBinding {
	return &_NetworkBinding{}
}
func init() {
	var val NetworkBinding
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("NetworkBinding", t, func() interface{} {
		return NewNetworkBinding()
	})
}

type PortMapping interface {
	SetContainerPort(i *int32)
	ContainerPort() *int32
	SetHostPort(i *int32)
	HostPort() *int32
}
type _PortMapping struct {
	ContainerPort_ *int32 `awsjson:"containerPort"`
	HostPort_      *int32 `awsjson:"hostPort"`
}

func (this *_PortMapping) ContainerPort() *int32 {
	return this.ContainerPort_
}
func (this *_PortMapping) SetContainerPort(i *int32) {
	this.ContainerPort_ = i
}
func (this *_PortMapping) HostPort() *int32 {
	return this.HostPort_
}
func (this *_PortMapping) SetHostPort(i *int32) {
	this.HostPort_ = i
}
func NewPortMapping() PortMapping {
	return &_PortMapping{}
}
func init() {
	var val PortMapping
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("PortMapping", t, func() interface{} {
		return NewPortMapping()
	})
}

type RegisterContainerInstanceRequest interface {
	SetCluster(s *string)
	Cluster() *string
	SetInstanceIdentityDocument(s *string)
	InstanceIdentityDocument() *string
	SetInstanceIdentityDocumentSignature(s *string)
	InstanceIdentityDocumentSignature() *string
	SetTotalResources(r []Resource)
	TotalResources() []Resource
}
type _RegisterContainerInstanceRequest struct {
	Cluster_                           *string    `awsjson:"cluster"`
	InstanceIdentityDocument_          *string    `awsjson:"instanceIdentityDocument"`
	InstanceIdentityDocumentSignature_ *string    `awsjson:"instanceIdentityDocumentSignature"`
	TotalResources_                    []Resource `awsjson:"totalResources"`
}

func (this *_RegisterContainerInstanceRequest) Cluster() *string {
	return this.Cluster_
}
func (this *_RegisterContainerInstanceRequest) SetCluster(s *string) {
	this.Cluster_ = s
}
func (this *_RegisterContainerInstanceRequest) InstanceIdentityDocument() *string {
	return this.InstanceIdentityDocument_
}
func (this *_RegisterContainerInstanceRequest) SetInstanceIdentityDocument(s *string) {
	this.InstanceIdentityDocument_ = s
}
func (this *_RegisterContainerInstanceRequest) InstanceIdentityDocumentSignature() *string {
	return this.InstanceIdentityDocumentSignature_
}
func (this *_RegisterContainerInstanceRequest) SetInstanceIdentityDocumentSignature(s *string) {
	this.InstanceIdentityDocumentSignature_ = s
}
func (this *_RegisterContainerInstanceRequest) TotalResources() []Resource {
	return this.TotalResources_
}
func (this *_RegisterContainerInstanceRequest) SetTotalResources(r []Resource) {
	this.TotalResources_ = r
}
func NewRegisterContainerInstanceRequest() RegisterContainerInstanceRequest {
	return &_RegisterContainerInstanceRequest{}
}
func init() {
	var val RegisterContainerInstanceRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("RegisterContainerInstanceRequest", t, func() interface{} {
		return NewRegisterContainerInstanceRequest()
	})
}

type RegisterContainerInstanceResponse interface {
	SetContainerInstance(c ContainerInstance)
	ContainerInstance() ContainerInstance
}
type _RegisterContainerInstanceResponse struct {
	ContainerInstance_ ContainerInstance `awsjson:"containerInstance"`
}

func (this *_RegisterContainerInstanceResponse) ContainerInstance() ContainerInstance {
	return this.ContainerInstance_
}
func (this *_RegisterContainerInstanceResponse) SetContainerInstance(c ContainerInstance) {
	this.ContainerInstance_ = c
}
func NewRegisterContainerInstanceResponse() RegisterContainerInstanceResponse {
	return &_RegisterContainerInstanceResponse{}
}
func init() {
	var val RegisterContainerInstanceResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("RegisterContainerInstanceResponse", t, func() interface{} {
		return NewRegisterContainerInstanceResponse()
	})
}

type RegisterTaskDefinitionRequest interface {
	SetFamily(s *string)
	Family() *string
	SetContainerDefinitions(c []ContainerDefinition)
	ContainerDefinitions() []ContainerDefinition
}
type _RegisterTaskDefinitionRequest struct {
	Family_               *string               `awsjson:"family"`
	ContainerDefinitions_ []ContainerDefinition `awsjson:"containerDefinitions"`
}

func (this *_RegisterTaskDefinitionRequest) ContainerDefinitions() []ContainerDefinition {
	return this.ContainerDefinitions_
}
func (this *_RegisterTaskDefinitionRequest) SetContainerDefinitions(c []ContainerDefinition) {
	this.ContainerDefinitions_ = c
}
func (this *_RegisterTaskDefinitionRequest) Family() *string {
	return this.Family_
}
func (this *_RegisterTaskDefinitionRequest) SetFamily(s *string) {
	this.Family_ = s
}
func NewRegisterTaskDefinitionRequest() RegisterTaskDefinitionRequest {
	return &_RegisterTaskDefinitionRequest{}
}
func init() {
	var val RegisterTaskDefinitionRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("RegisterTaskDefinitionRequest", t, func() interface{} {
		return NewRegisterTaskDefinitionRequest()
	})
}

type RegisterTaskDefinitionResponse interface {
	SetTaskDefinition(t TaskDefinition)
	TaskDefinition() TaskDefinition
}
type _RegisterTaskDefinitionResponse struct {
	TaskDefinition_ TaskDefinition `awsjson:"taskDefinition"`
}

func (this *_RegisterTaskDefinitionResponse) TaskDefinition() TaskDefinition {
	return this.TaskDefinition_
}
func (this *_RegisterTaskDefinitionResponse) SetTaskDefinition(t TaskDefinition) {
	this.TaskDefinition_ = t
}
func NewRegisterTaskDefinitionResponse() RegisterTaskDefinitionResponse {
	return &_RegisterTaskDefinitionResponse{}
}
func init() {
	var val RegisterTaskDefinitionResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("RegisterTaskDefinitionResponse", t, func() interface{} {
		return NewRegisterTaskDefinitionResponse()
	})
}

type Resource interface {
	SetIntegerValue(i *int32)
	IntegerValue() *int32
	SetStringSetValue(s []*string)
	StringSetValue() []*string
	SetName(s *string)
	Name() *string
	SetType(s *string)
	Type() *string
	SetDoubleValue(d *float64)
	DoubleValue() *float64
	SetLongValue(l *int64)
	LongValue() *int64
}
type _Resource struct {
	Name_           *string   `awsjson:"name"`
	Type_           *string   `awsjson:"type"`
	DoubleValue_    *float64  `awsjson:"doubleValue"`
	LongValue_      *int64    `awsjson:"longValue"`
	IntegerValue_   *int32    `awsjson:"integerValue"`
	StringSetValue_ []*string `awsjson:"stringSetValue"`
}

func (this *_Resource) DoubleValue() *float64 {
	return this.DoubleValue_
}
func (this *_Resource) SetDoubleValue(d *float64) {
	this.DoubleValue_ = d
}
func (this *_Resource) IntegerValue() *int32 {
	return this.IntegerValue_
}
func (this *_Resource) SetIntegerValue(i *int32) {
	this.IntegerValue_ = i
}
func (this *_Resource) LongValue() *int64 {
	return this.LongValue_
}
func (this *_Resource) SetLongValue(l *int64) {
	this.LongValue_ = l
}
func (this *_Resource) Name() *string {
	return this.Name_
}
func (this *_Resource) SetName(s *string) {
	this.Name_ = s
}
func (this *_Resource) StringSetValue() []*string {
	return this.StringSetValue_
}
func (this *_Resource) SetStringSetValue(s []*string) {
	this.StringSetValue_ = s
}
func (this *_Resource) Type() *string {
	return this.Type_
}
func (this *_Resource) SetType(s *string) {
	this.Type_ = s
}
func NewResource() Resource {
	return &_Resource{}
}
func init() {
	var val Resource
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("Resource", t, func() interface{} {
		return NewResource()
	})
}

type RunTaskRequest interface {
	SetOverrides(t TaskOverride)
	Overrides() TaskOverride
	SetCount(b *int32)
	Count() *int32
	SetCluster(s *string)
	Cluster() *string
	SetTaskDefinition(s *string)
	TaskDefinition() *string
}
type _RunTaskRequest struct {
	Cluster_        *string      `awsjson:"cluster"`
	TaskDefinition_ *string      `awsjson:"taskDefinition"`
	Overrides_      TaskOverride `awsjson:"overrides"`
	Count_          *int32       `awsjson:"count"`
}

func (this *_RunTaskRequest) Cluster() *string {
	return this.Cluster_
}
func (this *_RunTaskRequest) SetCluster(s *string) {
	this.Cluster_ = s
}
func (this *_RunTaskRequest) Count() *int32 {
	return this.Count_
}
func (this *_RunTaskRequest) SetCount(b *int32) {
	this.Count_ = b
}
func (this *_RunTaskRequest) Overrides() TaskOverride {
	return this.Overrides_
}
func (this *_RunTaskRequest) SetOverrides(t TaskOverride) {
	this.Overrides_ = t
}
func (this *_RunTaskRequest) TaskDefinition() *string {
	return this.TaskDefinition_
}
func (this *_RunTaskRequest) SetTaskDefinition(s *string) {
	this.TaskDefinition_ = s
}
func NewRunTaskRequest() RunTaskRequest {
	return &_RunTaskRequest{}
}
func init() {
	var val RunTaskRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("RunTaskRequest", t, func() interface{} {
		return NewRunTaskRequest()
	})
}

type RunTaskResponse interface {
	SetTasks(t []Task)
	Tasks() []Task
	SetFailures(f []Failure)
	Failures() []Failure
}
type _RunTaskResponse struct {
	Tasks_    []Task    `awsjson:"tasks"`
	Failures_ []Failure `awsjson:"failures"`
}

func (this *_RunTaskResponse) Failures() []Failure {
	return this.Failures_
}
func (this *_RunTaskResponse) SetFailures(f []Failure) {
	this.Failures_ = f
}
func (this *_RunTaskResponse) Tasks() []Task {
	return this.Tasks_
}
func (this *_RunTaskResponse) SetTasks(t []Task) {
	this.Tasks_ = t
}
func NewRunTaskResponse() RunTaskResponse {
	return &_RunTaskResponse{}
}
func init() {
	var val RunTaskResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("RunTaskResponse", t, func() interface{} {
		return NewRunTaskResponse()
	})
}

type ServerException interface {
	error
	SetMessage(s *string)
	Message() *string
}
type _ServerException struct {
	Message_ *string `awsjson:"message"`
}

func (this *_ServerException) Error() string {
	return __model__.ErrorMessage(this)
}
func (this *_ServerException) Message() *string {
	return this.Message_
}
func (this *_ServerException) SetMessage(s *string) {
	this.Message_ = s
}
func NewServerException() ServerException {
	return &_ServerException{}
}
func init() {
	var val ServerException
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("ServerException", t, func() interface{} {
		return NewServerException()
	})
}

type StartTaskRequest interface {
	SetOverrides(t TaskOverride)
	Overrides() TaskOverride
	SetContainerInstances(s []*string)
	ContainerInstances() []*string
	SetCluster(s *string)
	Cluster() *string
	SetTaskDefinition(s *string)
	TaskDefinition() *string
}
type _StartTaskRequest struct {
	ContainerInstances_ []*string    `awsjson:"containerInstances"`
	Cluster_            *string      `awsjson:"cluster"`
	TaskDefinition_     *string      `awsjson:"taskDefinition"`
	Overrides_          TaskOverride `awsjson:"overrides"`
}

func (this *_StartTaskRequest) Cluster() *string {
	return this.Cluster_
}
func (this *_StartTaskRequest) SetCluster(s *string) {
	this.Cluster_ = s
}
func (this *_StartTaskRequest) ContainerInstances() []*string {
	return this.ContainerInstances_
}
func (this *_StartTaskRequest) SetContainerInstances(s []*string) {
	this.ContainerInstances_ = s
}
func (this *_StartTaskRequest) Overrides() TaskOverride {
	return this.Overrides_
}
func (this *_StartTaskRequest) SetOverrides(t TaskOverride) {
	this.Overrides_ = t
}
func (this *_StartTaskRequest) TaskDefinition() *string {
	return this.TaskDefinition_
}
func (this *_StartTaskRequest) SetTaskDefinition(s *string) {
	this.TaskDefinition_ = s
}
func NewStartTaskRequest() StartTaskRequest {
	return &_StartTaskRequest{}
}
func init() {
	var val StartTaskRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("StartTaskRequest", t, func() interface{} {
		return NewStartTaskRequest()
	})
}

type StartTaskResponse interface {
	SetTasks(t []Task)
	Tasks() []Task
	SetFailures(f []Failure)
	Failures() []Failure
}
type _StartTaskResponse struct {
	Tasks_    []Task    `awsjson:"tasks"`
	Failures_ []Failure `awsjson:"failures"`
}

func (this *_StartTaskResponse) Failures() []Failure {
	return this.Failures_
}
func (this *_StartTaskResponse) SetFailures(f []Failure) {
	this.Failures_ = f
}
func (this *_StartTaskResponse) Tasks() []Task {
	return this.Tasks_
}
func (this *_StartTaskResponse) SetTasks(t []Task) {
	this.Tasks_ = t
}
func NewStartTaskResponse() StartTaskResponse {
	return &_StartTaskResponse{}
}
func init() {
	var val StartTaskResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("StartTaskResponse", t, func() interface{} {
		return NewStartTaskResponse()
	})
}

type StopTaskRequest interface {
	SetCluster(s *string)
	Cluster() *string
	SetTask(s *string)
	Task() *string
}
type _StopTaskRequest struct {
	Cluster_ *string `awsjson:"cluster"`
	Task_    *string `awsjson:"task"`
}

func (this *_StopTaskRequest) Cluster() *string {
	return this.Cluster_
}
func (this *_StopTaskRequest) SetCluster(s *string) {
	this.Cluster_ = s
}
func (this *_StopTaskRequest) Task() *string {
	return this.Task_
}
func (this *_StopTaskRequest) SetTask(s *string) {
	this.Task_ = s
}
func NewStopTaskRequest() StopTaskRequest {
	return &_StopTaskRequest{}
}
func init() {
	var val StopTaskRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("StopTaskRequest", t, func() interface{} {
		return NewStopTaskRequest()
	})
}

type StopTaskResponse interface {
	SetTask(t Task)
	Task() Task
}
type _StopTaskResponse struct {
	Task_ Task `awsjson:"task"`
}

func (this *_StopTaskResponse) Task() Task {
	return this.Task_
}
func (this *_StopTaskResponse) SetTask(t Task) {
	this.Task_ = t
}
func NewStopTaskResponse() StopTaskResponse {
	return &_StopTaskResponse{}
}
func init() {
	var val StopTaskResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("StopTaskResponse", t, func() interface{} {
		return NewStopTaskResponse()
	})
}

type SubmitContainerStateChangeRequest interface {
	SetCluster(s *string)
	Cluster() *string
	SetTask(s *string)
	Task() *string
	SetContainerName(s *string)
	ContainerName() *string
	SetStatus(s *string)
	Status() *string
	SetExitCode(b *int32)
	ExitCode() *int32
	SetReason(s *string)
	Reason() *string
	SetNetworkBindings(n []NetworkBinding)
	NetworkBindings() []NetworkBinding
}
type _SubmitContainerStateChangeRequest struct {
	Task_            *string          `awsjson:"task"`
	ContainerName_   *string          `awsjson:"containerName"`
	Status_          *string          `awsjson:"status"`
	ExitCode_        *int32           `awsjson:"exitCode"`
	Reason_          *string          `awsjson:"reason"`
	NetworkBindings_ []NetworkBinding `awsjson:"networkBindings"`
	Cluster_         *string          `awsjson:"cluster"`
}

func (this *_SubmitContainerStateChangeRequest) Cluster() *string {
	return this.Cluster_
}
func (this *_SubmitContainerStateChangeRequest) SetCluster(s *string) {
	this.Cluster_ = s
}
func (this *_SubmitContainerStateChangeRequest) ContainerName() *string {
	return this.ContainerName_
}
func (this *_SubmitContainerStateChangeRequest) SetContainerName(s *string) {
	this.ContainerName_ = s
}
func (this *_SubmitContainerStateChangeRequest) ExitCode() *int32 {
	return this.ExitCode_
}
func (this *_SubmitContainerStateChangeRequest) SetExitCode(b *int32) {
	this.ExitCode_ = b
}
func (this *_SubmitContainerStateChangeRequest) NetworkBindings() []NetworkBinding {
	return this.NetworkBindings_
}
func (this *_SubmitContainerStateChangeRequest) SetNetworkBindings(n []NetworkBinding) {
	this.NetworkBindings_ = n
}
func (this *_SubmitContainerStateChangeRequest) Reason() *string {
	return this.Reason_
}
func (this *_SubmitContainerStateChangeRequest) SetReason(s *string) {
	this.Reason_ = s
}
func (this *_SubmitContainerStateChangeRequest) Status() *string {
	return this.Status_
}
func (this *_SubmitContainerStateChangeRequest) SetStatus(s *string) {
	this.Status_ = s
}
func (this *_SubmitContainerStateChangeRequest) Task() *string {
	return this.Task_
}
func (this *_SubmitContainerStateChangeRequest) SetTask(s *string) {
	this.Task_ = s
}
func NewSubmitContainerStateChangeRequest() SubmitContainerStateChangeRequest {
	return &_SubmitContainerStateChangeRequest{}
}
func init() {
	var val SubmitContainerStateChangeRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("SubmitContainerStateChangeRequest", t, func() interface{} {
		return NewSubmitContainerStateChangeRequest()
	})
}

type SubmitContainerStateChangeResponse interface {
	SetAcknowledgment(s *string)
	Acknowledgment() *string
}
type _SubmitContainerStateChangeResponse struct {
	Acknowledgment_ *string `awsjson:"acknowledgment"`
}

func (this *_SubmitContainerStateChangeResponse) Acknowledgment() *string {
	return this.Acknowledgment_
}
func (this *_SubmitContainerStateChangeResponse) SetAcknowledgment(s *string) {
	this.Acknowledgment_ = s
}
func NewSubmitContainerStateChangeResponse() SubmitContainerStateChangeResponse {
	return &_SubmitContainerStateChangeResponse{}
}
func init() {
	var val SubmitContainerStateChangeResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("SubmitContainerStateChangeResponse", t, func() interface{} {
		return NewSubmitContainerStateChangeResponse()
	})
}

type SubmitTaskStateChangeRequest interface {
	SetStatus(s *string)
	Status() *string
	SetReason(s *string)
	Reason() *string
	SetCluster(s *string)
	Cluster() *string
	SetTask(s *string)
	Task() *string
}
type _SubmitTaskStateChangeRequest struct {
	Reason_  *string `awsjson:"reason"`
	Cluster_ *string `awsjson:"cluster"`
	Task_    *string `awsjson:"task"`
	Status_  *string `awsjson:"status"`
}

func (this *_SubmitTaskStateChangeRequest) Cluster() *string {
	return this.Cluster_
}
func (this *_SubmitTaskStateChangeRequest) SetCluster(s *string) {
	this.Cluster_ = s
}
func (this *_SubmitTaskStateChangeRequest) Reason() *string {
	return this.Reason_
}
func (this *_SubmitTaskStateChangeRequest) SetReason(s *string) {
	this.Reason_ = s
}
func (this *_SubmitTaskStateChangeRequest) Status() *string {
	return this.Status_
}
func (this *_SubmitTaskStateChangeRequest) SetStatus(s *string) {
	this.Status_ = s
}
func (this *_SubmitTaskStateChangeRequest) Task() *string {
	return this.Task_
}
func (this *_SubmitTaskStateChangeRequest) SetTask(s *string) {
	this.Task_ = s
}
func NewSubmitTaskStateChangeRequest() SubmitTaskStateChangeRequest {
	return &_SubmitTaskStateChangeRequest{}
}
func init() {
	var val SubmitTaskStateChangeRequest
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("SubmitTaskStateChangeRequest", t, func() interface{} {
		return NewSubmitTaskStateChangeRequest()
	})
}

type SubmitTaskStateChangeResponse interface {
	SetAcknowledgment(s *string)
	Acknowledgment() *string
}
type _SubmitTaskStateChangeResponse struct {
	Acknowledgment_ *string `awsjson:"acknowledgment"`
}

func (this *_SubmitTaskStateChangeResponse) Acknowledgment() *string {
	return this.Acknowledgment_
}
func (this *_SubmitTaskStateChangeResponse) SetAcknowledgment(s *string) {
	this.Acknowledgment_ = s
}
func NewSubmitTaskStateChangeResponse() SubmitTaskStateChangeResponse {
	return &_SubmitTaskStateChangeResponse{}
}
func init() {
	var val SubmitTaskStateChangeResponse
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("SubmitTaskStateChangeResponse", t, func() interface{} {
		return NewSubmitTaskStateChangeResponse()
	})
}

type Task interface {
	SetContainerInstanceArn(s *string)
	ContainerInstanceArn() *string
	SetOverrides(t TaskOverride)
	Overrides() TaskOverride
	SetLastStatus(s *string)
	LastStatus() *string
	SetDesiredStatus(s *string)
	DesiredStatus() *string
	SetContainers(c []Container)
	Containers() []Container
	SetTaskArn(s *string)
	TaskArn() *string
	SetClusterArn(s *string)
	ClusterArn() *string
	SetTaskDefinitionArn(s *string)
	TaskDefinitionArn() *string
}
type _Task struct {
	ClusterArn_           *string      `awsjson:"clusterArn"`
	TaskDefinitionArn_    *string      `awsjson:"taskDefinitionArn"`
	ContainerInstanceArn_ *string      `awsjson:"containerInstanceArn"`
	Overrides_            TaskOverride `awsjson:"overrides"`
	LastStatus_           *string      `awsjson:"lastStatus"`
	DesiredStatus_        *string      `awsjson:"desiredStatus"`
	Containers_           []Container  `awsjson:"containers"`
	TaskArn_              *string      `awsjson:"taskArn"`
}

func (this *_Task) ClusterArn() *string {
	return this.ClusterArn_
}
func (this *_Task) SetClusterArn(s *string) {
	this.ClusterArn_ = s
}
func (this *_Task) ContainerInstanceArn() *string {
	return this.ContainerInstanceArn_
}
func (this *_Task) SetContainerInstanceArn(s *string) {
	this.ContainerInstanceArn_ = s
}
func (this *_Task) Containers() []Container {
	return this.Containers_
}
func (this *_Task) SetContainers(c []Container) {
	this.Containers_ = c
}
func (this *_Task) DesiredStatus() *string {
	return this.DesiredStatus_
}
func (this *_Task) SetDesiredStatus(s *string) {
	this.DesiredStatus_ = s
}
func (this *_Task) LastStatus() *string {
	return this.LastStatus_
}
func (this *_Task) SetLastStatus(s *string) {
	this.LastStatus_ = s
}
func (this *_Task) Overrides() TaskOverride {
	return this.Overrides_
}
func (this *_Task) SetOverrides(t TaskOverride) {
	this.Overrides_ = t
}
func (this *_Task) TaskArn() *string {
	return this.TaskArn_
}
func (this *_Task) SetTaskArn(s *string) {
	this.TaskArn_ = s
}
func (this *_Task) TaskDefinitionArn() *string {
	return this.TaskDefinitionArn_
}
func (this *_Task) SetTaskDefinitionArn(s *string) {
	this.TaskDefinitionArn_ = s
}
func NewTask() Task {
	return &_Task{}
}
func init() {
	var val Task
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("Task", t, func() interface{} {
		return NewTask()
	})
}

type TaskDefinition interface {
	SetTaskDefinitionArn(s *string)
	TaskDefinitionArn() *string
	SetContainerDefinitions(c []ContainerDefinition)
	ContainerDefinitions() []ContainerDefinition
	SetFamily(s *string)
	Family() *string
	SetRevision(i *int32)
	Revision() *int32
}
type _TaskDefinition struct {
	TaskDefinitionArn_    *string               `awsjson:"taskDefinitionArn"`
	ContainerDefinitions_ []ContainerDefinition `awsjson:"containerDefinitions"`
	Family_               *string               `awsjson:"family"`
	Revision_             *int32                `awsjson:"revision"`
}

func (this *_TaskDefinition) ContainerDefinitions() []ContainerDefinition {
	return this.ContainerDefinitions_
}
func (this *_TaskDefinition) SetContainerDefinitions(c []ContainerDefinition) {
	this.ContainerDefinitions_ = c
}
func (this *_TaskDefinition) Family() *string {
	return this.Family_
}
func (this *_TaskDefinition) SetFamily(s *string) {
	this.Family_ = s
}
func (this *_TaskDefinition) Revision() *int32 {
	return this.Revision_
}
func (this *_TaskDefinition) SetRevision(i *int32) {
	this.Revision_ = i
}
func (this *_TaskDefinition) TaskDefinitionArn() *string {
	return this.TaskDefinitionArn_
}
func (this *_TaskDefinition) SetTaskDefinitionArn(s *string) {
	this.TaskDefinitionArn_ = s
}
func NewTaskDefinition() TaskDefinition {
	return &_TaskDefinition{}
}
func init() {
	var val TaskDefinition
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("TaskDefinition", t, func() interface{} {
		return NewTaskDefinition()
	})
}

type TaskOverride interface {
	SetContainerOverrides(c []ContainerOverride)
	ContainerOverrides() []ContainerOverride
}
type _TaskOverride struct {
	ContainerOverrides_ []ContainerOverride `awsjson:"containerOverrides"`
}

func (this *_TaskOverride) ContainerOverrides() []ContainerOverride {
	return this.ContainerOverrides_
}
func (this *_TaskOverride) SetContainerOverrides(c []ContainerOverride) {
	this.ContainerOverrides_ = c
}
func NewTaskOverride() TaskOverride {
	return &_TaskOverride{}
}
func init() {
	var val TaskOverride
	t := __reflect__.TypeOf(&val)
	__model__.RegisterShape("TaskOverride", t, func() interface{} {
		return NewTaskOverride()
	})
}
