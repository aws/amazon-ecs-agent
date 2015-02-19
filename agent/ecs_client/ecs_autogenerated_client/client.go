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
	__client__ "github.com/aws/amazon-ecs-agent/agent/ecs_client/client/client"
	__dialer__ "github.com/aws/amazon-ecs-agent/agent/ecs_client/client/dialer"
	__codec__ "github.com/aws/amazon-ecs-agent/agent/ecs_client/codec/codec"
)

type AmazonEC2ContainerServiceV20141113Client struct {
	C __client__.Client
}

//Creates a new AmazonEC2ContainerServiceV20141113Client
func NewAmazonEC2ContainerServiceV20141113Client(dialer __dialer__.Dialer, codec __codec__.Codec) (service *AmazonEC2ContainerServiceV20141113Client) {
	return &AmazonEC2ContainerServiceV20141113Client{__client__.NewClient("AmazonEC2ContainerServiceV20141113", dialer, codec)}
}
func (this *AmazonEC2ContainerServiceV20141113Client) DescribeClusters(input DescribeClustersRequest) (DescribeClustersResponse, error) {
	var output DescribeClustersResponse
	err := this.C.Call("DescribeClusters", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) DeregisterContainerInstance(input DeregisterContainerInstanceRequest) (DeregisterContainerInstanceResponse, error) {
	var output DeregisterContainerInstanceResponse
	err := this.C.Call("DeregisterContainerInstance", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) DeregisterTaskDefinition(input DeregisterTaskDefinitionRequest) (DeregisterTaskDefinitionResponse, error) {
	var output DeregisterTaskDefinitionResponse
	err := this.C.Call("DeregisterTaskDefinition", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) ListTasks(input ListTasksRequest) (ListTasksResponse, error) {
	var output ListTasksResponse
	err := this.C.Call("ListTasks", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) SubmitTaskStateChange(input SubmitTaskStateChangeRequest) (SubmitTaskStateChangeResponse, error) {
	var output SubmitTaskStateChangeResponse
	err := this.C.Call("SubmitTaskStateChange", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) CreateCluster(input CreateClusterRequest) (CreateClusterResponse, error) {
	var output CreateClusterResponse
	err := this.C.Call("CreateCluster", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) DeleteCluster(input DeleteClusterRequest) (DeleteClusterResponse, error) {
	var output DeleteClusterResponse
	err := this.C.Call("DeleteCluster", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) RegisterContainerInstance(input RegisterContainerInstanceRequest) (RegisterContainerInstanceResponse, error) {
	var output RegisterContainerInstanceResponse
	err := this.C.Call("RegisterContainerInstance", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) DiscoverPollEndpoint(input DiscoverPollEndpointRequest) (DiscoverPollEndpointResponse, error) {
	var output DiscoverPollEndpointResponse
	err := this.C.Call("DiscoverPollEndpoint", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) ListClusters(input ListClustersRequest) (ListClustersResponse, error) {
	var output ListClustersResponse
	err := this.C.Call("ListClusters", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) RunTask(input RunTaskRequest) (RunTaskResponse, error) {
	var output RunTaskResponse
	err := this.C.Call("RunTask", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) SubmitContainerStateChange(input SubmitContainerStateChangeRequest) (SubmitContainerStateChangeResponse, error) {
	var output SubmitContainerStateChangeResponse
	err := this.C.Call("SubmitContainerStateChange", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) DescribeContainerInstances(input DescribeContainerInstancesRequest) (DescribeContainerInstancesResponse, error) {
	var output DescribeContainerInstancesResponse
	err := this.C.Call("DescribeContainerInstances", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) RegisterTaskDefinition(input RegisterTaskDefinitionRequest) (RegisterTaskDefinitionResponse, error) {
	var output RegisterTaskDefinitionResponse
	err := this.C.Call("RegisterTaskDefinition", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) DescribeTaskDefinition(input DescribeTaskDefinitionRequest) (DescribeTaskDefinitionResponse, error) {
	var output DescribeTaskDefinitionResponse
	err := this.C.Call("DescribeTaskDefinition", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) ListTaskDefinitions(input ListTaskDefinitionsRequest) (ListTaskDefinitionsResponse, error) {
	var output ListTaskDefinitionsResponse
	err := this.C.Call("ListTaskDefinitions", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) StartTask(input StartTaskRequest) (StartTaskResponse, error) {
	var output StartTaskResponse
	err := this.C.Call("StartTask", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) DescribeTasks(input DescribeTasksRequest) (DescribeTasksResponse, error) {
	var output DescribeTasksResponse
	err := this.C.Call("DescribeTasks", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) StopTask(input StopTaskRequest) (StopTaskResponse, error) {
	var output StopTaskResponse
	err := this.C.Call("StopTask", input, &output)
	return output, err
}
func (this *AmazonEC2ContainerServiceV20141113Client) ListContainerInstances(input ListContainerInstancesRequest) (ListContainerInstancesResponse, error) {
	var output ListContainerInstancesResponse
	err := this.C.Call("ListContainerInstances", input, &output)
	return output, err
}
