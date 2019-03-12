// Copyright 2014-2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package ecs

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/request"
)

const opCreateCluster = "CreateCluster"

// CreateClusterRequest generates a "aws/request.Request" representing the
// client's request for the CreateCluster operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See CreateCluster for more information on using the CreateCluster
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the CreateClusterRequest method.
//    req, resp := client.CreateClusterRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) CreateClusterRequest(input *CreateClusterInput) (req *request.Request, output *CreateClusterOutput) {
	op := &request.Operation{
		Name:       opCreateCluster,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &CreateClusterInput{}
	}

	output = &CreateClusterOutput{}
	req = c.newRequest(op, input, output)
	return
}

// CreateCluster API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation CreateCluster for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) CreateCluster(input *CreateClusterInput) (*CreateClusterOutput, error) {
	req, out := c.CreateClusterRequest(input)
	return out, req.Send()
}

// CreateClusterWithContext is the same as CreateCluster with the addition of
// the ability to pass a context and additional request options.
//
// See CreateCluster for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) CreateClusterWithContext(ctx aws.Context, input *CreateClusterInput, opts ...request.Option) (*CreateClusterOutput, error) {
	req, out := c.CreateClusterRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opCreateService = "CreateService"

// CreateServiceRequest generates a "aws/request.Request" representing the
// client's request for the CreateService operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See CreateService for more information on using the CreateService
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the CreateServiceRequest method.
//    req, resp := client.CreateServiceRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) CreateServiceRequest(input *CreateServiceInput) (req *request.Request, output *CreateServiceOutput) {
	op := &request.Operation{
		Name:       opCreateService,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &CreateServiceInput{}
	}

	output = &CreateServiceOutput{}
	req = c.newRequest(op, input, output)
	return
}

// CreateService API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation CreateService for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeUnsupportedFeatureException "UnsupportedFeatureException"
//
//   * ErrCodePlatformUnknownException "PlatformUnknownException"
//
//   * ErrCodePlatformTaskDefinitionIncompatibilityException "PlatformTaskDefinitionIncompatibilityException"
//
//   * ErrCodeAccessDeniedException "AccessDeniedException"
//
func (c *ECS) CreateService(input *CreateServiceInput) (*CreateServiceOutput, error) {
	req, out := c.CreateServiceRequest(input)
	return out, req.Send()
}

// CreateServiceWithContext is the same as CreateService with the addition of
// the ability to pass a context and additional request options.
//
// See CreateService for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) CreateServiceWithContext(ctx aws.Context, input *CreateServiceInput, opts ...request.Option) (*CreateServiceOutput, error) {
	req, out := c.CreateServiceRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opCreateTaskSet = "CreateTaskSet"

// CreateTaskSetRequest generates a "aws/request.Request" representing the
// client's request for the CreateTaskSet operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See CreateTaskSet for more information on using the CreateTaskSet
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the CreateTaskSetRequest method.
//    req, resp := client.CreateTaskSetRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) CreateTaskSetRequest(input *CreateTaskSetInput) (req *request.Request, output *CreateTaskSetOutput) {
	op := &request.Operation{
		Name:       opCreateTaskSet,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &CreateTaskSetInput{}
	}

	output = &CreateTaskSetOutput{}
	req = c.newRequest(op, input, output)
	return
}

// CreateTaskSet API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation CreateTaskSet for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeUnsupportedFeatureException "UnsupportedFeatureException"
//
//   * ErrCodePlatformUnknownException "PlatformUnknownException"
//
//   * ErrCodePlatformTaskDefinitionIncompatibilityException "PlatformTaskDefinitionIncompatibilityException"
//
//   * ErrCodeAccessDeniedException "AccessDeniedException"
//
//   * ErrCodeLimitExceededException "LimitExceededException"
//
//   * ErrCodeServiceNotFoundException "ServiceNotFoundException"
//
//   * ErrCodeServiceNotActiveException "ServiceNotActiveException"
//
func (c *ECS) CreateTaskSet(input *CreateTaskSetInput) (*CreateTaskSetOutput, error) {
	req, out := c.CreateTaskSetRequest(input)
	return out, req.Send()
}

// CreateTaskSetWithContext is the same as CreateTaskSet with the addition of
// the ability to pass a context and additional request options.
//
// See CreateTaskSet for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) CreateTaskSetWithContext(ctx aws.Context, input *CreateTaskSetInput, opts ...request.Option) (*CreateTaskSetOutput, error) {
	req, out := c.CreateTaskSetRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opDeleteAccountSetting = "DeleteAccountSetting"

// DeleteAccountSettingRequest generates a "aws/request.Request" representing the
// client's request for the DeleteAccountSetting operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DeleteAccountSetting for more information on using the DeleteAccountSetting
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DeleteAccountSettingRequest method.
//    req, resp := client.DeleteAccountSettingRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) DeleteAccountSettingRequest(input *DeleteAccountSettingInput) (req *request.Request, output *DeleteAccountSettingOutput) {
	op := &request.Operation{
		Name:       opDeleteAccountSetting,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &DeleteAccountSettingInput{}
	}

	output = &DeleteAccountSettingOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DeleteAccountSetting API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation DeleteAccountSetting for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) DeleteAccountSetting(input *DeleteAccountSettingInput) (*DeleteAccountSettingOutput, error) {
	req, out := c.DeleteAccountSettingRequest(input)
	return out, req.Send()
}

// DeleteAccountSettingWithContext is the same as DeleteAccountSetting with the addition of
// the ability to pass a context and additional request options.
//
// See DeleteAccountSetting for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) DeleteAccountSettingWithContext(ctx aws.Context, input *DeleteAccountSettingInput, opts ...request.Option) (*DeleteAccountSettingOutput, error) {
	req, out := c.DeleteAccountSettingRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opDeleteAttributes = "DeleteAttributes"

// DeleteAttributesRequest generates a "aws/request.Request" representing the
// client's request for the DeleteAttributes operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DeleteAttributes for more information on using the DeleteAttributes
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DeleteAttributesRequest method.
//    req, resp := client.DeleteAttributesRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) DeleteAttributesRequest(input *DeleteAttributesInput) (req *request.Request, output *DeleteAttributesOutput) {
	op := &request.Operation{
		Name:       opDeleteAttributes,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &DeleteAttributesInput{}
	}

	output = &DeleteAttributesOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DeleteAttributes API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation DeleteAttributes for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeTargetNotFoundException "TargetNotFoundException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) DeleteAttributes(input *DeleteAttributesInput) (*DeleteAttributesOutput, error) {
	req, out := c.DeleteAttributesRequest(input)
	return out, req.Send()
}

// DeleteAttributesWithContext is the same as DeleteAttributes with the addition of
// the ability to pass a context and additional request options.
//
// See DeleteAttributes for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) DeleteAttributesWithContext(ctx aws.Context, input *DeleteAttributesInput, opts ...request.Option) (*DeleteAttributesOutput, error) {
	req, out := c.DeleteAttributesRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opDeleteCluster = "DeleteCluster"

// DeleteClusterRequest generates a "aws/request.Request" representing the
// client's request for the DeleteCluster operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DeleteCluster for more information on using the DeleteCluster
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DeleteClusterRequest method.
//    req, resp := client.DeleteClusterRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) DeleteClusterRequest(input *DeleteClusterInput) (req *request.Request, output *DeleteClusterOutput) {
	op := &request.Operation{
		Name:       opDeleteCluster,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &DeleteClusterInput{}
	}

	output = &DeleteClusterOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DeleteCluster API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation DeleteCluster for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeClusterContainsContainerInstancesException "ClusterContainsContainerInstancesException"
//
//   * ErrCodeClusterContainsServicesException "ClusterContainsServicesException"
//
//   * ErrCodeClusterContainsTasksException "ClusterContainsTasksException"
//
func (c *ECS) DeleteCluster(input *DeleteClusterInput) (*DeleteClusterOutput, error) {
	req, out := c.DeleteClusterRequest(input)
	return out, req.Send()
}

// DeleteClusterWithContext is the same as DeleteCluster with the addition of
// the ability to pass a context and additional request options.
//
// See DeleteCluster for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) DeleteClusterWithContext(ctx aws.Context, input *DeleteClusterInput, opts ...request.Option) (*DeleteClusterOutput, error) {
	req, out := c.DeleteClusterRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opDeleteService = "DeleteService"

// DeleteServiceRequest generates a "aws/request.Request" representing the
// client's request for the DeleteService operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DeleteService for more information on using the DeleteService
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DeleteServiceRequest method.
//    req, resp := client.DeleteServiceRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) DeleteServiceRequest(input *DeleteServiceInput) (req *request.Request, output *DeleteServiceOutput) {
	op := &request.Operation{
		Name:       opDeleteService,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &DeleteServiceInput{}
	}

	output = &DeleteServiceOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DeleteService API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation DeleteService for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeServiceNotFoundException "ServiceNotFoundException"
//
func (c *ECS) DeleteService(input *DeleteServiceInput) (*DeleteServiceOutput, error) {
	req, out := c.DeleteServiceRequest(input)
	return out, req.Send()
}

// DeleteServiceWithContext is the same as DeleteService with the addition of
// the ability to pass a context and additional request options.
//
// See DeleteService for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) DeleteServiceWithContext(ctx aws.Context, input *DeleteServiceInput, opts ...request.Option) (*DeleteServiceOutput, error) {
	req, out := c.DeleteServiceRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opDeleteTaskSet = "DeleteTaskSet"

// DeleteTaskSetRequest generates a "aws/request.Request" representing the
// client's request for the DeleteTaskSet operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DeleteTaskSet for more information on using the DeleteTaskSet
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DeleteTaskSetRequest method.
//    req, resp := client.DeleteTaskSetRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) DeleteTaskSetRequest(input *DeleteTaskSetInput) (req *request.Request, output *DeleteTaskSetOutput) {
	op := &request.Operation{
		Name:       opDeleteTaskSet,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &DeleteTaskSetInput{}
	}

	output = &DeleteTaskSetOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DeleteTaskSet API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation DeleteTaskSet for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeUnsupportedFeatureException "UnsupportedFeatureException"
//
//   * ErrCodeAccessDeniedException "AccessDeniedException"
//
//   * ErrCodeLimitExceededException "LimitExceededException"
//
//   * ErrCodeServiceNotFoundException "ServiceNotFoundException"
//
//   * ErrCodeServiceNotActiveException "ServiceNotActiveException"
//
//   * ErrCodeTaskSetNotFoundException "TaskSetNotFoundException"
//
func (c *ECS) DeleteTaskSet(input *DeleteTaskSetInput) (*DeleteTaskSetOutput, error) {
	req, out := c.DeleteTaskSetRequest(input)
	return out, req.Send()
}

// DeleteTaskSetWithContext is the same as DeleteTaskSet with the addition of
// the ability to pass a context and additional request options.
//
// See DeleteTaskSet for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) DeleteTaskSetWithContext(ctx aws.Context, input *DeleteTaskSetInput, opts ...request.Option) (*DeleteTaskSetOutput, error) {
	req, out := c.DeleteTaskSetRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opDeregisterContainerInstance = "DeregisterContainerInstance"

// DeregisterContainerInstanceRequest generates a "aws/request.Request" representing the
// client's request for the DeregisterContainerInstance operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DeregisterContainerInstance for more information on using the DeregisterContainerInstance
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DeregisterContainerInstanceRequest method.
//    req, resp := client.DeregisterContainerInstanceRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) DeregisterContainerInstanceRequest(input *DeregisterContainerInstanceInput) (req *request.Request, output *DeregisterContainerInstanceOutput) {
	op := &request.Operation{
		Name:       opDeregisterContainerInstance,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &DeregisterContainerInstanceInput{}
	}

	output = &DeregisterContainerInstanceOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DeregisterContainerInstance API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation DeregisterContainerInstance for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
func (c *ECS) DeregisterContainerInstance(input *DeregisterContainerInstanceInput) (*DeregisterContainerInstanceOutput, error) {
	req, out := c.DeregisterContainerInstanceRequest(input)
	return out, req.Send()
}

// DeregisterContainerInstanceWithContext is the same as DeregisterContainerInstance with the addition of
// the ability to pass a context and additional request options.
//
// See DeregisterContainerInstance for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) DeregisterContainerInstanceWithContext(ctx aws.Context, input *DeregisterContainerInstanceInput, opts ...request.Option) (*DeregisterContainerInstanceOutput, error) {
	req, out := c.DeregisterContainerInstanceRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opDeregisterTaskDefinition = "DeregisterTaskDefinition"

// DeregisterTaskDefinitionRequest generates a "aws/request.Request" representing the
// client's request for the DeregisterTaskDefinition operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DeregisterTaskDefinition for more information on using the DeregisterTaskDefinition
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DeregisterTaskDefinitionRequest method.
//    req, resp := client.DeregisterTaskDefinitionRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) DeregisterTaskDefinitionRequest(input *DeregisterTaskDefinitionInput) (req *request.Request, output *DeregisterTaskDefinitionOutput) {
	op := &request.Operation{
		Name:       opDeregisterTaskDefinition,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &DeregisterTaskDefinitionInput{}
	}

	output = &DeregisterTaskDefinitionOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DeregisterTaskDefinition API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation DeregisterTaskDefinition for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) DeregisterTaskDefinition(input *DeregisterTaskDefinitionInput) (*DeregisterTaskDefinitionOutput, error) {
	req, out := c.DeregisterTaskDefinitionRequest(input)
	return out, req.Send()
}

// DeregisterTaskDefinitionWithContext is the same as DeregisterTaskDefinition with the addition of
// the ability to pass a context and additional request options.
//
// See DeregisterTaskDefinition for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) DeregisterTaskDefinitionWithContext(ctx aws.Context, input *DeregisterTaskDefinitionInput, opts ...request.Option) (*DeregisterTaskDefinitionOutput, error) {
	req, out := c.DeregisterTaskDefinitionRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opDescribeClusters = "DescribeClusters"

// DescribeClustersRequest generates a "aws/request.Request" representing the
// client's request for the DescribeClusters operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DescribeClusters for more information on using the DescribeClusters
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DescribeClustersRequest method.
//    req, resp := client.DescribeClustersRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) DescribeClustersRequest(input *DescribeClustersInput) (req *request.Request, output *DescribeClustersOutput) {
	op := &request.Operation{
		Name:       opDescribeClusters,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &DescribeClustersInput{}
	}

	output = &DescribeClustersOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DescribeClusters API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation DescribeClusters for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) DescribeClusters(input *DescribeClustersInput) (*DescribeClustersOutput, error) {
	req, out := c.DescribeClustersRequest(input)
	return out, req.Send()
}

// DescribeClustersWithContext is the same as DescribeClusters with the addition of
// the ability to pass a context and additional request options.
//
// See DescribeClusters for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) DescribeClustersWithContext(ctx aws.Context, input *DescribeClustersInput, opts ...request.Option) (*DescribeClustersOutput, error) {
	req, out := c.DescribeClustersRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opDescribeContainerInstances = "DescribeContainerInstances"

// DescribeContainerInstancesRequest generates a "aws/request.Request" representing the
// client's request for the DescribeContainerInstances operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DescribeContainerInstances for more information on using the DescribeContainerInstances
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DescribeContainerInstancesRequest method.
//    req, resp := client.DescribeContainerInstancesRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) DescribeContainerInstancesRequest(input *DescribeContainerInstancesInput) (req *request.Request, output *DescribeContainerInstancesOutput) {
	op := &request.Operation{
		Name:       opDescribeContainerInstances,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &DescribeContainerInstancesInput{}
	}

	output = &DescribeContainerInstancesOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DescribeContainerInstances API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation DescribeContainerInstances for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
func (c *ECS) DescribeContainerInstances(input *DescribeContainerInstancesInput) (*DescribeContainerInstancesOutput, error) {
	req, out := c.DescribeContainerInstancesRequest(input)
	return out, req.Send()
}

// DescribeContainerInstancesWithContext is the same as DescribeContainerInstances with the addition of
// the ability to pass a context and additional request options.
//
// See DescribeContainerInstances for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) DescribeContainerInstancesWithContext(ctx aws.Context, input *DescribeContainerInstancesInput, opts ...request.Option) (*DescribeContainerInstancesOutput, error) {
	req, out := c.DescribeContainerInstancesRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opDescribeServices = "DescribeServices"

// DescribeServicesRequest generates a "aws/request.Request" representing the
// client's request for the DescribeServices operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DescribeServices for more information on using the DescribeServices
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DescribeServicesRequest method.
//    req, resp := client.DescribeServicesRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) DescribeServicesRequest(input *DescribeServicesInput) (req *request.Request, output *DescribeServicesOutput) {
	op := &request.Operation{
		Name:       opDescribeServices,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &DescribeServicesInput{}
	}

	output = &DescribeServicesOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DescribeServices API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation DescribeServices for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
func (c *ECS) DescribeServices(input *DescribeServicesInput) (*DescribeServicesOutput, error) {
	req, out := c.DescribeServicesRequest(input)
	return out, req.Send()
}

// DescribeServicesWithContext is the same as DescribeServices with the addition of
// the ability to pass a context and additional request options.
//
// See DescribeServices for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) DescribeServicesWithContext(ctx aws.Context, input *DescribeServicesInput, opts ...request.Option) (*DescribeServicesOutput, error) {
	req, out := c.DescribeServicesRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opDescribeTaskDefinition = "DescribeTaskDefinition"

// DescribeTaskDefinitionRequest generates a "aws/request.Request" representing the
// client's request for the DescribeTaskDefinition operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DescribeTaskDefinition for more information on using the DescribeTaskDefinition
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DescribeTaskDefinitionRequest method.
//    req, resp := client.DescribeTaskDefinitionRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) DescribeTaskDefinitionRequest(input *DescribeTaskDefinitionInput) (req *request.Request, output *DescribeTaskDefinitionOutput) {
	op := &request.Operation{
		Name:       opDescribeTaskDefinition,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &DescribeTaskDefinitionInput{}
	}

	output = &DescribeTaskDefinitionOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DescribeTaskDefinition API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation DescribeTaskDefinition for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) DescribeTaskDefinition(input *DescribeTaskDefinitionInput) (*DescribeTaskDefinitionOutput, error) {
	req, out := c.DescribeTaskDefinitionRequest(input)
	return out, req.Send()
}

// DescribeTaskDefinitionWithContext is the same as DescribeTaskDefinition with the addition of
// the ability to pass a context and additional request options.
//
// See DescribeTaskDefinition for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) DescribeTaskDefinitionWithContext(ctx aws.Context, input *DescribeTaskDefinitionInput, opts ...request.Option) (*DescribeTaskDefinitionOutput, error) {
	req, out := c.DescribeTaskDefinitionRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opDescribeTaskSets = "DescribeTaskSets"

// DescribeTaskSetsRequest generates a "aws/request.Request" representing the
// client's request for the DescribeTaskSets operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DescribeTaskSets for more information on using the DescribeTaskSets
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DescribeTaskSetsRequest method.
//    req, resp := client.DescribeTaskSetsRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) DescribeTaskSetsRequest(input *DescribeTaskSetsInput) (req *request.Request, output *DescribeTaskSetsOutput) {
	op := &request.Operation{
		Name:       opDescribeTaskSets,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &DescribeTaskSetsInput{}
	}

	output = &DescribeTaskSetsOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DescribeTaskSets API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation DescribeTaskSets for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeServiceNotFoundException "ServiceNotFoundException"
//
func (c *ECS) DescribeTaskSets(input *DescribeTaskSetsInput) (*DescribeTaskSetsOutput, error) {
	req, out := c.DescribeTaskSetsRequest(input)
	return out, req.Send()
}

// DescribeTaskSetsWithContext is the same as DescribeTaskSets with the addition of
// the ability to pass a context and additional request options.
//
// See DescribeTaskSets for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) DescribeTaskSetsWithContext(ctx aws.Context, input *DescribeTaskSetsInput, opts ...request.Option) (*DescribeTaskSetsOutput, error) {
	req, out := c.DescribeTaskSetsRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opDescribeTasks = "DescribeTasks"

// DescribeTasksRequest generates a "aws/request.Request" representing the
// client's request for the DescribeTasks operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DescribeTasks for more information on using the DescribeTasks
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DescribeTasksRequest method.
//    req, resp := client.DescribeTasksRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) DescribeTasksRequest(input *DescribeTasksInput) (req *request.Request, output *DescribeTasksOutput) {
	op := &request.Operation{
		Name:       opDescribeTasks,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &DescribeTasksInput{}
	}

	output = &DescribeTasksOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DescribeTasks API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation DescribeTasks for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
func (c *ECS) DescribeTasks(input *DescribeTasksInput) (*DescribeTasksOutput, error) {
	req, out := c.DescribeTasksRequest(input)
	return out, req.Send()
}

// DescribeTasksWithContext is the same as DescribeTasks with the addition of
// the ability to pass a context and additional request options.
//
// See DescribeTasks for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) DescribeTasksWithContext(ctx aws.Context, input *DescribeTasksInput, opts ...request.Option) (*DescribeTasksOutput, error) {
	req, out := c.DescribeTasksRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opDiscoverPollEndpoint = "DiscoverPollEndpoint"

// DiscoverPollEndpointRequest generates a "aws/request.Request" representing the
// client's request for the DiscoverPollEndpoint operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DiscoverPollEndpoint for more information on using the DiscoverPollEndpoint
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DiscoverPollEndpointRequest method.
//    req, resp := client.DiscoverPollEndpointRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) DiscoverPollEndpointRequest(input *DiscoverPollEndpointInput) (req *request.Request, output *DiscoverPollEndpointOutput) {
	op := &request.Operation{
		Name:       opDiscoverPollEndpoint,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &DiscoverPollEndpointInput{}
	}

	output = &DiscoverPollEndpointOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DiscoverPollEndpoint API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation DiscoverPollEndpoint for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
func (c *ECS) DiscoverPollEndpoint(input *DiscoverPollEndpointInput) (*DiscoverPollEndpointOutput, error) {
	req, out := c.DiscoverPollEndpointRequest(input)
	return out, req.Send()
}

// DiscoverPollEndpointWithContext is the same as DiscoverPollEndpoint with the addition of
// the ability to pass a context and additional request options.
//
// See DiscoverPollEndpoint for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) DiscoverPollEndpointWithContext(ctx aws.Context, input *DiscoverPollEndpointInput, opts ...request.Option) (*DiscoverPollEndpointOutput, error) {
	req, out := c.DiscoverPollEndpointRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opListAccountSettings = "ListAccountSettings"

// ListAccountSettingsRequest generates a "aws/request.Request" representing the
// client's request for the ListAccountSettings operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See ListAccountSettings for more information on using the ListAccountSettings
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the ListAccountSettingsRequest method.
//    req, resp := client.ListAccountSettingsRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) ListAccountSettingsRequest(input *ListAccountSettingsInput) (req *request.Request, output *ListAccountSettingsOutput) {
	op := &request.Operation{
		Name:       opListAccountSettings,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &ListAccountSettingsInput{}
	}

	output = &ListAccountSettingsOutput{}
	req = c.newRequest(op, input, output)
	return
}

// ListAccountSettings API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation ListAccountSettings for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) ListAccountSettings(input *ListAccountSettingsInput) (*ListAccountSettingsOutput, error) {
	req, out := c.ListAccountSettingsRequest(input)
	return out, req.Send()
}

// ListAccountSettingsWithContext is the same as ListAccountSettings with the addition of
// the ability to pass a context and additional request options.
//
// See ListAccountSettings for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) ListAccountSettingsWithContext(ctx aws.Context, input *ListAccountSettingsInput, opts ...request.Option) (*ListAccountSettingsOutput, error) {
	req, out := c.ListAccountSettingsRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opListAttributes = "ListAttributes"

// ListAttributesRequest generates a "aws/request.Request" representing the
// client's request for the ListAttributes operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See ListAttributes for more information on using the ListAttributes
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the ListAttributesRequest method.
//    req, resp := client.ListAttributesRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) ListAttributesRequest(input *ListAttributesInput) (req *request.Request, output *ListAttributesOutput) {
	op := &request.Operation{
		Name:       opListAttributes,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &ListAttributesInput{}
	}

	output = &ListAttributesOutput{}
	req = c.newRequest(op, input, output)
	return
}

// ListAttributes API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation ListAttributes for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) ListAttributes(input *ListAttributesInput) (*ListAttributesOutput, error) {
	req, out := c.ListAttributesRequest(input)
	return out, req.Send()
}

// ListAttributesWithContext is the same as ListAttributes with the addition of
// the ability to pass a context and additional request options.
//
// See ListAttributes for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) ListAttributesWithContext(ctx aws.Context, input *ListAttributesInput, opts ...request.Option) (*ListAttributesOutput, error) {
	req, out := c.ListAttributesRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opListClusters = "ListClusters"

// ListClustersRequest generates a "aws/request.Request" representing the
// client's request for the ListClusters operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See ListClusters for more information on using the ListClusters
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the ListClustersRequest method.
//    req, resp := client.ListClustersRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) ListClustersRequest(input *ListClustersInput) (req *request.Request, output *ListClustersOutput) {
	op := &request.Operation{
		Name:       opListClusters,
		HTTPMethod: "POST",
		HTTPPath:   "/",
		Paginator: &request.Paginator{
			InputTokens:     []string{"nextToken"},
			OutputTokens:    []string{"nextToken"},
			LimitToken:      "maxResults",
			TruncationToken: "",
		},
	}

	if input == nil {
		input = &ListClustersInput{}
	}

	output = &ListClustersOutput{}
	req = c.newRequest(op, input, output)
	return
}

// ListClusters API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation ListClusters for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) ListClusters(input *ListClustersInput) (*ListClustersOutput, error) {
	req, out := c.ListClustersRequest(input)
	return out, req.Send()
}

// ListClustersWithContext is the same as ListClusters with the addition of
// the ability to pass a context and additional request options.
//
// See ListClusters for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) ListClustersWithContext(ctx aws.Context, input *ListClustersInput, opts ...request.Option) (*ListClustersOutput, error) {
	req, out := c.ListClustersRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

// ListClustersPages iterates over the pages of a ListClusters operation,
// calling the "fn" function with the response data for each page. To stop
// iterating, return false from the fn function.
//
// See ListClusters method for more information on how to use this operation.
//
// Note: This operation can generate multiple requests to a service.
//
//    // Example iterating over at most 3 pages of a ListClusters operation.
//    pageNum := 0
//    err := client.ListClustersPages(params,
//        func(page *ListClustersOutput, lastPage bool) bool {
//            pageNum++
//            fmt.Println(page)
//            return pageNum <= 3
//        })
//
func (c *ECS) ListClustersPages(input *ListClustersInput, fn func(*ListClustersOutput, bool) bool) error {
	return c.ListClustersPagesWithContext(aws.BackgroundContext(), input, fn)
}

// ListClustersPagesWithContext same as ListClustersPages except
// it takes a Context and allows setting request options on the pages.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) ListClustersPagesWithContext(ctx aws.Context, input *ListClustersInput, fn func(*ListClustersOutput, bool) bool, opts ...request.Option) error {
	p := request.Pagination{
		NewRequest: func() (*request.Request, error) {
			var inCpy *ListClustersInput
			if input != nil {
				tmp := *input
				inCpy = &tmp
			}
			req, _ := c.ListClustersRequest(inCpy)
			req.SetContext(ctx)
			req.ApplyOptions(opts...)
			return req, nil
		},
	}

	cont := true
	for p.Next() && cont {
		cont = fn(p.Page().(*ListClustersOutput), !p.HasNextPage())
	}
	return p.Err()
}

const opListContainerInstances = "ListContainerInstances"

// ListContainerInstancesRequest generates a "aws/request.Request" representing the
// client's request for the ListContainerInstances operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See ListContainerInstances for more information on using the ListContainerInstances
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the ListContainerInstancesRequest method.
//    req, resp := client.ListContainerInstancesRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) ListContainerInstancesRequest(input *ListContainerInstancesInput) (req *request.Request, output *ListContainerInstancesOutput) {
	op := &request.Operation{
		Name:       opListContainerInstances,
		HTTPMethod: "POST",
		HTTPPath:   "/",
		Paginator: &request.Paginator{
			InputTokens:     []string{"nextToken"},
			OutputTokens:    []string{"nextToken"},
			LimitToken:      "maxResults",
			TruncationToken: "",
		},
	}

	if input == nil {
		input = &ListContainerInstancesInput{}
	}

	output = &ListContainerInstancesOutput{}
	req = c.newRequest(op, input, output)
	return
}

// ListContainerInstances API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation ListContainerInstances for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
func (c *ECS) ListContainerInstances(input *ListContainerInstancesInput) (*ListContainerInstancesOutput, error) {
	req, out := c.ListContainerInstancesRequest(input)
	return out, req.Send()
}

// ListContainerInstancesWithContext is the same as ListContainerInstances with the addition of
// the ability to pass a context and additional request options.
//
// See ListContainerInstances for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) ListContainerInstancesWithContext(ctx aws.Context, input *ListContainerInstancesInput, opts ...request.Option) (*ListContainerInstancesOutput, error) {
	req, out := c.ListContainerInstancesRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

// ListContainerInstancesPages iterates over the pages of a ListContainerInstances operation,
// calling the "fn" function with the response data for each page. To stop
// iterating, return false from the fn function.
//
// See ListContainerInstances method for more information on how to use this operation.
//
// Note: This operation can generate multiple requests to a service.
//
//    // Example iterating over at most 3 pages of a ListContainerInstances operation.
//    pageNum := 0
//    err := client.ListContainerInstancesPages(params,
//        func(page *ListContainerInstancesOutput, lastPage bool) bool {
//            pageNum++
//            fmt.Println(page)
//            return pageNum <= 3
//        })
//
func (c *ECS) ListContainerInstancesPages(input *ListContainerInstancesInput, fn func(*ListContainerInstancesOutput, bool) bool) error {
	return c.ListContainerInstancesPagesWithContext(aws.BackgroundContext(), input, fn)
}

// ListContainerInstancesPagesWithContext same as ListContainerInstancesPages except
// it takes a Context and allows setting request options on the pages.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) ListContainerInstancesPagesWithContext(ctx aws.Context, input *ListContainerInstancesInput, fn func(*ListContainerInstancesOutput, bool) bool, opts ...request.Option) error {
	p := request.Pagination{
		NewRequest: func() (*request.Request, error) {
			var inCpy *ListContainerInstancesInput
			if input != nil {
				tmp := *input
				inCpy = &tmp
			}
			req, _ := c.ListContainerInstancesRequest(inCpy)
			req.SetContext(ctx)
			req.ApplyOptions(opts...)
			return req, nil
		},
	}

	cont := true
	for p.Next() && cont {
		cont = fn(p.Page().(*ListContainerInstancesOutput), !p.HasNextPage())
	}
	return p.Err()
}

const opListServices = "ListServices"

// ListServicesRequest generates a "aws/request.Request" representing the
// client's request for the ListServices operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See ListServices for more information on using the ListServices
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the ListServicesRequest method.
//    req, resp := client.ListServicesRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) ListServicesRequest(input *ListServicesInput) (req *request.Request, output *ListServicesOutput) {
	op := &request.Operation{
		Name:       opListServices,
		HTTPMethod: "POST",
		HTTPPath:   "/",
		Paginator: &request.Paginator{
			InputTokens:     []string{"nextToken"},
			OutputTokens:    []string{"nextToken"},
			LimitToken:      "maxResults",
			TruncationToken: "",
		},
	}

	if input == nil {
		input = &ListServicesInput{}
	}

	output = &ListServicesOutput{}
	req = c.newRequest(op, input, output)
	return
}

// ListServices API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation ListServices for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
func (c *ECS) ListServices(input *ListServicesInput) (*ListServicesOutput, error) {
	req, out := c.ListServicesRequest(input)
	return out, req.Send()
}

// ListServicesWithContext is the same as ListServices with the addition of
// the ability to pass a context and additional request options.
//
// See ListServices for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) ListServicesWithContext(ctx aws.Context, input *ListServicesInput, opts ...request.Option) (*ListServicesOutput, error) {
	req, out := c.ListServicesRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

// ListServicesPages iterates over the pages of a ListServices operation,
// calling the "fn" function with the response data for each page. To stop
// iterating, return false from the fn function.
//
// See ListServices method for more information on how to use this operation.
//
// Note: This operation can generate multiple requests to a service.
//
//    // Example iterating over at most 3 pages of a ListServices operation.
//    pageNum := 0
//    err := client.ListServicesPages(params,
//        func(page *ListServicesOutput, lastPage bool) bool {
//            pageNum++
//            fmt.Println(page)
//            return pageNum <= 3
//        })
//
func (c *ECS) ListServicesPages(input *ListServicesInput, fn func(*ListServicesOutput, bool) bool) error {
	return c.ListServicesPagesWithContext(aws.BackgroundContext(), input, fn)
}

// ListServicesPagesWithContext same as ListServicesPages except
// it takes a Context and allows setting request options on the pages.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) ListServicesPagesWithContext(ctx aws.Context, input *ListServicesInput, fn func(*ListServicesOutput, bool) bool, opts ...request.Option) error {
	p := request.Pagination{
		NewRequest: func() (*request.Request, error) {
			var inCpy *ListServicesInput
			if input != nil {
				tmp := *input
				inCpy = &tmp
			}
			req, _ := c.ListServicesRequest(inCpy)
			req.SetContext(ctx)
			req.ApplyOptions(opts...)
			return req, nil
		},
	}

	cont := true
	for p.Next() && cont {
		cont = fn(p.Page().(*ListServicesOutput), !p.HasNextPage())
	}
	return p.Err()
}

const opListTagsForResource = "ListTagsForResource"

// ListTagsForResourceRequest generates a "aws/request.Request" representing the
// client's request for the ListTagsForResource operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See ListTagsForResource for more information on using the ListTagsForResource
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the ListTagsForResourceRequest method.
//    req, resp := client.ListTagsForResourceRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) ListTagsForResourceRequest(input *ListTagsForResourceInput) (req *request.Request, output *ListTagsForResourceOutput) {
	op := &request.Operation{
		Name:       opListTagsForResource,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &ListTagsForResourceInput{}
	}

	output = &ListTagsForResourceOutput{}
	req = c.newRequest(op, input, output)
	return
}

// ListTagsForResource API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation ListTagsForResource for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) ListTagsForResource(input *ListTagsForResourceInput) (*ListTagsForResourceOutput, error) {
	req, out := c.ListTagsForResourceRequest(input)
	return out, req.Send()
}

// ListTagsForResourceWithContext is the same as ListTagsForResource with the addition of
// the ability to pass a context and additional request options.
//
// See ListTagsForResource for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) ListTagsForResourceWithContext(ctx aws.Context, input *ListTagsForResourceInput, opts ...request.Option) (*ListTagsForResourceOutput, error) {
	req, out := c.ListTagsForResourceRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opListTaskDefinitionFamilies = "ListTaskDefinitionFamilies"

// ListTaskDefinitionFamiliesRequest generates a "aws/request.Request" representing the
// client's request for the ListTaskDefinitionFamilies operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See ListTaskDefinitionFamilies for more information on using the ListTaskDefinitionFamilies
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the ListTaskDefinitionFamiliesRequest method.
//    req, resp := client.ListTaskDefinitionFamiliesRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) ListTaskDefinitionFamiliesRequest(input *ListTaskDefinitionFamiliesInput) (req *request.Request, output *ListTaskDefinitionFamiliesOutput) {
	op := &request.Operation{
		Name:       opListTaskDefinitionFamilies,
		HTTPMethod: "POST",
		HTTPPath:   "/",
		Paginator: &request.Paginator{
			InputTokens:     []string{"nextToken"},
			OutputTokens:    []string{"nextToken"},
			LimitToken:      "maxResults",
			TruncationToken: "",
		},
	}

	if input == nil {
		input = &ListTaskDefinitionFamiliesInput{}
	}

	output = &ListTaskDefinitionFamiliesOutput{}
	req = c.newRequest(op, input, output)
	return
}

// ListTaskDefinitionFamilies API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation ListTaskDefinitionFamilies for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) ListTaskDefinitionFamilies(input *ListTaskDefinitionFamiliesInput) (*ListTaskDefinitionFamiliesOutput, error) {
	req, out := c.ListTaskDefinitionFamiliesRequest(input)
	return out, req.Send()
}

// ListTaskDefinitionFamiliesWithContext is the same as ListTaskDefinitionFamilies with the addition of
// the ability to pass a context and additional request options.
//
// See ListTaskDefinitionFamilies for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) ListTaskDefinitionFamiliesWithContext(ctx aws.Context, input *ListTaskDefinitionFamiliesInput, opts ...request.Option) (*ListTaskDefinitionFamiliesOutput, error) {
	req, out := c.ListTaskDefinitionFamiliesRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

// ListTaskDefinitionFamiliesPages iterates over the pages of a ListTaskDefinitionFamilies operation,
// calling the "fn" function with the response data for each page. To stop
// iterating, return false from the fn function.
//
// See ListTaskDefinitionFamilies method for more information on how to use this operation.
//
// Note: This operation can generate multiple requests to a service.
//
//    // Example iterating over at most 3 pages of a ListTaskDefinitionFamilies operation.
//    pageNum := 0
//    err := client.ListTaskDefinitionFamiliesPages(params,
//        func(page *ListTaskDefinitionFamiliesOutput, lastPage bool) bool {
//            pageNum++
//            fmt.Println(page)
//            return pageNum <= 3
//        })
//
func (c *ECS) ListTaskDefinitionFamiliesPages(input *ListTaskDefinitionFamiliesInput, fn func(*ListTaskDefinitionFamiliesOutput, bool) bool) error {
	return c.ListTaskDefinitionFamiliesPagesWithContext(aws.BackgroundContext(), input, fn)
}

// ListTaskDefinitionFamiliesPagesWithContext same as ListTaskDefinitionFamiliesPages except
// it takes a Context and allows setting request options on the pages.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) ListTaskDefinitionFamiliesPagesWithContext(ctx aws.Context, input *ListTaskDefinitionFamiliesInput, fn func(*ListTaskDefinitionFamiliesOutput, bool) bool, opts ...request.Option) error {
	p := request.Pagination{
		NewRequest: func() (*request.Request, error) {
			var inCpy *ListTaskDefinitionFamiliesInput
			if input != nil {
				tmp := *input
				inCpy = &tmp
			}
			req, _ := c.ListTaskDefinitionFamiliesRequest(inCpy)
			req.SetContext(ctx)
			req.ApplyOptions(opts...)
			return req, nil
		},
	}

	cont := true
	for p.Next() && cont {
		cont = fn(p.Page().(*ListTaskDefinitionFamiliesOutput), !p.HasNextPage())
	}
	return p.Err()
}

const opListTaskDefinitions = "ListTaskDefinitions"

// ListTaskDefinitionsRequest generates a "aws/request.Request" representing the
// client's request for the ListTaskDefinitions operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See ListTaskDefinitions for more information on using the ListTaskDefinitions
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the ListTaskDefinitionsRequest method.
//    req, resp := client.ListTaskDefinitionsRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) ListTaskDefinitionsRequest(input *ListTaskDefinitionsInput) (req *request.Request, output *ListTaskDefinitionsOutput) {
	op := &request.Operation{
		Name:       opListTaskDefinitions,
		HTTPMethod: "POST",
		HTTPPath:   "/",
		Paginator: &request.Paginator{
			InputTokens:     []string{"nextToken"},
			OutputTokens:    []string{"nextToken"},
			LimitToken:      "maxResults",
			TruncationToken: "",
		},
	}

	if input == nil {
		input = &ListTaskDefinitionsInput{}
	}

	output = &ListTaskDefinitionsOutput{}
	req = c.newRequest(op, input, output)
	return
}

// ListTaskDefinitions API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation ListTaskDefinitions for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) ListTaskDefinitions(input *ListTaskDefinitionsInput) (*ListTaskDefinitionsOutput, error) {
	req, out := c.ListTaskDefinitionsRequest(input)
	return out, req.Send()
}

// ListTaskDefinitionsWithContext is the same as ListTaskDefinitions with the addition of
// the ability to pass a context and additional request options.
//
// See ListTaskDefinitions for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) ListTaskDefinitionsWithContext(ctx aws.Context, input *ListTaskDefinitionsInput, opts ...request.Option) (*ListTaskDefinitionsOutput, error) {
	req, out := c.ListTaskDefinitionsRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

// ListTaskDefinitionsPages iterates over the pages of a ListTaskDefinitions operation,
// calling the "fn" function with the response data for each page. To stop
// iterating, return false from the fn function.
//
// See ListTaskDefinitions method for more information on how to use this operation.
//
// Note: This operation can generate multiple requests to a service.
//
//    // Example iterating over at most 3 pages of a ListTaskDefinitions operation.
//    pageNum := 0
//    err := client.ListTaskDefinitionsPages(params,
//        func(page *ListTaskDefinitionsOutput, lastPage bool) bool {
//            pageNum++
//            fmt.Println(page)
//            return pageNum <= 3
//        })
//
func (c *ECS) ListTaskDefinitionsPages(input *ListTaskDefinitionsInput, fn func(*ListTaskDefinitionsOutput, bool) bool) error {
	return c.ListTaskDefinitionsPagesWithContext(aws.BackgroundContext(), input, fn)
}

// ListTaskDefinitionsPagesWithContext same as ListTaskDefinitionsPages except
// it takes a Context and allows setting request options on the pages.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) ListTaskDefinitionsPagesWithContext(ctx aws.Context, input *ListTaskDefinitionsInput, fn func(*ListTaskDefinitionsOutput, bool) bool, opts ...request.Option) error {
	p := request.Pagination{
		NewRequest: func() (*request.Request, error) {
			var inCpy *ListTaskDefinitionsInput
			if input != nil {
				tmp := *input
				inCpy = &tmp
			}
			req, _ := c.ListTaskDefinitionsRequest(inCpy)
			req.SetContext(ctx)
			req.ApplyOptions(opts...)
			return req, nil
		},
	}

	cont := true
	for p.Next() && cont {
		cont = fn(p.Page().(*ListTaskDefinitionsOutput), !p.HasNextPage())
	}
	return p.Err()
}

const opListTasks = "ListTasks"

// ListTasksRequest generates a "aws/request.Request" representing the
// client's request for the ListTasks operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See ListTasks for more information on using the ListTasks
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the ListTasksRequest method.
//    req, resp := client.ListTasksRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) ListTasksRequest(input *ListTasksInput) (req *request.Request, output *ListTasksOutput) {
	op := &request.Operation{
		Name:       opListTasks,
		HTTPMethod: "POST",
		HTTPPath:   "/",
		Paginator: &request.Paginator{
			InputTokens:     []string{"nextToken"},
			OutputTokens:    []string{"nextToken"},
			LimitToken:      "maxResults",
			TruncationToken: "",
		},
	}

	if input == nil {
		input = &ListTasksInput{}
	}

	output = &ListTasksOutput{}
	req = c.newRequest(op, input, output)
	return
}

// ListTasks API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation ListTasks for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeServiceNotFoundException "ServiceNotFoundException"
//
func (c *ECS) ListTasks(input *ListTasksInput) (*ListTasksOutput, error) {
	req, out := c.ListTasksRequest(input)
	return out, req.Send()
}

// ListTasksWithContext is the same as ListTasks with the addition of
// the ability to pass a context and additional request options.
//
// See ListTasks for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) ListTasksWithContext(ctx aws.Context, input *ListTasksInput, opts ...request.Option) (*ListTasksOutput, error) {
	req, out := c.ListTasksRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

// ListTasksPages iterates over the pages of a ListTasks operation,
// calling the "fn" function with the response data for each page. To stop
// iterating, return false from the fn function.
//
// See ListTasks method for more information on how to use this operation.
//
// Note: This operation can generate multiple requests to a service.
//
//    // Example iterating over at most 3 pages of a ListTasks operation.
//    pageNum := 0
//    err := client.ListTasksPages(params,
//        func(page *ListTasksOutput, lastPage bool) bool {
//            pageNum++
//            fmt.Println(page)
//            return pageNum <= 3
//        })
//
func (c *ECS) ListTasksPages(input *ListTasksInput, fn func(*ListTasksOutput, bool) bool) error {
	return c.ListTasksPagesWithContext(aws.BackgroundContext(), input, fn)
}

// ListTasksPagesWithContext same as ListTasksPages except
// it takes a Context and allows setting request options on the pages.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) ListTasksPagesWithContext(ctx aws.Context, input *ListTasksInput, fn func(*ListTasksOutput, bool) bool, opts ...request.Option) error {
	p := request.Pagination{
		NewRequest: func() (*request.Request, error) {
			var inCpy *ListTasksInput
			if input != nil {
				tmp := *input
				inCpy = &tmp
			}
			req, _ := c.ListTasksRequest(inCpy)
			req.SetContext(ctx)
			req.ApplyOptions(opts...)
			return req, nil
		},
	}

	cont := true
	for p.Next() && cont {
		cont = fn(p.Page().(*ListTasksOutput), !p.HasNextPage())
	}
	return p.Err()
}

const opPutAccountSetting = "PutAccountSetting"

// PutAccountSettingRequest generates a "aws/request.Request" representing the
// client's request for the PutAccountSetting operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See PutAccountSetting for more information on using the PutAccountSetting
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the PutAccountSettingRequest method.
//    req, resp := client.PutAccountSettingRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) PutAccountSettingRequest(input *PutAccountSettingInput) (req *request.Request, output *PutAccountSettingOutput) {
	op := &request.Operation{
		Name:       opPutAccountSetting,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &PutAccountSettingInput{}
	}

	output = &PutAccountSettingOutput{}
	req = c.newRequest(op, input, output)
	return
}

// PutAccountSetting API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation PutAccountSetting for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) PutAccountSetting(input *PutAccountSettingInput) (*PutAccountSettingOutput, error) {
	req, out := c.PutAccountSettingRequest(input)
	return out, req.Send()
}

// PutAccountSettingWithContext is the same as PutAccountSetting with the addition of
// the ability to pass a context and additional request options.
//
// See PutAccountSetting for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) PutAccountSettingWithContext(ctx aws.Context, input *PutAccountSettingInput, opts ...request.Option) (*PutAccountSettingOutput, error) {
	req, out := c.PutAccountSettingRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opPutAccountSettingDefault = "PutAccountSettingDefault"

// PutAccountSettingDefaultRequest generates a "aws/request.Request" representing the
// client's request for the PutAccountSettingDefault operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See PutAccountSettingDefault for more information on using the PutAccountSettingDefault
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the PutAccountSettingDefaultRequest method.
//    req, resp := client.PutAccountSettingDefaultRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) PutAccountSettingDefaultRequest(input *PutAccountSettingDefaultInput) (req *request.Request, output *PutAccountSettingDefaultOutput) {
	op := &request.Operation{
		Name:       opPutAccountSettingDefault,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &PutAccountSettingDefaultInput{}
	}

	output = &PutAccountSettingDefaultOutput{}
	req = c.newRequest(op, input, output)
	return
}

// PutAccountSettingDefault API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation PutAccountSettingDefault for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) PutAccountSettingDefault(input *PutAccountSettingDefaultInput) (*PutAccountSettingDefaultOutput, error) {
	req, out := c.PutAccountSettingDefaultRequest(input)
	return out, req.Send()
}

// PutAccountSettingDefaultWithContext is the same as PutAccountSettingDefault with the addition of
// the ability to pass a context and additional request options.
//
// See PutAccountSettingDefault for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) PutAccountSettingDefaultWithContext(ctx aws.Context, input *PutAccountSettingDefaultInput, opts ...request.Option) (*PutAccountSettingDefaultOutput, error) {
	req, out := c.PutAccountSettingDefaultRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opPutAttributes = "PutAttributes"

// PutAttributesRequest generates a "aws/request.Request" representing the
// client's request for the PutAttributes operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See PutAttributes for more information on using the PutAttributes
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the PutAttributesRequest method.
//    req, resp := client.PutAttributesRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) PutAttributesRequest(input *PutAttributesInput) (req *request.Request, output *PutAttributesOutput) {
	op := &request.Operation{
		Name:       opPutAttributes,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &PutAttributesInput{}
	}

	output = &PutAttributesOutput{}
	req = c.newRequest(op, input, output)
	return
}

// PutAttributes API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation PutAttributes for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeTargetNotFoundException "TargetNotFoundException"
//
//   * ErrCodeAttributeLimitExceededException "AttributeLimitExceededException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) PutAttributes(input *PutAttributesInput) (*PutAttributesOutput, error) {
	req, out := c.PutAttributesRequest(input)
	return out, req.Send()
}

// PutAttributesWithContext is the same as PutAttributes with the addition of
// the ability to pass a context and additional request options.
//
// See PutAttributes for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) PutAttributesWithContext(ctx aws.Context, input *PutAttributesInput, opts ...request.Option) (*PutAttributesOutput, error) {
	req, out := c.PutAttributesRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opRegisterContainerInstance = "RegisterContainerInstance"

// RegisterContainerInstanceRequest generates a "aws/request.Request" representing the
// client's request for the RegisterContainerInstance operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See RegisterContainerInstance for more information on using the RegisterContainerInstance
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the RegisterContainerInstanceRequest method.
//    req, resp := client.RegisterContainerInstanceRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) RegisterContainerInstanceRequest(input *RegisterContainerInstanceInput) (req *request.Request, output *RegisterContainerInstanceOutput) {
	op := &request.Operation{
		Name:       opRegisterContainerInstance,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &RegisterContainerInstanceInput{}
	}

	output = &RegisterContainerInstanceOutput{}
	req = c.newRequest(op, input, output)
	return
}

// RegisterContainerInstance API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation RegisterContainerInstance for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) RegisterContainerInstance(input *RegisterContainerInstanceInput) (*RegisterContainerInstanceOutput, error) {
	req, out := c.RegisterContainerInstanceRequest(input)
	return out, req.Send()
}

// RegisterContainerInstanceWithContext is the same as RegisterContainerInstance with the addition of
// the ability to pass a context and additional request options.
//
// See RegisterContainerInstance for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) RegisterContainerInstanceWithContext(ctx aws.Context, input *RegisterContainerInstanceInput, opts ...request.Option) (*RegisterContainerInstanceOutput, error) {
	req, out := c.RegisterContainerInstanceRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opRegisterTaskDefinition = "RegisterTaskDefinition"

// RegisterTaskDefinitionRequest generates a "aws/request.Request" representing the
// client's request for the RegisterTaskDefinition operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See RegisterTaskDefinition for more information on using the RegisterTaskDefinition
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the RegisterTaskDefinitionRequest method.
//    req, resp := client.RegisterTaskDefinitionRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) RegisterTaskDefinitionRequest(input *RegisterTaskDefinitionInput) (req *request.Request, output *RegisterTaskDefinitionOutput) {
	op := &request.Operation{
		Name:       opRegisterTaskDefinition,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &RegisterTaskDefinitionInput{}
	}

	output = &RegisterTaskDefinitionOutput{}
	req = c.newRequest(op, input, output)
	return
}

// RegisterTaskDefinition API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation RegisterTaskDefinition for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) RegisterTaskDefinition(input *RegisterTaskDefinitionInput) (*RegisterTaskDefinitionOutput, error) {
	req, out := c.RegisterTaskDefinitionRequest(input)
	return out, req.Send()
}

// RegisterTaskDefinitionWithContext is the same as RegisterTaskDefinition with the addition of
// the ability to pass a context and additional request options.
//
// See RegisterTaskDefinition for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) RegisterTaskDefinitionWithContext(ctx aws.Context, input *RegisterTaskDefinitionInput, opts ...request.Option) (*RegisterTaskDefinitionOutput, error) {
	req, out := c.RegisterTaskDefinitionRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opRunTask = "RunTask"

// RunTaskRequest generates a "aws/request.Request" representing the
// client's request for the RunTask operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See RunTask for more information on using the RunTask
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the RunTaskRequest method.
//    req, resp := client.RunTaskRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) RunTaskRequest(input *RunTaskInput) (req *request.Request, output *RunTaskOutput) {
	op := &request.Operation{
		Name:       opRunTask,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &RunTaskInput{}
	}

	output = &RunTaskOutput{}
	req = c.newRequest(op, input, output)
	return
}

// RunTask API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation RunTask for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeUnsupportedFeatureException "UnsupportedFeatureException"
//
//   * ErrCodePlatformUnknownException "PlatformUnknownException"
//
//   * ErrCodePlatformTaskDefinitionIncompatibilityException "PlatformTaskDefinitionIncompatibilityException"
//
//   * ErrCodeAccessDeniedException "AccessDeniedException"
//
//   * ErrCodeBlockedException "BlockedException"
//
func (c *ECS) RunTask(input *RunTaskInput) (*RunTaskOutput, error) {
	req, out := c.RunTaskRequest(input)
	return out, req.Send()
}

// RunTaskWithContext is the same as RunTask with the addition of
// the ability to pass a context and additional request options.
//
// See RunTask for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) RunTaskWithContext(ctx aws.Context, input *RunTaskInput, opts ...request.Option) (*RunTaskOutput, error) {
	req, out := c.RunTaskRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opStartTask = "StartTask"

// StartTaskRequest generates a "aws/request.Request" representing the
// client's request for the StartTask operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See StartTask for more information on using the StartTask
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the StartTaskRequest method.
//    req, resp := client.StartTaskRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) StartTaskRequest(input *StartTaskInput) (req *request.Request, output *StartTaskOutput) {
	op := &request.Operation{
		Name:       opStartTask,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &StartTaskInput{}
	}

	output = &StartTaskOutput{}
	req = c.newRequest(op, input, output)
	return
}

// StartTask API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation StartTask for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
func (c *ECS) StartTask(input *StartTaskInput) (*StartTaskOutput, error) {
	req, out := c.StartTaskRequest(input)
	return out, req.Send()
}

// StartTaskWithContext is the same as StartTask with the addition of
// the ability to pass a context and additional request options.
//
// See StartTask for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) StartTaskWithContext(ctx aws.Context, input *StartTaskInput, opts ...request.Option) (*StartTaskOutput, error) {
	req, out := c.StartTaskRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opStopTask = "StopTask"

// StopTaskRequest generates a "aws/request.Request" representing the
// client's request for the StopTask operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See StopTask for more information on using the StopTask
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the StopTaskRequest method.
//    req, resp := client.StopTaskRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) StopTaskRequest(input *StopTaskInput) (req *request.Request, output *StopTaskOutput) {
	op := &request.Operation{
		Name:       opStopTask,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &StopTaskInput{}
	}

	output = &StopTaskOutput{}
	req = c.newRequest(op, input, output)
	return
}

// StopTask API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation StopTask for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
func (c *ECS) StopTask(input *StopTaskInput) (*StopTaskOutput, error) {
	req, out := c.StopTaskRequest(input)
	return out, req.Send()
}

// StopTaskWithContext is the same as StopTask with the addition of
// the ability to pass a context and additional request options.
//
// See StopTask for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) StopTaskWithContext(ctx aws.Context, input *StopTaskInput, opts ...request.Option) (*StopTaskOutput, error) {
	req, out := c.StopTaskRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opSubmitAttachmentStateChanges = "SubmitAttachmentStateChanges"

// SubmitAttachmentStateChangesRequest generates a "aws/request.Request" representing the
// client's request for the SubmitAttachmentStateChanges operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See SubmitAttachmentStateChanges for more information on using the SubmitAttachmentStateChanges
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the SubmitAttachmentStateChangesRequest method.
//    req, resp := client.SubmitAttachmentStateChangesRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) SubmitAttachmentStateChangesRequest(input *SubmitAttachmentStateChangesInput) (req *request.Request, output *SubmitAttachmentStateChangesOutput) {
	op := &request.Operation{
		Name:       opSubmitAttachmentStateChanges,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &SubmitAttachmentStateChangesInput{}
	}

	output = &SubmitAttachmentStateChangesOutput{}
	req = c.newRequest(op, input, output)
	return
}

// SubmitAttachmentStateChanges API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation SubmitAttachmentStateChanges for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeAccessDeniedException "AccessDeniedException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) SubmitAttachmentStateChanges(input *SubmitAttachmentStateChangesInput) (*SubmitAttachmentStateChangesOutput, error) {
	req, out := c.SubmitAttachmentStateChangesRequest(input)
	return out, req.Send()
}

// SubmitAttachmentStateChangesWithContext is the same as SubmitAttachmentStateChanges with the addition of
// the ability to pass a context and additional request options.
//
// See SubmitAttachmentStateChanges for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) SubmitAttachmentStateChangesWithContext(ctx aws.Context, input *SubmitAttachmentStateChangesInput, opts ...request.Option) (*SubmitAttachmentStateChangesOutput, error) {
	req, out := c.SubmitAttachmentStateChangesRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opSubmitContainerStateChange = "SubmitContainerStateChange"

// SubmitContainerStateChangeRequest generates a "aws/request.Request" representing the
// client's request for the SubmitContainerStateChange operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See SubmitContainerStateChange for more information on using the SubmitContainerStateChange
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the SubmitContainerStateChangeRequest method.
//    req, resp := client.SubmitContainerStateChangeRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) SubmitContainerStateChangeRequest(input *SubmitContainerStateChangeInput) (req *request.Request, output *SubmitContainerStateChangeOutput) {
	op := &request.Operation{
		Name:       opSubmitContainerStateChange,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &SubmitContainerStateChangeInput{}
	}

	output = &SubmitContainerStateChangeOutput{}
	req = c.newRequest(op, input, output)
	return
}

// SubmitContainerStateChange API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation SubmitContainerStateChange for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeAccessDeniedException "AccessDeniedException"
//
func (c *ECS) SubmitContainerStateChange(input *SubmitContainerStateChangeInput) (*SubmitContainerStateChangeOutput, error) {
	req, out := c.SubmitContainerStateChangeRequest(input)
	return out, req.Send()
}

// SubmitContainerStateChangeWithContext is the same as SubmitContainerStateChange with the addition of
// the ability to pass a context and additional request options.
//
// See SubmitContainerStateChange for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) SubmitContainerStateChangeWithContext(ctx aws.Context, input *SubmitContainerStateChangeInput, opts ...request.Option) (*SubmitContainerStateChangeOutput, error) {
	req, out := c.SubmitContainerStateChangeRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opSubmitTaskStateChange = "SubmitTaskStateChange"

// SubmitTaskStateChangeRequest generates a "aws/request.Request" representing the
// client's request for the SubmitTaskStateChange operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See SubmitTaskStateChange for more information on using the SubmitTaskStateChange
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the SubmitTaskStateChangeRequest method.
//    req, resp := client.SubmitTaskStateChangeRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) SubmitTaskStateChangeRequest(input *SubmitTaskStateChangeInput) (req *request.Request, output *SubmitTaskStateChangeOutput) {
	op := &request.Operation{
		Name:       opSubmitTaskStateChange,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &SubmitTaskStateChangeInput{}
	}

	output = &SubmitTaskStateChangeOutput{}
	req = c.newRequest(op, input, output)
	return
}

// SubmitTaskStateChange API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation SubmitTaskStateChange for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeAccessDeniedException "AccessDeniedException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) SubmitTaskStateChange(input *SubmitTaskStateChangeInput) (*SubmitTaskStateChangeOutput, error) {
	req, out := c.SubmitTaskStateChangeRequest(input)
	return out, req.Send()
}

// SubmitTaskStateChangeWithContext is the same as SubmitTaskStateChange with the addition of
// the ability to pass a context and additional request options.
//
// See SubmitTaskStateChange for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) SubmitTaskStateChangeWithContext(ctx aws.Context, input *SubmitTaskStateChangeInput, opts ...request.Option) (*SubmitTaskStateChangeOutput, error) {
	req, out := c.SubmitTaskStateChangeRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opTagResource = "TagResource"

// TagResourceRequest generates a "aws/request.Request" representing the
// client's request for the TagResource operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See TagResource for more information on using the TagResource
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the TagResourceRequest method.
//    req, resp := client.TagResourceRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) TagResourceRequest(input *TagResourceInput) (req *request.Request, output *TagResourceOutput) {
	op := &request.Operation{
		Name:       opTagResource,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &TagResourceInput{}
	}

	output = &TagResourceOutput{}
	req = c.newRequest(op, input, output)
	return
}

// TagResource API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation TagResource for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeResourceNotFoundException "ResourceNotFoundException"
//
//   * ErrCodeLimitExceededException "LimitExceededException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) TagResource(input *TagResourceInput) (*TagResourceOutput, error) {
	req, out := c.TagResourceRequest(input)
	return out, req.Send()
}

// TagResourceWithContext is the same as TagResource with the addition of
// the ability to pass a context and additional request options.
//
// See TagResource for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) TagResourceWithContext(ctx aws.Context, input *TagResourceInput, opts ...request.Option) (*TagResourceOutput, error) {
	req, out := c.TagResourceRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opUntagResource = "UntagResource"

// UntagResourceRequest generates a "aws/request.Request" representing the
// client's request for the UntagResource operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See UntagResource for more information on using the UntagResource
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the UntagResourceRequest method.
//    req, resp := client.UntagResourceRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) UntagResourceRequest(input *UntagResourceInput) (req *request.Request, output *UntagResourceOutput) {
	op := &request.Operation{
		Name:       opUntagResource,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &UntagResourceInput{}
	}

	output = &UntagResourceOutput{}
	req = c.newRequest(op, input, output)
	return
}

// UntagResource API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation UntagResource for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeResourceNotFoundException "ResourceNotFoundException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
func (c *ECS) UntagResource(input *UntagResourceInput) (*UntagResourceOutput, error) {
	req, out := c.UntagResourceRequest(input)
	return out, req.Send()
}

// UntagResourceWithContext is the same as UntagResource with the addition of
// the ability to pass a context and additional request options.
//
// See UntagResource for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) UntagResourceWithContext(ctx aws.Context, input *UntagResourceInput, opts ...request.Option) (*UntagResourceOutput, error) {
	req, out := c.UntagResourceRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opUpdateContainerAgent = "UpdateContainerAgent"

// UpdateContainerAgentRequest generates a "aws/request.Request" representing the
// client's request for the UpdateContainerAgent operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See UpdateContainerAgent for more information on using the UpdateContainerAgent
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the UpdateContainerAgentRequest method.
//    req, resp := client.UpdateContainerAgentRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) UpdateContainerAgentRequest(input *UpdateContainerAgentInput) (req *request.Request, output *UpdateContainerAgentOutput) {
	op := &request.Operation{
		Name:       opUpdateContainerAgent,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &UpdateContainerAgentInput{}
	}

	output = &UpdateContainerAgentOutput{}
	req = c.newRequest(op, input, output)
	return
}

// UpdateContainerAgent API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation UpdateContainerAgent for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeUpdateInProgressException "UpdateInProgressException"
//
//   * ErrCodeNoUpdateAvailableException "NoUpdateAvailableException"
//
//   * ErrCodeMissingVersionException "MissingVersionException"
//
func (c *ECS) UpdateContainerAgent(input *UpdateContainerAgentInput) (*UpdateContainerAgentOutput, error) {
	req, out := c.UpdateContainerAgentRequest(input)
	return out, req.Send()
}

// UpdateContainerAgentWithContext is the same as UpdateContainerAgent with the addition of
// the ability to pass a context and additional request options.
//
// See UpdateContainerAgent for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) UpdateContainerAgentWithContext(ctx aws.Context, input *UpdateContainerAgentInput, opts ...request.Option) (*UpdateContainerAgentOutput, error) {
	req, out := c.UpdateContainerAgentRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opUpdateContainerInstancesState = "UpdateContainerInstancesState"

// UpdateContainerInstancesStateRequest generates a "aws/request.Request" representing the
// client's request for the UpdateContainerInstancesState operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See UpdateContainerInstancesState for more information on using the UpdateContainerInstancesState
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the UpdateContainerInstancesStateRequest method.
//    req, resp := client.UpdateContainerInstancesStateRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) UpdateContainerInstancesStateRequest(input *UpdateContainerInstancesStateInput) (req *request.Request, output *UpdateContainerInstancesStateOutput) {
	op := &request.Operation{
		Name:       opUpdateContainerInstancesState,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &UpdateContainerInstancesStateInput{}
	}

	output = &UpdateContainerInstancesStateOutput{}
	req = c.newRequest(op, input, output)
	return
}

// UpdateContainerInstancesState API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation UpdateContainerInstancesState for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
func (c *ECS) UpdateContainerInstancesState(input *UpdateContainerInstancesStateInput) (*UpdateContainerInstancesStateOutput, error) {
	req, out := c.UpdateContainerInstancesStateRequest(input)
	return out, req.Send()
}

// UpdateContainerInstancesStateWithContext is the same as UpdateContainerInstancesState with the addition of
// the ability to pass a context and additional request options.
//
// See UpdateContainerInstancesState for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) UpdateContainerInstancesStateWithContext(ctx aws.Context, input *UpdateContainerInstancesStateInput, opts ...request.Option) (*UpdateContainerInstancesStateOutput, error) {
	req, out := c.UpdateContainerInstancesStateRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opUpdateService = "UpdateService"

// UpdateServiceRequest generates a "aws/request.Request" representing the
// client's request for the UpdateService operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See UpdateService for more information on using the UpdateService
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the UpdateServiceRequest method.
//    req, resp := client.UpdateServiceRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) UpdateServiceRequest(input *UpdateServiceInput) (req *request.Request, output *UpdateServiceOutput) {
	op := &request.Operation{
		Name:       opUpdateService,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &UpdateServiceInput{}
	}

	output = &UpdateServiceOutput{}
	req = c.newRequest(op, input, output)
	return
}

// UpdateService API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation UpdateService for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeServiceNotFoundException "ServiceNotFoundException"
//
//   * ErrCodeServiceNotActiveException "ServiceNotActiveException"
//
//   * ErrCodePlatformUnknownException "PlatformUnknownException"
//
//   * ErrCodePlatformTaskDefinitionIncompatibilityException "PlatformTaskDefinitionIncompatibilityException"
//
//   * ErrCodeAccessDeniedException "AccessDeniedException"
//
func (c *ECS) UpdateService(input *UpdateServiceInput) (*UpdateServiceOutput, error) {
	req, out := c.UpdateServiceRequest(input)
	return out, req.Send()
}

// UpdateServiceWithContext is the same as UpdateService with the addition of
// the ability to pass a context and additional request options.
//
// See UpdateService for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) UpdateServiceWithContext(ctx aws.Context, input *UpdateServiceInput, opts ...request.Option) (*UpdateServiceOutput, error) {
	req, out := c.UpdateServiceRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opUpdateServicePrimaryTaskSet = "UpdateServicePrimaryTaskSet"

// UpdateServicePrimaryTaskSetRequest generates a "aws/request.Request" representing the
// client's request for the UpdateServicePrimaryTaskSet operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See UpdateServicePrimaryTaskSet for more information on using the UpdateServicePrimaryTaskSet
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the UpdateServicePrimaryTaskSetRequest method.
//    req, resp := client.UpdateServicePrimaryTaskSetRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) UpdateServicePrimaryTaskSetRequest(input *UpdateServicePrimaryTaskSetInput) (req *request.Request, output *UpdateServicePrimaryTaskSetOutput) {
	op := &request.Operation{
		Name:       opUpdateServicePrimaryTaskSet,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &UpdateServicePrimaryTaskSetInput{}
	}

	output = &UpdateServicePrimaryTaskSetOutput{}
	req = c.newRequest(op, input, output)
	return
}

// UpdateServicePrimaryTaskSet API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation UpdateServicePrimaryTaskSet for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeServiceNotFoundException "ServiceNotFoundException"
//
//   * ErrCodeServiceNotActiveException "ServiceNotActiveException"
//
//   * ErrCodeTaskSetNotFoundException "TaskSetNotFoundException"
//
//   * ErrCodeAccessDeniedException "AccessDeniedException"
//
func (c *ECS) UpdateServicePrimaryTaskSet(input *UpdateServicePrimaryTaskSetInput) (*UpdateServicePrimaryTaskSetOutput, error) {
	req, out := c.UpdateServicePrimaryTaskSetRequest(input)
	return out, req.Send()
}

// UpdateServicePrimaryTaskSetWithContext is the same as UpdateServicePrimaryTaskSet with the addition of
// the ability to pass a context and additional request options.
//
// See UpdateServicePrimaryTaskSet for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) UpdateServicePrimaryTaskSetWithContext(ctx aws.Context, input *UpdateServicePrimaryTaskSetInput, opts ...request.Option) (*UpdateServicePrimaryTaskSetOutput, error) {
	req, out := c.UpdateServicePrimaryTaskSetRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opUpdateTaskSet = "UpdateTaskSet"

// UpdateTaskSetRequest generates a "aws/request.Request" representing the
// client's request for the UpdateTaskSet operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See UpdateTaskSet for more information on using the UpdateTaskSet
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the UpdateTaskSetRequest method.
//    req, resp := client.UpdateTaskSetRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *ECS) UpdateTaskSetRequest(input *UpdateTaskSetInput) (req *request.Request, output *UpdateTaskSetOutput) {
	op := &request.Operation{
		Name:       opUpdateTaskSet,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &UpdateTaskSetInput{}
	}

	output = &UpdateTaskSetOutput{}
	req = c.newRequest(op, input, output)
	return
}

// UpdateTaskSet API operation for Amazon Elastic Container Service.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Elastic Container Service's
// API operation UpdateTaskSet for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeServerException "ServerException"
//
//   * ErrCodeClientException "ClientException"
//
//   * ErrCodeInvalidParameterException "InvalidParameterException"
//
//   * ErrCodeClusterNotFoundException "ClusterNotFoundException"
//
//   * ErrCodeUnsupportedFeatureException "UnsupportedFeatureException"
//
//   * ErrCodeAccessDeniedException "AccessDeniedException"
//
//   * ErrCodeLimitExceededException "LimitExceededException"
//
//   * ErrCodeServiceNotFoundException "ServiceNotFoundException"
//
//   * ErrCodeServiceNotActiveException "ServiceNotActiveException"
//
//   * ErrCodeTaskSetNotFoundException "TaskSetNotFoundException"
//
func (c *ECS) UpdateTaskSet(input *UpdateTaskSetInput) (*UpdateTaskSetOutput, error) {
	req, out := c.UpdateTaskSetRequest(input)
	return out, req.Send()
}

// UpdateTaskSetWithContext is the same as UpdateTaskSet with the addition of
// the ability to pass a context and additional request options.
//
// See UpdateTaskSet for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *ECS) UpdateTaskSetWithContext(ctx aws.Context, input *UpdateTaskSetInput, opts ...request.Option) (*UpdateTaskSetOutput, error) {
	req, out := c.UpdateTaskSetRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

type Attachment struct {
	_ struct{} `type:"structure"`

	Details []*KeyValuePair `locationName:"details" type:"list"`

	Id *string `locationName:"id" type:"string"`

	Status *string `locationName:"status" type:"string"`

	Type *string `locationName:"type" type:"string"`
}

// String returns the string representation
func (s Attachment) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Attachment) GoString() string {
	return s.String()
}

// SetDetails sets the Details field's value.
func (s *Attachment) SetDetails(v []*KeyValuePair) *Attachment {
	s.Details = v
	return s
}

// SetId sets the Id field's value.
func (s *Attachment) SetId(v string) *Attachment {
	s.Id = &v
	return s
}

// SetStatus sets the Status field's value.
func (s *Attachment) SetStatus(v string) *Attachment {
	s.Status = &v
	return s
}

// SetType sets the Type field's value.
func (s *Attachment) SetType(v string) *Attachment {
	s.Type = &v
	return s
}

type AttachmentStateChange struct {
	_ struct{} `type:"structure"`

	// AttachmentArn is a required field
	AttachmentArn *string `locationName:"attachmentArn" type:"string" required:"true"`

	// Status is a required field
	Status *string `locationName:"status" type:"string" required:"true"`
}

// String returns the string representation
func (s AttachmentStateChange) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s AttachmentStateChange) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *AttachmentStateChange) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "AttachmentStateChange"}
	if s.AttachmentArn == nil {
		invalidParams.Add(request.NewErrParamRequired("AttachmentArn"))
	}
	if s.Status == nil {
		invalidParams.Add(request.NewErrParamRequired("Status"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetAttachmentArn sets the AttachmentArn field's value.
func (s *AttachmentStateChange) SetAttachmentArn(v string) *AttachmentStateChange {
	s.AttachmentArn = &v
	return s
}

// SetStatus sets the Status field's value.
func (s *AttachmentStateChange) SetStatus(v string) *AttachmentStateChange {
	s.Status = &v
	return s
}

type Attribute struct {
	_ struct{} `type:"structure"`

	// Name is a required field
	Name *string `locationName:"name" type:"string" required:"true"`

	TargetId *string `locationName:"targetId" type:"string"`

	TargetType *string `locationName:"targetType" type:"string" enum:"TargetType"`

	Value *string `locationName:"value" type:"string"`
}

// String returns the string representation
func (s Attribute) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Attribute) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *Attribute) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "Attribute"}
	if s.Name == nil {
		invalidParams.Add(request.NewErrParamRequired("Name"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetName sets the Name field's value.
func (s *Attribute) SetName(v string) *Attribute {
	s.Name = &v
	return s
}

// SetTargetId sets the TargetId field's value.
func (s *Attribute) SetTargetId(v string) *Attribute {
	s.TargetId = &v
	return s
}

// SetTargetType sets the TargetType field's value.
func (s *Attribute) SetTargetType(v string) *Attribute {
	s.TargetType = &v
	return s
}

// SetValue sets the Value field's value.
func (s *Attribute) SetValue(v string) *Attribute {
	s.Value = &v
	return s
}

type AwsVpcConfiguration struct {
	_ struct{} `type:"structure"`

	AssignPublicIp *string `locationName:"assignPublicIp" type:"string" enum:"AssignPublicIp"`

	NetworkInterfaceCredential *string `locationName:"networkInterfaceCredential" type:"string"`

	NetworkInterfaceOwner *string `locationName:"networkInterfaceOwner" type:"string"`

	SecurityGroups []*string `locationName:"securityGroups" type:"list"`

	// Subnets is a required field
	Subnets []*string `locationName:"subnets" type:"list" required:"true"`
}

// String returns the string representation
func (s AwsVpcConfiguration) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s AwsVpcConfiguration) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *AwsVpcConfiguration) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "AwsVpcConfiguration"}
	if s.Subnets == nil {
		invalidParams.Add(request.NewErrParamRequired("Subnets"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetAssignPublicIp sets the AssignPublicIp field's value.
func (s *AwsVpcConfiguration) SetAssignPublicIp(v string) *AwsVpcConfiguration {
	s.AssignPublicIp = &v
	return s
}

// SetNetworkInterfaceCredential sets the NetworkInterfaceCredential field's value.
func (s *AwsVpcConfiguration) SetNetworkInterfaceCredential(v string) *AwsVpcConfiguration {
	s.NetworkInterfaceCredential = &v
	return s
}

// SetNetworkInterfaceOwner sets the NetworkInterfaceOwner field's value.
func (s *AwsVpcConfiguration) SetNetworkInterfaceOwner(v string) *AwsVpcConfiguration {
	s.NetworkInterfaceOwner = &v
	return s
}

// SetSecurityGroups sets the SecurityGroups field's value.
func (s *AwsVpcConfiguration) SetSecurityGroups(v []*string) *AwsVpcConfiguration {
	s.SecurityGroups = v
	return s
}

// SetSubnets sets the Subnets field's value.
func (s *AwsVpcConfiguration) SetSubnets(v []*string) *AwsVpcConfiguration {
	s.Subnets = v
	return s
}

type Cluster struct {
	_ struct{} `type:"structure"`

	ActiveServicesCount *int64 `locationName:"activeServicesCount" type:"integer"`

	ClusterArn *string `locationName:"clusterArn" type:"string"`

	ClusterName *string `locationName:"clusterName" type:"string"`

	PendingTasksCount *int64 `locationName:"pendingTasksCount" type:"integer"`

	RegisteredContainerInstancesCount *int64 `locationName:"registeredContainerInstancesCount" type:"integer"`

	RunningTasksCount *int64 `locationName:"runningTasksCount" type:"integer"`

	Statistics []*KeyValuePair `locationName:"statistics" type:"list"`

	Status *string `locationName:"status" type:"string"`

	Tags []*Tag `locationName:"tags" type:"list"`
}

// String returns the string representation
func (s Cluster) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Cluster) GoString() string {
	return s.String()
}

// SetActiveServicesCount sets the ActiveServicesCount field's value.
func (s *Cluster) SetActiveServicesCount(v int64) *Cluster {
	s.ActiveServicesCount = &v
	return s
}

// SetClusterArn sets the ClusterArn field's value.
func (s *Cluster) SetClusterArn(v string) *Cluster {
	s.ClusterArn = &v
	return s
}

// SetClusterName sets the ClusterName field's value.
func (s *Cluster) SetClusterName(v string) *Cluster {
	s.ClusterName = &v
	return s
}

// SetPendingTasksCount sets the PendingTasksCount field's value.
func (s *Cluster) SetPendingTasksCount(v int64) *Cluster {
	s.PendingTasksCount = &v
	return s
}

// SetRegisteredContainerInstancesCount sets the RegisteredContainerInstancesCount field's value.
func (s *Cluster) SetRegisteredContainerInstancesCount(v int64) *Cluster {
	s.RegisteredContainerInstancesCount = &v
	return s
}

// SetRunningTasksCount sets the RunningTasksCount field's value.
func (s *Cluster) SetRunningTasksCount(v int64) *Cluster {
	s.RunningTasksCount = &v
	return s
}

// SetStatistics sets the Statistics field's value.
func (s *Cluster) SetStatistics(v []*KeyValuePair) *Cluster {
	s.Statistics = v
	return s
}

// SetStatus sets the Status field's value.
func (s *Cluster) SetStatus(v string) *Cluster {
	s.Status = &v
	return s
}

// SetTags sets the Tags field's value.
func (s *Cluster) SetTags(v []*Tag) *Cluster {
	s.Tags = v
	return s
}

type Container struct {
	_ struct{} `type:"structure"`

	ContainerArn *string `locationName:"containerArn" type:"string"`

	Cpu *string `locationName:"cpu" type:"string"`

	ExitCode *int64 `locationName:"exitCode" type:"integer"`

	GpuIds []*string `locationName:"gpuIds" type:"list"`

	HealthStatus *string `locationName:"healthStatus" type:"string" enum:"HealthStatus"`

	LastStatus *string `locationName:"lastStatus" type:"string"`

	Memory *string `locationName:"memory" type:"string"`

	MemoryReservation *string `locationName:"memoryReservation" type:"string"`

	Name *string `locationName:"name" type:"string"`

	NetworkBindings []*NetworkBinding `locationName:"networkBindings" type:"list"`

	NetworkInterfaces []*NetworkInterface `locationName:"networkInterfaces" type:"list"`

	Reason *string `locationName:"reason" type:"string"`

	TaskArn *string `locationName:"taskArn" type:"string"`
}

// String returns the string representation
func (s Container) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Container) GoString() string {
	return s.String()
}

// SetContainerArn sets the ContainerArn field's value.
func (s *Container) SetContainerArn(v string) *Container {
	s.ContainerArn = &v
	return s
}

// SetCpu sets the Cpu field's value.
func (s *Container) SetCpu(v string) *Container {
	s.Cpu = &v
	return s
}

// SetExitCode sets the ExitCode field's value.
func (s *Container) SetExitCode(v int64) *Container {
	s.ExitCode = &v
	return s
}

// SetGpuIds sets the GpuIds field's value.
func (s *Container) SetGpuIds(v []*string) *Container {
	s.GpuIds = v
	return s
}

// SetHealthStatus sets the HealthStatus field's value.
func (s *Container) SetHealthStatus(v string) *Container {
	s.HealthStatus = &v
	return s
}

// SetLastStatus sets the LastStatus field's value.
func (s *Container) SetLastStatus(v string) *Container {
	s.LastStatus = &v
	return s
}

// SetMemory sets the Memory field's value.
func (s *Container) SetMemory(v string) *Container {
	s.Memory = &v
	return s
}

// SetMemoryReservation sets the MemoryReservation field's value.
func (s *Container) SetMemoryReservation(v string) *Container {
	s.MemoryReservation = &v
	return s
}

// SetName sets the Name field's value.
func (s *Container) SetName(v string) *Container {
	s.Name = &v
	return s
}

// SetNetworkBindings sets the NetworkBindings field's value.
func (s *Container) SetNetworkBindings(v []*NetworkBinding) *Container {
	s.NetworkBindings = v
	return s
}

// SetNetworkInterfaces sets the NetworkInterfaces field's value.
func (s *Container) SetNetworkInterfaces(v []*NetworkInterface) *Container {
	s.NetworkInterfaces = v
	return s
}

// SetReason sets the Reason field's value.
func (s *Container) SetReason(v string) *Container {
	s.Reason = &v
	return s
}

// SetTaskArn sets the TaskArn field's value.
func (s *Container) SetTaskArn(v string) *Container {
	s.TaskArn = &v
	return s
}

type ContainerDefinition struct {
	_ struct{} `type:"structure"`

	Command []*string `locationName:"command" type:"list"`

	Cpu *int64 `locationName:"cpu" type:"integer"`

	DependsOn []*ContainerDependency `locationName:"dependsOn" type:"list"`

	DisableNetworking *bool `locationName:"disableNetworking" type:"boolean"`

	DnsSearchDomains []*string `locationName:"dnsSearchDomains" type:"list"`

	DnsServers []*string `locationName:"dnsServers" type:"list"`

	DockerLabels map[string]*string `locationName:"dockerLabels" type:"map"`

	DockerSecurityOptions []*string `locationName:"dockerSecurityOptions" type:"list"`

	EntryPoint []*string `locationName:"entryPoint" type:"list"`

	Environment []*KeyValuePair `locationName:"environment" type:"list"`

	Essential *bool `locationName:"essential" type:"boolean"`

	ExtraHosts []*HostEntry `locationName:"extraHosts" type:"list"`

	HealthCheck *HealthCheck `locationName:"healthCheck" type:"structure"`

	Hostname *string `locationName:"hostname" type:"string"`

	Image *string `locationName:"image" type:"string"`

	InferenceDevices []*string `locationName:"inferenceDevices" type:"list"`

	Interactive *bool `locationName:"interactive" type:"boolean"`

	Links []*string `locationName:"links" type:"list"`

	LinuxParameters *LinuxParameters `locationName:"linuxParameters" type:"structure"`

	LogConfiguration *LogConfiguration `locationName:"logConfiguration" type:"structure"`

	Memory *int64 `locationName:"memory" type:"integer"`

	MemoryReservation *int64 `locationName:"memoryReservation" type:"integer"`

	MountPoints []*MountPoint `locationName:"mountPoints" type:"list"`

	Name *string `locationName:"name" type:"string"`

	PortMappings []*PortMapping `locationName:"portMappings" type:"list"`

	Privileged *bool `locationName:"privileged" type:"boolean"`

	PseudoTerminal *bool `locationName:"pseudoTerminal" type:"boolean"`

	ReadonlyRootFilesystem *bool `locationName:"readonlyRootFilesystem" type:"boolean"`

	RepositoryCredentials *RepositoryCredentials `locationName:"repositoryCredentials" type:"structure"`

	ResourceRequirements []*ResourceRequirement `locationName:"resourceRequirements" type:"list"`

	Secrets []*Secret `locationName:"secrets" type:"list"`

	StartTimeout *int64 `locationName:"startTimeout" type:"integer"`

	StopTimeout *int64 `locationName:"stopTimeout" type:"integer"`

	SystemControls []*SystemControl `locationName:"systemControls" type:"list"`

	Ulimits []*Ulimit `locationName:"ulimits" type:"list"`

	User *string `locationName:"user" type:"string"`

	VolumesFrom []*VolumeFrom `locationName:"volumesFrom" type:"list"`

	WorkingDirectory *string `locationName:"workingDirectory" type:"string"`
}

// String returns the string representation
func (s ContainerDefinition) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ContainerDefinition) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *ContainerDefinition) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "ContainerDefinition"}
	if s.DependsOn != nil {
		for i, v := range s.DependsOn {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "DependsOn", i), err.(request.ErrInvalidParams))
			}
		}
	}
	if s.ExtraHosts != nil {
		for i, v := range s.ExtraHosts {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "ExtraHosts", i), err.(request.ErrInvalidParams))
			}
		}
	}
	if s.HealthCheck != nil {
		if err := s.HealthCheck.Validate(); err != nil {
			invalidParams.AddNested("HealthCheck", err.(request.ErrInvalidParams))
		}
	}
	if s.LinuxParameters != nil {
		if err := s.LinuxParameters.Validate(); err != nil {
			invalidParams.AddNested("LinuxParameters", err.(request.ErrInvalidParams))
		}
	}
	if s.LogConfiguration != nil {
		if err := s.LogConfiguration.Validate(); err != nil {
			invalidParams.AddNested("LogConfiguration", err.(request.ErrInvalidParams))
		}
	}
	if s.RepositoryCredentials != nil {
		if err := s.RepositoryCredentials.Validate(); err != nil {
			invalidParams.AddNested("RepositoryCredentials", err.(request.ErrInvalidParams))
		}
	}
	if s.ResourceRequirements != nil {
		for i, v := range s.ResourceRequirements {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "ResourceRequirements", i), err.(request.ErrInvalidParams))
			}
		}
	}
	if s.Secrets != nil {
		for i, v := range s.Secrets {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Secrets", i), err.(request.ErrInvalidParams))
			}
		}
	}
	if s.Ulimits != nil {
		for i, v := range s.Ulimits {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Ulimits", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCommand sets the Command field's value.
func (s *ContainerDefinition) SetCommand(v []*string) *ContainerDefinition {
	s.Command = v
	return s
}

// SetCpu sets the Cpu field's value.
func (s *ContainerDefinition) SetCpu(v int64) *ContainerDefinition {
	s.Cpu = &v
	return s
}

// SetDependsOn sets the DependsOn field's value.
func (s *ContainerDefinition) SetDependsOn(v []*ContainerDependency) *ContainerDefinition {
	s.DependsOn = v
	return s
}

// SetDisableNetworking sets the DisableNetworking field's value.
func (s *ContainerDefinition) SetDisableNetworking(v bool) *ContainerDefinition {
	s.DisableNetworking = &v
	return s
}

// SetDnsSearchDomains sets the DnsSearchDomains field's value.
func (s *ContainerDefinition) SetDnsSearchDomains(v []*string) *ContainerDefinition {
	s.DnsSearchDomains = v
	return s
}

// SetDnsServers sets the DnsServers field's value.
func (s *ContainerDefinition) SetDnsServers(v []*string) *ContainerDefinition {
	s.DnsServers = v
	return s
}

// SetDockerLabels sets the DockerLabels field's value.
func (s *ContainerDefinition) SetDockerLabels(v map[string]*string) *ContainerDefinition {
	s.DockerLabels = v
	return s
}

// SetDockerSecurityOptions sets the DockerSecurityOptions field's value.
func (s *ContainerDefinition) SetDockerSecurityOptions(v []*string) *ContainerDefinition {
	s.DockerSecurityOptions = v
	return s
}

// SetEntryPoint sets the EntryPoint field's value.
func (s *ContainerDefinition) SetEntryPoint(v []*string) *ContainerDefinition {
	s.EntryPoint = v
	return s
}

// SetEnvironment sets the Environment field's value.
func (s *ContainerDefinition) SetEnvironment(v []*KeyValuePair) *ContainerDefinition {
	s.Environment = v
	return s
}

// SetEssential sets the Essential field's value.
func (s *ContainerDefinition) SetEssential(v bool) *ContainerDefinition {
	s.Essential = &v
	return s
}

// SetExtraHosts sets the ExtraHosts field's value.
func (s *ContainerDefinition) SetExtraHosts(v []*HostEntry) *ContainerDefinition {
	s.ExtraHosts = v
	return s
}

// SetHealthCheck sets the HealthCheck field's value.
func (s *ContainerDefinition) SetHealthCheck(v *HealthCheck) *ContainerDefinition {
	s.HealthCheck = v
	return s
}

// SetHostname sets the Hostname field's value.
func (s *ContainerDefinition) SetHostname(v string) *ContainerDefinition {
	s.Hostname = &v
	return s
}

// SetImage sets the Image field's value.
func (s *ContainerDefinition) SetImage(v string) *ContainerDefinition {
	s.Image = &v
	return s
}

// SetInferenceDevices sets the InferenceDevices field's value.
func (s *ContainerDefinition) SetInferenceDevices(v []*string) *ContainerDefinition {
	s.InferenceDevices = v
	return s
}

// SetInteractive sets the Interactive field's value.
func (s *ContainerDefinition) SetInteractive(v bool) *ContainerDefinition {
	s.Interactive = &v
	return s
}

// SetLinks sets the Links field's value.
func (s *ContainerDefinition) SetLinks(v []*string) *ContainerDefinition {
	s.Links = v
	return s
}

// SetLinuxParameters sets the LinuxParameters field's value.
func (s *ContainerDefinition) SetLinuxParameters(v *LinuxParameters) *ContainerDefinition {
	s.LinuxParameters = v
	return s
}

// SetLogConfiguration sets the LogConfiguration field's value.
func (s *ContainerDefinition) SetLogConfiguration(v *LogConfiguration) *ContainerDefinition {
	s.LogConfiguration = v
	return s
}

// SetMemory sets the Memory field's value.
func (s *ContainerDefinition) SetMemory(v int64) *ContainerDefinition {
	s.Memory = &v
	return s
}

// SetMemoryReservation sets the MemoryReservation field's value.
func (s *ContainerDefinition) SetMemoryReservation(v int64) *ContainerDefinition {
	s.MemoryReservation = &v
	return s
}

// SetMountPoints sets the MountPoints field's value.
func (s *ContainerDefinition) SetMountPoints(v []*MountPoint) *ContainerDefinition {
	s.MountPoints = v
	return s
}

// SetName sets the Name field's value.
func (s *ContainerDefinition) SetName(v string) *ContainerDefinition {
	s.Name = &v
	return s
}

// SetPortMappings sets the PortMappings field's value.
func (s *ContainerDefinition) SetPortMappings(v []*PortMapping) *ContainerDefinition {
	s.PortMappings = v
	return s
}

// SetPrivileged sets the Privileged field's value.
func (s *ContainerDefinition) SetPrivileged(v bool) *ContainerDefinition {
	s.Privileged = &v
	return s
}

// SetPseudoTerminal sets the PseudoTerminal field's value.
func (s *ContainerDefinition) SetPseudoTerminal(v bool) *ContainerDefinition {
	s.PseudoTerminal = &v
	return s
}

// SetReadonlyRootFilesystem sets the ReadonlyRootFilesystem field's value.
func (s *ContainerDefinition) SetReadonlyRootFilesystem(v bool) *ContainerDefinition {
	s.ReadonlyRootFilesystem = &v
	return s
}

// SetRepositoryCredentials sets the RepositoryCredentials field's value.
func (s *ContainerDefinition) SetRepositoryCredentials(v *RepositoryCredentials) *ContainerDefinition {
	s.RepositoryCredentials = v
	return s
}

// SetResourceRequirements sets the ResourceRequirements field's value.
func (s *ContainerDefinition) SetResourceRequirements(v []*ResourceRequirement) *ContainerDefinition {
	s.ResourceRequirements = v
	return s
}

// SetSecrets sets the Secrets field's value.
func (s *ContainerDefinition) SetSecrets(v []*Secret) *ContainerDefinition {
	s.Secrets = v
	return s
}

// SetStartTimeout sets the StartTimeout field's value.
func (s *ContainerDefinition) SetStartTimeout(v int64) *ContainerDefinition {
	s.StartTimeout = &v
	return s
}

// SetStopTimeout sets the StopTimeout field's value.
func (s *ContainerDefinition) SetStopTimeout(v int64) *ContainerDefinition {
	s.StopTimeout = &v
	return s
}

// SetSystemControls sets the SystemControls field's value.
func (s *ContainerDefinition) SetSystemControls(v []*SystemControl) *ContainerDefinition {
	s.SystemControls = v
	return s
}

// SetUlimits sets the Ulimits field's value.
func (s *ContainerDefinition) SetUlimits(v []*Ulimit) *ContainerDefinition {
	s.Ulimits = v
	return s
}

// SetUser sets the User field's value.
func (s *ContainerDefinition) SetUser(v string) *ContainerDefinition {
	s.User = &v
	return s
}

// SetVolumesFrom sets the VolumesFrom field's value.
func (s *ContainerDefinition) SetVolumesFrom(v []*VolumeFrom) *ContainerDefinition {
	s.VolumesFrom = v
	return s
}

// SetWorkingDirectory sets the WorkingDirectory field's value.
func (s *ContainerDefinition) SetWorkingDirectory(v string) *ContainerDefinition {
	s.WorkingDirectory = &v
	return s
}

type ContainerDependency struct {
	_ struct{} `type:"structure"`

	// Condition is a required field
	Condition *string `locationName:"condition" type:"string" required:"true" enum:"ContainerCondition"`

	// ContainerName is a required field
	ContainerName *string `locationName:"containerName" type:"string" required:"true"`
}

// String returns the string representation
func (s ContainerDependency) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ContainerDependency) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *ContainerDependency) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "ContainerDependency"}
	if s.Condition == nil {
		invalidParams.Add(request.NewErrParamRequired("Condition"))
	}
	if s.ContainerName == nil {
		invalidParams.Add(request.NewErrParamRequired("ContainerName"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCondition sets the Condition field's value.
func (s *ContainerDependency) SetCondition(v string) *ContainerDependency {
	s.Condition = &v
	return s
}

// SetContainerName sets the ContainerName field's value.
func (s *ContainerDependency) SetContainerName(v string) *ContainerDependency {
	s.ContainerName = &v
	return s
}

type ContainerInstance struct {
	_ struct{} `type:"structure"`

	AgentConnected *bool `locationName:"agentConnected" type:"boolean"`

	AgentUpdateStatus *string `locationName:"agentUpdateStatus" type:"string" enum:"AgentUpdateStatus"`

	Attachments []*Attachment `locationName:"attachments" type:"list"`

	Attributes []*Attribute `locationName:"attributes" type:"list"`

	ClientToken *string `locationName:"clientToken" type:"string"`

	ContainerInstanceArn *string `locationName:"containerInstanceArn" type:"string"`

	Ec2InstanceId *string `locationName:"ec2InstanceId" type:"string"`

	PendingTasksCount *int64 `locationName:"pendingTasksCount" type:"integer"`

	RegisteredAt *time.Time `locationName:"registeredAt" type:"timestamp"`

	RegisteredResources []*Resource `locationName:"registeredResources" type:"list"`

	RemainingResources []*Resource `locationName:"remainingResources" type:"list"`

	RunningTasksCount *int64 `locationName:"runningTasksCount" type:"integer"`

	Status *string `locationName:"status" type:"string"`

	Tags []*Tag `locationName:"tags" type:"list"`

	Version *int64 `locationName:"version" type:"long"`

	VersionInfo *VersionInfo `locationName:"versionInfo" type:"structure"`
}

// String returns the string representation
func (s ContainerInstance) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ContainerInstance) GoString() string {
	return s.String()
}

// SetAgentConnected sets the AgentConnected field's value.
func (s *ContainerInstance) SetAgentConnected(v bool) *ContainerInstance {
	s.AgentConnected = &v
	return s
}

// SetAgentUpdateStatus sets the AgentUpdateStatus field's value.
func (s *ContainerInstance) SetAgentUpdateStatus(v string) *ContainerInstance {
	s.AgentUpdateStatus = &v
	return s
}

// SetAttachments sets the Attachments field's value.
func (s *ContainerInstance) SetAttachments(v []*Attachment) *ContainerInstance {
	s.Attachments = v
	return s
}

// SetAttributes sets the Attributes field's value.
func (s *ContainerInstance) SetAttributes(v []*Attribute) *ContainerInstance {
	s.Attributes = v
	return s
}

// SetClientToken sets the ClientToken field's value.
func (s *ContainerInstance) SetClientToken(v string) *ContainerInstance {
	s.ClientToken = &v
	return s
}

// SetContainerInstanceArn sets the ContainerInstanceArn field's value.
func (s *ContainerInstance) SetContainerInstanceArn(v string) *ContainerInstance {
	s.ContainerInstanceArn = &v
	return s
}

// SetEc2InstanceId sets the Ec2InstanceId field's value.
func (s *ContainerInstance) SetEc2InstanceId(v string) *ContainerInstance {
	s.Ec2InstanceId = &v
	return s
}

// SetPendingTasksCount sets the PendingTasksCount field's value.
func (s *ContainerInstance) SetPendingTasksCount(v int64) *ContainerInstance {
	s.PendingTasksCount = &v
	return s
}

// SetRegisteredAt sets the RegisteredAt field's value.
func (s *ContainerInstance) SetRegisteredAt(v time.Time) *ContainerInstance {
	s.RegisteredAt = &v
	return s
}

// SetRegisteredResources sets the RegisteredResources field's value.
func (s *ContainerInstance) SetRegisteredResources(v []*Resource) *ContainerInstance {
	s.RegisteredResources = v
	return s
}

// SetRemainingResources sets the RemainingResources field's value.
func (s *ContainerInstance) SetRemainingResources(v []*Resource) *ContainerInstance {
	s.RemainingResources = v
	return s
}

// SetRunningTasksCount sets the RunningTasksCount field's value.
func (s *ContainerInstance) SetRunningTasksCount(v int64) *ContainerInstance {
	s.RunningTasksCount = &v
	return s
}

// SetStatus sets the Status field's value.
func (s *ContainerInstance) SetStatus(v string) *ContainerInstance {
	s.Status = &v
	return s
}

// SetTags sets the Tags field's value.
func (s *ContainerInstance) SetTags(v []*Tag) *ContainerInstance {
	s.Tags = v
	return s
}

// SetVersion sets the Version field's value.
func (s *ContainerInstance) SetVersion(v int64) *ContainerInstance {
	s.Version = &v
	return s
}

// SetVersionInfo sets the VersionInfo field's value.
func (s *ContainerInstance) SetVersionInfo(v *VersionInfo) *ContainerInstance {
	s.VersionInfo = v
	return s
}

type ContainerOverride struct {
	_ struct{} `type:"structure"`

	Command []*string `locationName:"command" type:"list"`

	Cpu *int64 `locationName:"cpu" type:"integer"`

	Environment []*KeyValuePair `locationName:"environment" type:"list"`

	Memory *int64 `locationName:"memory" type:"integer"`

	MemoryReservation *int64 `locationName:"memoryReservation" type:"integer"`

	Name *string `locationName:"name" type:"string"`

	ResourceRequirements []*ResourceRequirement `locationName:"resourceRequirements" type:"list"`
}

// String returns the string representation
func (s ContainerOverride) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ContainerOverride) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *ContainerOverride) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "ContainerOverride"}
	if s.ResourceRequirements != nil {
		for i, v := range s.ResourceRequirements {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "ResourceRequirements", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCommand sets the Command field's value.
func (s *ContainerOverride) SetCommand(v []*string) *ContainerOverride {
	s.Command = v
	return s
}

// SetCpu sets the Cpu field's value.
func (s *ContainerOverride) SetCpu(v int64) *ContainerOverride {
	s.Cpu = &v
	return s
}

// SetEnvironment sets the Environment field's value.
func (s *ContainerOverride) SetEnvironment(v []*KeyValuePair) *ContainerOverride {
	s.Environment = v
	return s
}

// SetMemory sets the Memory field's value.
func (s *ContainerOverride) SetMemory(v int64) *ContainerOverride {
	s.Memory = &v
	return s
}

// SetMemoryReservation sets the MemoryReservation field's value.
func (s *ContainerOverride) SetMemoryReservation(v int64) *ContainerOverride {
	s.MemoryReservation = &v
	return s
}

// SetName sets the Name field's value.
func (s *ContainerOverride) SetName(v string) *ContainerOverride {
	s.Name = &v
	return s
}

// SetResourceRequirements sets the ResourceRequirements field's value.
func (s *ContainerOverride) SetResourceRequirements(v []*ResourceRequirement) *ContainerOverride {
	s.ResourceRequirements = v
	return s
}

type ContainerStateChange struct {
	_ struct{} `type:"structure"`

	ContainerName *string `locationName:"containerName" type:"string"`

	ExitCode *int64 `locationName:"exitCode" type:"integer"`

	NetworkBindings []*NetworkBinding `locationName:"networkBindings" type:"list"`

	Reason *string `locationName:"reason" type:"string"`

	Status *string `locationName:"status" type:"string"`
}

// String returns the string representation
func (s ContainerStateChange) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ContainerStateChange) GoString() string {
	return s.String()
}

// SetContainerName sets the ContainerName field's value.
func (s *ContainerStateChange) SetContainerName(v string) *ContainerStateChange {
	s.ContainerName = &v
	return s
}

// SetExitCode sets the ExitCode field's value.
func (s *ContainerStateChange) SetExitCode(v int64) *ContainerStateChange {
	s.ExitCode = &v
	return s
}

// SetNetworkBindings sets the NetworkBindings field's value.
func (s *ContainerStateChange) SetNetworkBindings(v []*NetworkBinding) *ContainerStateChange {
	s.NetworkBindings = v
	return s
}

// SetReason sets the Reason field's value.
func (s *ContainerStateChange) SetReason(v string) *ContainerStateChange {
	s.Reason = &v
	return s
}

// SetStatus sets the Status field's value.
func (s *ContainerStateChange) SetStatus(v string) *ContainerStateChange {
	s.Status = &v
	return s
}

type CreateClusterInput struct {
	_ struct{} `type:"structure"`

	ClusterName *string `locationName:"clusterName" type:"string"`

	Tags []*Tag `locationName:"tags" type:"list"`
}

// String returns the string representation
func (s CreateClusterInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s CreateClusterInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *CreateClusterInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "CreateClusterInput"}
	if s.Tags != nil {
		for i, v := range s.Tags {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Tags", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetClusterName sets the ClusterName field's value.
func (s *CreateClusterInput) SetClusterName(v string) *CreateClusterInput {
	s.ClusterName = &v
	return s
}

// SetTags sets the Tags field's value.
func (s *CreateClusterInput) SetTags(v []*Tag) *CreateClusterInput {
	s.Tags = v
	return s
}

type CreateClusterOutput struct {
	_ struct{} `type:"structure"`

	Cluster *Cluster `locationName:"cluster" type:"structure"`
}

// String returns the string representation
func (s CreateClusterOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s CreateClusterOutput) GoString() string {
	return s.String()
}

// SetCluster sets the Cluster field's value.
func (s *CreateClusterOutput) SetCluster(v *Cluster) *CreateClusterOutput {
	s.Cluster = v
	return s
}

type CreateServiceInput struct {
	_ struct{} `type:"structure"`

	ClientToken *string `locationName:"clientToken" type:"string"`

	Cluster *string `locationName:"cluster" type:"string"`

	DeploymentConfiguration *DeploymentConfiguration `locationName:"deploymentConfiguration" type:"structure"`

	DeploymentController *DeploymentController `locationName:"deploymentController" type:"structure"`

	DesiredCount *int64 `locationName:"desiredCount" type:"integer"`

	EnableECSManagedTags *bool `locationName:"enableECSManagedTags" type:"boolean"`

	HealthCheckGracePeriodSeconds *int64 `locationName:"healthCheckGracePeriodSeconds" type:"integer"`

	LaunchType *string `locationName:"launchType" type:"string" enum:"LaunchType"`

	LoadBalancers []*LoadBalancer `locationName:"loadBalancers" type:"list"`

	NetworkConfiguration *NetworkConfiguration `locationName:"networkConfiguration" type:"structure"`

	PlacementConstraints []*PlacementConstraint `locationName:"placementConstraints" type:"list"`

	PlacementStrategy []*PlacementStrategy `locationName:"placementStrategy" type:"list"`

	PlatformVersion *string `locationName:"platformVersion" type:"string"`

	PropagateTags *string `locationName:"propagateTags" type:"string" enum:"PropagateTags"`

	Role *string `locationName:"role" type:"string"`

	SchedulingStrategy *string `locationName:"schedulingStrategy" type:"string" enum:"SchedulingStrategy"`

	// ServiceName is a required field
	ServiceName *string `locationName:"serviceName" type:"string" required:"true"`

	ServiceRegistries []*ServiceRegistry `locationName:"serviceRegistries" type:"list"`

	Tags []*Tag `locationName:"tags" type:"list"`

	TaskDefinition *string `locationName:"taskDefinition" type:"string"`
}

// String returns the string representation
func (s CreateServiceInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s CreateServiceInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *CreateServiceInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "CreateServiceInput"}
	if s.ServiceName == nil {
		invalidParams.Add(request.NewErrParamRequired("ServiceName"))
	}
	if s.DeploymentController != nil {
		if err := s.DeploymentController.Validate(); err != nil {
			invalidParams.AddNested("DeploymentController", err.(request.ErrInvalidParams))
		}
	}
	if s.NetworkConfiguration != nil {
		if err := s.NetworkConfiguration.Validate(); err != nil {
			invalidParams.AddNested("NetworkConfiguration", err.(request.ErrInvalidParams))
		}
	}
	if s.Tags != nil {
		for i, v := range s.Tags {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Tags", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetClientToken sets the ClientToken field's value.
func (s *CreateServiceInput) SetClientToken(v string) *CreateServiceInput {
	s.ClientToken = &v
	return s
}

// SetCluster sets the Cluster field's value.
func (s *CreateServiceInput) SetCluster(v string) *CreateServiceInput {
	s.Cluster = &v
	return s
}

// SetDeploymentConfiguration sets the DeploymentConfiguration field's value.
func (s *CreateServiceInput) SetDeploymentConfiguration(v *DeploymentConfiguration) *CreateServiceInput {
	s.DeploymentConfiguration = v
	return s
}

// SetDeploymentController sets the DeploymentController field's value.
func (s *CreateServiceInput) SetDeploymentController(v *DeploymentController) *CreateServiceInput {
	s.DeploymentController = v
	return s
}

// SetDesiredCount sets the DesiredCount field's value.
func (s *CreateServiceInput) SetDesiredCount(v int64) *CreateServiceInput {
	s.DesiredCount = &v
	return s
}

// SetEnableECSManagedTags sets the EnableECSManagedTags field's value.
func (s *CreateServiceInput) SetEnableECSManagedTags(v bool) *CreateServiceInput {
	s.EnableECSManagedTags = &v
	return s
}

// SetHealthCheckGracePeriodSeconds sets the HealthCheckGracePeriodSeconds field's value.
func (s *CreateServiceInput) SetHealthCheckGracePeriodSeconds(v int64) *CreateServiceInput {
	s.HealthCheckGracePeriodSeconds = &v
	return s
}

// SetLaunchType sets the LaunchType field's value.
func (s *CreateServiceInput) SetLaunchType(v string) *CreateServiceInput {
	s.LaunchType = &v
	return s
}

// SetLoadBalancers sets the LoadBalancers field's value.
func (s *CreateServiceInput) SetLoadBalancers(v []*LoadBalancer) *CreateServiceInput {
	s.LoadBalancers = v
	return s
}

// SetNetworkConfiguration sets the NetworkConfiguration field's value.
func (s *CreateServiceInput) SetNetworkConfiguration(v *NetworkConfiguration) *CreateServiceInput {
	s.NetworkConfiguration = v
	return s
}

// SetPlacementConstraints sets the PlacementConstraints field's value.
func (s *CreateServiceInput) SetPlacementConstraints(v []*PlacementConstraint) *CreateServiceInput {
	s.PlacementConstraints = v
	return s
}

// SetPlacementStrategy sets the PlacementStrategy field's value.
func (s *CreateServiceInput) SetPlacementStrategy(v []*PlacementStrategy) *CreateServiceInput {
	s.PlacementStrategy = v
	return s
}

// SetPlatformVersion sets the PlatformVersion field's value.
func (s *CreateServiceInput) SetPlatformVersion(v string) *CreateServiceInput {
	s.PlatformVersion = &v
	return s
}

// SetPropagateTags sets the PropagateTags field's value.
func (s *CreateServiceInput) SetPropagateTags(v string) *CreateServiceInput {
	s.PropagateTags = &v
	return s
}

// SetRole sets the Role field's value.
func (s *CreateServiceInput) SetRole(v string) *CreateServiceInput {
	s.Role = &v
	return s
}

// SetSchedulingStrategy sets the SchedulingStrategy field's value.
func (s *CreateServiceInput) SetSchedulingStrategy(v string) *CreateServiceInput {
	s.SchedulingStrategy = &v
	return s
}

// SetServiceName sets the ServiceName field's value.
func (s *CreateServiceInput) SetServiceName(v string) *CreateServiceInput {
	s.ServiceName = &v
	return s
}

// SetServiceRegistries sets the ServiceRegistries field's value.
func (s *CreateServiceInput) SetServiceRegistries(v []*ServiceRegistry) *CreateServiceInput {
	s.ServiceRegistries = v
	return s
}

// SetTags sets the Tags field's value.
func (s *CreateServiceInput) SetTags(v []*Tag) *CreateServiceInput {
	s.Tags = v
	return s
}

// SetTaskDefinition sets the TaskDefinition field's value.
func (s *CreateServiceInput) SetTaskDefinition(v string) *CreateServiceInput {
	s.TaskDefinition = &v
	return s
}

type CreateServiceOutput struct {
	_ struct{} `type:"structure"`

	Service *Service `locationName:"service" type:"structure"`
}

// String returns the string representation
func (s CreateServiceOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s CreateServiceOutput) GoString() string {
	return s.String()
}

// SetService sets the Service field's value.
func (s *CreateServiceOutput) SetService(v *Service) *CreateServiceOutput {
	s.Service = v
	return s
}

type CreateTaskSetInput struct {
	_ struct{} `type:"structure"`

	ClientToken *string `locationName:"clientToken" type:"string"`

	// Cluster is a required field
	Cluster *string `locationName:"cluster" type:"string" required:"true"`

	ExternalId *string `locationName:"externalId" type:"string"`

	LaunchType *string `locationName:"launchType" type:"string" enum:"LaunchType"`

	LoadBalancers []*LoadBalancer `locationName:"loadBalancers" type:"list"`

	NetworkConfiguration *NetworkConfiguration `locationName:"networkConfiguration" type:"structure"`

	PlatformVersion *string `locationName:"platformVersion" type:"string"`

	Scale *Scale `locationName:"scale" type:"structure"`

	// Service is a required field
	Service *string `locationName:"service" type:"string" required:"true"`

	ServiceRegistries []*ServiceRegistry `locationName:"serviceRegistries" type:"list"`

	StartedBy *string `locationName:"startedBy" type:"string"`

	// TaskDefinition is a required field
	TaskDefinition *string `locationName:"taskDefinition" type:"string" required:"true"`
}

// String returns the string representation
func (s CreateTaskSetInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s CreateTaskSetInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *CreateTaskSetInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "CreateTaskSetInput"}
	if s.Cluster == nil {
		invalidParams.Add(request.NewErrParamRequired("Cluster"))
	}
	if s.Service == nil {
		invalidParams.Add(request.NewErrParamRequired("Service"))
	}
	if s.TaskDefinition == nil {
		invalidParams.Add(request.NewErrParamRequired("TaskDefinition"))
	}
	if s.NetworkConfiguration != nil {
		if err := s.NetworkConfiguration.Validate(); err != nil {
			invalidParams.AddNested("NetworkConfiguration", err.(request.ErrInvalidParams))
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetClientToken sets the ClientToken field's value.
func (s *CreateTaskSetInput) SetClientToken(v string) *CreateTaskSetInput {
	s.ClientToken = &v
	return s
}

// SetCluster sets the Cluster field's value.
func (s *CreateTaskSetInput) SetCluster(v string) *CreateTaskSetInput {
	s.Cluster = &v
	return s
}

// SetExternalId sets the ExternalId field's value.
func (s *CreateTaskSetInput) SetExternalId(v string) *CreateTaskSetInput {
	s.ExternalId = &v
	return s
}

// SetLaunchType sets the LaunchType field's value.
func (s *CreateTaskSetInput) SetLaunchType(v string) *CreateTaskSetInput {
	s.LaunchType = &v
	return s
}

// SetLoadBalancers sets the LoadBalancers field's value.
func (s *CreateTaskSetInput) SetLoadBalancers(v []*LoadBalancer) *CreateTaskSetInput {
	s.LoadBalancers = v
	return s
}

// SetNetworkConfiguration sets the NetworkConfiguration field's value.
func (s *CreateTaskSetInput) SetNetworkConfiguration(v *NetworkConfiguration) *CreateTaskSetInput {
	s.NetworkConfiguration = v
	return s
}

// SetPlatformVersion sets the PlatformVersion field's value.
func (s *CreateTaskSetInput) SetPlatformVersion(v string) *CreateTaskSetInput {
	s.PlatformVersion = &v
	return s
}

// SetScale sets the Scale field's value.
func (s *CreateTaskSetInput) SetScale(v *Scale) *CreateTaskSetInput {
	s.Scale = v
	return s
}

// SetService sets the Service field's value.
func (s *CreateTaskSetInput) SetService(v string) *CreateTaskSetInput {
	s.Service = &v
	return s
}

// SetServiceRegistries sets the ServiceRegistries field's value.
func (s *CreateTaskSetInput) SetServiceRegistries(v []*ServiceRegistry) *CreateTaskSetInput {
	s.ServiceRegistries = v
	return s
}

// SetStartedBy sets the StartedBy field's value.
func (s *CreateTaskSetInput) SetStartedBy(v string) *CreateTaskSetInput {
	s.StartedBy = &v
	return s
}

// SetTaskDefinition sets the TaskDefinition field's value.
func (s *CreateTaskSetInput) SetTaskDefinition(v string) *CreateTaskSetInput {
	s.TaskDefinition = &v
	return s
}

type CreateTaskSetOutput struct {
	_ struct{} `type:"structure"`

	TaskSet *TaskSet `locationName:"taskSet" type:"structure"`
}

// String returns the string representation
func (s CreateTaskSetOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s CreateTaskSetOutput) GoString() string {
	return s.String()
}

// SetTaskSet sets the TaskSet field's value.
func (s *CreateTaskSetOutput) SetTaskSet(v *TaskSet) *CreateTaskSetOutput {
	s.TaskSet = v
	return s
}

type DeleteAccountSettingInput struct {
	_ struct{} `type:"structure"`

	// Name is a required field
	Name *string `locationName:"name" type:"string" required:"true" enum:"SettingName"`

	PrincipalArn *string `locationName:"principalArn" type:"string"`
}

// String returns the string representation
func (s DeleteAccountSettingInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeleteAccountSettingInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DeleteAccountSettingInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DeleteAccountSettingInput"}
	if s.Name == nil {
		invalidParams.Add(request.NewErrParamRequired("Name"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetName sets the Name field's value.
func (s *DeleteAccountSettingInput) SetName(v string) *DeleteAccountSettingInput {
	s.Name = &v
	return s
}

// SetPrincipalArn sets the PrincipalArn field's value.
func (s *DeleteAccountSettingInput) SetPrincipalArn(v string) *DeleteAccountSettingInput {
	s.PrincipalArn = &v
	return s
}

type DeleteAccountSettingOutput struct {
	_ struct{} `type:"structure"`

	Setting *Setting `locationName:"setting" type:"structure"`
}

// String returns the string representation
func (s DeleteAccountSettingOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeleteAccountSettingOutput) GoString() string {
	return s.String()
}

// SetSetting sets the Setting field's value.
func (s *DeleteAccountSettingOutput) SetSetting(v *Setting) *DeleteAccountSettingOutput {
	s.Setting = v
	return s
}

type DeleteAttributesInput struct {
	_ struct{} `type:"structure"`

	// Attributes is a required field
	Attributes []*Attribute `locationName:"attributes" type:"list" required:"true"`

	Cluster *string `locationName:"cluster" type:"string"`
}

// String returns the string representation
func (s DeleteAttributesInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeleteAttributesInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DeleteAttributesInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DeleteAttributesInput"}
	if s.Attributes == nil {
		invalidParams.Add(request.NewErrParamRequired("Attributes"))
	}
	if s.Attributes != nil {
		for i, v := range s.Attributes {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Attributes", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetAttributes sets the Attributes field's value.
func (s *DeleteAttributesInput) SetAttributes(v []*Attribute) *DeleteAttributesInput {
	s.Attributes = v
	return s
}

// SetCluster sets the Cluster field's value.
func (s *DeleteAttributesInput) SetCluster(v string) *DeleteAttributesInput {
	s.Cluster = &v
	return s
}

type DeleteAttributesOutput struct {
	_ struct{} `type:"structure"`

	Attributes []*Attribute `locationName:"attributes" type:"list"`
}

// String returns the string representation
func (s DeleteAttributesOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeleteAttributesOutput) GoString() string {
	return s.String()
}

// SetAttributes sets the Attributes field's value.
func (s *DeleteAttributesOutput) SetAttributes(v []*Attribute) *DeleteAttributesOutput {
	s.Attributes = v
	return s
}

type DeleteClusterInput struct {
	_ struct{} `type:"structure"`

	// Cluster is a required field
	Cluster *string `locationName:"cluster" type:"string" required:"true"`
}

// String returns the string representation
func (s DeleteClusterInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeleteClusterInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DeleteClusterInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DeleteClusterInput"}
	if s.Cluster == nil {
		invalidParams.Add(request.NewErrParamRequired("Cluster"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCluster sets the Cluster field's value.
func (s *DeleteClusterInput) SetCluster(v string) *DeleteClusterInput {
	s.Cluster = &v
	return s
}

type DeleteClusterOutput struct {
	_ struct{} `type:"structure"`

	Cluster *Cluster `locationName:"cluster" type:"structure"`
}

// String returns the string representation
func (s DeleteClusterOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeleteClusterOutput) GoString() string {
	return s.String()
}

// SetCluster sets the Cluster field's value.
func (s *DeleteClusterOutput) SetCluster(v *Cluster) *DeleteClusterOutput {
	s.Cluster = v
	return s
}

type DeleteServiceInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	Force *bool `locationName:"force" type:"boolean"`

	// Service is a required field
	Service *string `locationName:"service" type:"string" required:"true"`
}

// String returns the string representation
func (s DeleteServiceInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeleteServiceInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DeleteServiceInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DeleteServiceInput"}
	if s.Service == nil {
		invalidParams.Add(request.NewErrParamRequired("Service"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCluster sets the Cluster field's value.
func (s *DeleteServiceInput) SetCluster(v string) *DeleteServiceInput {
	s.Cluster = &v
	return s
}

// SetForce sets the Force field's value.
func (s *DeleteServiceInput) SetForce(v bool) *DeleteServiceInput {
	s.Force = &v
	return s
}

// SetService sets the Service field's value.
func (s *DeleteServiceInput) SetService(v string) *DeleteServiceInput {
	s.Service = &v
	return s
}

type DeleteServiceOutput struct {
	_ struct{} `type:"structure"`

	Service *Service `locationName:"service" type:"structure"`
}

// String returns the string representation
func (s DeleteServiceOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeleteServiceOutput) GoString() string {
	return s.String()
}

// SetService sets the Service field's value.
func (s *DeleteServiceOutput) SetService(v *Service) *DeleteServiceOutput {
	s.Service = v
	return s
}

type DeleteTaskSetInput struct {
	_ struct{} `type:"structure"`

	// Cluster is a required field
	Cluster *string `locationName:"cluster" type:"string" required:"true"`

	Force *bool `locationName:"force" type:"boolean"`

	// Service is a required field
	Service *string `locationName:"service" type:"string" required:"true"`

	// TaskSet is a required field
	TaskSet *string `locationName:"taskSet" type:"string" required:"true"`
}

// String returns the string representation
func (s DeleteTaskSetInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeleteTaskSetInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DeleteTaskSetInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DeleteTaskSetInput"}
	if s.Cluster == nil {
		invalidParams.Add(request.NewErrParamRequired("Cluster"))
	}
	if s.Service == nil {
		invalidParams.Add(request.NewErrParamRequired("Service"))
	}
	if s.TaskSet == nil {
		invalidParams.Add(request.NewErrParamRequired("TaskSet"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCluster sets the Cluster field's value.
func (s *DeleteTaskSetInput) SetCluster(v string) *DeleteTaskSetInput {
	s.Cluster = &v
	return s
}

// SetForce sets the Force field's value.
func (s *DeleteTaskSetInput) SetForce(v bool) *DeleteTaskSetInput {
	s.Force = &v
	return s
}

// SetService sets the Service field's value.
func (s *DeleteTaskSetInput) SetService(v string) *DeleteTaskSetInput {
	s.Service = &v
	return s
}

// SetTaskSet sets the TaskSet field's value.
func (s *DeleteTaskSetInput) SetTaskSet(v string) *DeleteTaskSetInput {
	s.TaskSet = &v
	return s
}

type DeleteTaskSetOutput struct {
	_ struct{} `type:"structure"`

	TaskSet *TaskSet `locationName:"taskSet" type:"structure"`
}

// String returns the string representation
func (s DeleteTaskSetOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeleteTaskSetOutput) GoString() string {
	return s.String()
}

// SetTaskSet sets the TaskSet field's value.
func (s *DeleteTaskSetOutput) SetTaskSet(v *TaskSet) *DeleteTaskSetOutput {
	s.TaskSet = v
	return s
}

type Deployment struct {
	_ struct{} `type:"structure"`

	CreatedAt *time.Time `locationName:"createdAt" type:"timestamp"`

	DesiredCount *int64 `locationName:"desiredCount" type:"integer"`

	Id *string `locationName:"id" type:"string"`

	LaunchType *string `locationName:"launchType" type:"string" enum:"LaunchType"`

	NetworkConfiguration *NetworkConfiguration `locationName:"networkConfiguration" type:"structure"`

	PendingCount *int64 `locationName:"pendingCount" type:"integer"`

	PlatformVersion *string `locationName:"platformVersion" type:"string"`

	RunningCount *int64 `locationName:"runningCount" type:"integer"`

	Status *string `locationName:"status" type:"string"`

	TaskDefinition *string `locationName:"taskDefinition" type:"string"`

	UpdatedAt *time.Time `locationName:"updatedAt" type:"timestamp"`
}

// String returns the string representation
func (s Deployment) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Deployment) GoString() string {
	return s.String()
}

// SetCreatedAt sets the CreatedAt field's value.
func (s *Deployment) SetCreatedAt(v time.Time) *Deployment {
	s.CreatedAt = &v
	return s
}

// SetDesiredCount sets the DesiredCount field's value.
func (s *Deployment) SetDesiredCount(v int64) *Deployment {
	s.DesiredCount = &v
	return s
}

// SetId sets the Id field's value.
func (s *Deployment) SetId(v string) *Deployment {
	s.Id = &v
	return s
}

// SetLaunchType sets the LaunchType field's value.
func (s *Deployment) SetLaunchType(v string) *Deployment {
	s.LaunchType = &v
	return s
}

// SetNetworkConfiguration sets the NetworkConfiguration field's value.
func (s *Deployment) SetNetworkConfiguration(v *NetworkConfiguration) *Deployment {
	s.NetworkConfiguration = v
	return s
}

// SetPendingCount sets the PendingCount field's value.
func (s *Deployment) SetPendingCount(v int64) *Deployment {
	s.PendingCount = &v
	return s
}

// SetPlatformVersion sets the PlatformVersion field's value.
func (s *Deployment) SetPlatformVersion(v string) *Deployment {
	s.PlatformVersion = &v
	return s
}

// SetRunningCount sets the RunningCount field's value.
func (s *Deployment) SetRunningCount(v int64) *Deployment {
	s.RunningCount = &v
	return s
}

// SetStatus sets the Status field's value.
func (s *Deployment) SetStatus(v string) *Deployment {
	s.Status = &v
	return s
}

// SetTaskDefinition sets the TaskDefinition field's value.
func (s *Deployment) SetTaskDefinition(v string) *Deployment {
	s.TaskDefinition = &v
	return s
}

// SetUpdatedAt sets the UpdatedAt field's value.
func (s *Deployment) SetUpdatedAt(v time.Time) *Deployment {
	s.UpdatedAt = &v
	return s
}

type DeploymentConfiguration struct {
	_ struct{} `type:"structure"`

	MaximumPercent *int64 `locationName:"maximumPercent" type:"integer"`

	MinimumHealthyPercent *int64 `locationName:"minimumHealthyPercent" type:"integer"`
}

// String returns the string representation
func (s DeploymentConfiguration) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeploymentConfiguration) GoString() string {
	return s.String()
}

// SetMaximumPercent sets the MaximumPercent field's value.
func (s *DeploymentConfiguration) SetMaximumPercent(v int64) *DeploymentConfiguration {
	s.MaximumPercent = &v
	return s
}

// SetMinimumHealthyPercent sets the MinimumHealthyPercent field's value.
func (s *DeploymentConfiguration) SetMinimumHealthyPercent(v int64) *DeploymentConfiguration {
	s.MinimumHealthyPercent = &v
	return s
}

type DeploymentController struct {
	_ struct{} `type:"structure"`

	// Type is a required field
	Type *string `locationName:"type" type:"string" required:"true" enum:"DeploymentControllerType"`
}

// String returns the string representation
func (s DeploymentController) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeploymentController) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DeploymentController) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DeploymentController"}
	if s.Type == nil {
		invalidParams.Add(request.NewErrParamRequired("Type"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetType sets the Type field's value.
func (s *DeploymentController) SetType(v string) *DeploymentController {
	s.Type = &v
	return s
}

type DeregisterContainerInstanceInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	// ContainerInstance is a required field
	ContainerInstance *string `locationName:"containerInstance" type:"string" required:"true"`

	Force *bool `locationName:"force" type:"boolean"`
}

// String returns the string representation
func (s DeregisterContainerInstanceInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeregisterContainerInstanceInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DeregisterContainerInstanceInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DeregisterContainerInstanceInput"}
	if s.ContainerInstance == nil {
		invalidParams.Add(request.NewErrParamRequired("ContainerInstance"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCluster sets the Cluster field's value.
func (s *DeregisterContainerInstanceInput) SetCluster(v string) *DeregisterContainerInstanceInput {
	s.Cluster = &v
	return s
}

// SetContainerInstance sets the ContainerInstance field's value.
func (s *DeregisterContainerInstanceInput) SetContainerInstance(v string) *DeregisterContainerInstanceInput {
	s.ContainerInstance = &v
	return s
}

// SetForce sets the Force field's value.
func (s *DeregisterContainerInstanceInput) SetForce(v bool) *DeregisterContainerInstanceInput {
	s.Force = &v
	return s
}

type DeregisterContainerInstanceOutput struct {
	_ struct{} `type:"structure"`

	ContainerInstance *ContainerInstance `locationName:"containerInstance" type:"structure"`
}

// String returns the string representation
func (s DeregisterContainerInstanceOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeregisterContainerInstanceOutput) GoString() string {
	return s.String()
}

// SetContainerInstance sets the ContainerInstance field's value.
func (s *DeregisterContainerInstanceOutput) SetContainerInstance(v *ContainerInstance) *DeregisterContainerInstanceOutput {
	s.ContainerInstance = v
	return s
}

type DeregisterTaskDefinitionInput struct {
	_ struct{} `type:"structure"`

	// TaskDefinition is a required field
	TaskDefinition *string `locationName:"taskDefinition" type:"string" required:"true"`
}

// String returns the string representation
func (s DeregisterTaskDefinitionInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeregisterTaskDefinitionInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DeregisterTaskDefinitionInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DeregisterTaskDefinitionInput"}
	if s.TaskDefinition == nil {
		invalidParams.Add(request.NewErrParamRequired("TaskDefinition"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetTaskDefinition sets the TaskDefinition field's value.
func (s *DeregisterTaskDefinitionInput) SetTaskDefinition(v string) *DeregisterTaskDefinitionInput {
	s.TaskDefinition = &v
	return s
}

type DeregisterTaskDefinitionOutput struct {
	_ struct{} `type:"structure"`

	TaskDefinition *TaskDefinition `locationName:"taskDefinition" type:"structure"`
}

// String returns the string representation
func (s DeregisterTaskDefinitionOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeregisterTaskDefinitionOutput) GoString() string {
	return s.String()
}

// SetTaskDefinition sets the TaskDefinition field's value.
func (s *DeregisterTaskDefinitionOutput) SetTaskDefinition(v *TaskDefinition) *DeregisterTaskDefinitionOutput {
	s.TaskDefinition = v
	return s
}

type DescribeClustersInput struct {
	_ struct{} `type:"structure"`

	Clusters []*string `locationName:"clusters" type:"list"`

	Include []*string `locationName:"include" type:"list"`
}

// String returns the string representation
func (s DescribeClustersInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeClustersInput) GoString() string {
	return s.String()
}

// SetClusters sets the Clusters field's value.
func (s *DescribeClustersInput) SetClusters(v []*string) *DescribeClustersInput {
	s.Clusters = v
	return s
}

// SetInclude sets the Include field's value.
func (s *DescribeClustersInput) SetInclude(v []*string) *DescribeClustersInput {
	s.Include = v
	return s
}

type DescribeClustersOutput struct {
	_ struct{} `type:"structure"`

	Clusters []*Cluster `locationName:"clusters" type:"list"`

	Failures []*Failure `locationName:"failures" type:"list"`
}

// String returns the string representation
func (s DescribeClustersOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeClustersOutput) GoString() string {
	return s.String()
}

// SetClusters sets the Clusters field's value.
func (s *DescribeClustersOutput) SetClusters(v []*Cluster) *DescribeClustersOutput {
	s.Clusters = v
	return s
}

// SetFailures sets the Failures field's value.
func (s *DescribeClustersOutput) SetFailures(v []*Failure) *DescribeClustersOutput {
	s.Failures = v
	return s
}

type DescribeContainerInstancesInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	// ContainerInstances is a required field
	ContainerInstances []*string `locationName:"containerInstances" type:"list" required:"true"`

	Include []*string `locationName:"include" type:"list"`
}

// String returns the string representation
func (s DescribeContainerInstancesInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeContainerInstancesInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DescribeContainerInstancesInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DescribeContainerInstancesInput"}
	if s.ContainerInstances == nil {
		invalidParams.Add(request.NewErrParamRequired("ContainerInstances"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCluster sets the Cluster field's value.
func (s *DescribeContainerInstancesInput) SetCluster(v string) *DescribeContainerInstancesInput {
	s.Cluster = &v
	return s
}

// SetContainerInstances sets the ContainerInstances field's value.
func (s *DescribeContainerInstancesInput) SetContainerInstances(v []*string) *DescribeContainerInstancesInput {
	s.ContainerInstances = v
	return s
}

// SetInclude sets the Include field's value.
func (s *DescribeContainerInstancesInput) SetInclude(v []*string) *DescribeContainerInstancesInput {
	s.Include = v
	return s
}

type DescribeContainerInstancesOutput struct {
	_ struct{} `type:"structure"`

	ContainerInstances []*ContainerInstance `locationName:"containerInstances" type:"list"`

	Failures []*Failure `locationName:"failures" type:"list"`
}

// String returns the string representation
func (s DescribeContainerInstancesOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeContainerInstancesOutput) GoString() string {
	return s.String()
}

// SetContainerInstances sets the ContainerInstances field's value.
func (s *DescribeContainerInstancesOutput) SetContainerInstances(v []*ContainerInstance) *DescribeContainerInstancesOutput {
	s.ContainerInstances = v
	return s
}

// SetFailures sets the Failures field's value.
func (s *DescribeContainerInstancesOutput) SetFailures(v []*Failure) *DescribeContainerInstancesOutput {
	s.Failures = v
	return s
}

type DescribeServicesInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	Include []*string `locationName:"include" type:"list"`

	// Services is a required field
	Services []*string `locationName:"services" type:"list" required:"true"`
}

// String returns the string representation
func (s DescribeServicesInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeServicesInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DescribeServicesInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DescribeServicesInput"}
	if s.Services == nil {
		invalidParams.Add(request.NewErrParamRequired("Services"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCluster sets the Cluster field's value.
func (s *DescribeServicesInput) SetCluster(v string) *DescribeServicesInput {
	s.Cluster = &v
	return s
}

// SetInclude sets the Include field's value.
func (s *DescribeServicesInput) SetInclude(v []*string) *DescribeServicesInput {
	s.Include = v
	return s
}

// SetServices sets the Services field's value.
func (s *DescribeServicesInput) SetServices(v []*string) *DescribeServicesInput {
	s.Services = v
	return s
}

type DescribeServicesOutput struct {
	_ struct{} `type:"structure"`

	Failures []*Failure `locationName:"failures" type:"list"`

	Services []*Service `locationName:"services" type:"list"`
}

// String returns the string representation
func (s DescribeServicesOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeServicesOutput) GoString() string {
	return s.String()
}

// SetFailures sets the Failures field's value.
func (s *DescribeServicesOutput) SetFailures(v []*Failure) *DescribeServicesOutput {
	s.Failures = v
	return s
}

// SetServices sets the Services field's value.
func (s *DescribeServicesOutput) SetServices(v []*Service) *DescribeServicesOutput {
	s.Services = v
	return s
}

type DescribeTaskDefinitionInput struct {
	_ struct{} `type:"structure"`

	Include []*string `locationName:"include" type:"list"`

	// TaskDefinition is a required field
	TaskDefinition *string `locationName:"taskDefinition" type:"string" required:"true"`
}

// String returns the string representation
func (s DescribeTaskDefinitionInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeTaskDefinitionInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DescribeTaskDefinitionInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DescribeTaskDefinitionInput"}
	if s.TaskDefinition == nil {
		invalidParams.Add(request.NewErrParamRequired("TaskDefinition"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetInclude sets the Include field's value.
func (s *DescribeTaskDefinitionInput) SetInclude(v []*string) *DescribeTaskDefinitionInput {
	s.Include = v
	return s
}

// SetTaskDefinition sets the TaskDefinition field's value.
func (s *DescribeTaskDefinitionInput) SetTaskDefinition(v string) *DescribeTaskDefinitionInput {
	s.TaskDefinition = &v
	return s
}

type DescribeTaskDefinitionOutput struct {
	_ struct{} `type:"structure"`

	Tags []*Tag `locationName:"tags" type:"list"`

	TaskDefinition *TaskDefinition `locationName:"taskDefinition" type:"structure"`
}

// String returns the string representation
func (s DescribeTaskDefinitionOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeTaskDefinitionOutput) GoString() string {
	return s.String()
}

// SetTags sets the Tags field's value.
func (s *DescribeTaskDefinitionOutput) SetTags(v []*Tag) *DescribeTaskDefinitionOutput {
	s.Tags = v
	return s
}

// SetTaskDefinition sets the TaskDefinition field's value.
func (s *DescribeTaskDefinitionOutput) SetTaskDefinition(v *TaskDefinition) *DescribeTaskDefinitionOutput {
	s.TaskDefinition = v
	return s
}

type DescribeTaskSetsInput struct {
	_ struct{} `type:"structure"`

	// Cluster is a required field
	Cluster *string `locationName:"cluster" type:"string" required:"true"`

	Include []*string `locationName:"include" type:"list"`

	// Service is a required field
	Service *string `locationName:"service" type:"string" required:"true"`

	TaskSets []*string `locationName:"taskSets" type:"list"`
}

// String returns the string representation
func (s DescribeTaskSetsInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeTaskSetsInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DescribeTaskSetsInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DescribeTaskSetsInput"}
	if s.Cluster == nil {
		invalidParams.Add(request.NewErrParamRequired("Cluster"))
	}
	if s.Service == nil {
		invalidParams.Add(request.NewErrParamRequired("Service"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCluster sets the Cluster field's value.
func (s *DescribeTaskSetsInput) SetCluster(v string) *DescribeTaskSetsInput {
	s.Cluster = &v
	return s
}

// SetInclude sets the Include field's value.
func (s *DescribeTaskSetsInput) SetInclude(v []*string) *DescribeTaskSetsInput {
	s.Include = v
	return s
}

// SetService sets the Service field's value.
func (s *DescribeTaskSetsInput) SetService(v string) *DescribeTaskSetsInput {
	s.Service = &v
	return s
}

// SetTaskSets sets the TaskSets field's value.
func (s *DescribeTaskSetsInput) SetTaskSets(v []*string) *DescribeTaskSetsInput {
	s.TaskSets = v
	return s
}

type DescribeTaskSetsOutput struct {
	_ struct{} `type:"structure"`

	Failures []*Failure `locationName:"failures" type:"list"`

	TaskSets []*TaskSet `locationName:"taskSets" type:"list"`
}

// String returns the string representation
func (s DescribeTaskSetsOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeTaskSetsOutput) GoString() string {
	return s.String()
}

// SetFailures sets the Failures field's value.
func (s *DescribeTaskSetsOutput) SetFailures(v []*Failure) *DescribeTaskSetsOutput {
	s.Failures = v
	return s
}

// SetTaskSets sets the TaskSets field's value.
func (s *DescribeTaskSetsOutput) SetTaskSets(v []*TaskSet) *DescribeTaskSetsOutput {
	s.TaskSets = v
	return s
}

type DescribeTasksInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	Include []*string `locationName:"include" type:"list"`

	// Tasks is a required field
	Tasks []*string `locationName:"tasks" type:"list" required:"true"`
}

// String returns the string representation
func (s DescribeTasksInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeTasksInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DescribeTasksInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DescribeTasksInput"}
	if s.Tasks == nil {
		invalidParams.Add(request.NewErrParamRequired("Tasks"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCluster sets the Cluster field's value.
func (s *DescribeTasksInput) SetCluster(v string) *DescribeTasksInput {
	s.Cluster = &v
	return s
}

// SetInclude sets the Include field's value.
func (s *DescribeTasksInput) SetInclude(v []*string) *DescribeTasksInput {
	s.Include = v
	return s
}

// SetTasks sets the Tasks field's value.
func (s *DescribeTasksInput) SetTasks(v []*string) *DescribeTasksInput {
	s.Tasks = v
	return s
}

type DescribeTasksOutput struct {
	_ struct{} `type:"structure"`

	Failures []*Failure `locationName:"failures" type:"list"`

	Tasks []*Task `locationName:"tasks" type:"list"`
}

// String returns the string representation
func (s DescribeTasksOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeTasksOutput) GoString() string {
	return s.String()
}

// SetFailures sets the Failures field's value.
func (s *DescribeTasksOutput) SetFailures(v []*Failure) *DescribeTasksOutput {
	s.Failures = v
	return s
}

// SetTasks sets the Tasks field's value.
func (s *DescribeTasksOutput) SetTasks(v []*Task) *DescribeTasksOutput {
	s.Tasks = v
	return s
}

type Device struct {
	_ struct{} `type:"structure"`

	ContainerPath *string `locationName:"containerPath" type:"string"`

	// HostPath is a required field
	HostPath *string `locationName:"hostPath" type:"string" required:"true"`

	Permissions []*string `locationName:"permissions" type:"list"`
}

// String returns the string representation
func (s Device) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Device) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *Device) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "Device"}
	if s.HostPath == nil {
		invalidParams.Add(request.NewErrParamRequired("HostPath"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetContainerPath sets the ContainerPath field's value.
func (s *Device) SetContainerPath(v string) *Device {
	s.ContainerPath = &v
	return s
}

// SetHostPath sets the HostPath field's value.
func (s *Device) SetHostPath(v string) *Device {
	s.HostPath = &v
	return s
}

// SetPermissions sets the Permissions field's value.
func (s *Device) SetPermissions(v []*string) *Device {
	s.Permissions = v
	return s
}

type DiscoverPollEndpointInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	ContainerInstance *string `locationName:"containerInstance" type:"string"`
}

// String returns the string representation
func (s DiscoverPollEndpointInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DiscoverPollEndpointInput) GoString() string {
	return s.String()
}

// SetCluster sets the Cluster field's value.
func (s *DiscoverPollEndpointInput) SetCluster(v string) *DiscoverPollEndpointInput {
	s.Cluster = &v
	return s
}

// SetContainerInstance sets the ContainerInstance field's value.
func (s *DiscoverPollEndpointInput) SetContainerInstance(v string) *DiscoverPollEndpointInput {
	s.ContainerInstance = &v
	return s
}

type DiscoverPollEndpointOutput struct {
	_ struct{} `type:"structure"`

	Endpoint *string `locationName:"endpoint" type:"string"`

	TelemetryEndpoint *string `locationName:"telemetryEndpoint" type:"string"`
}

// String returns the string representation
func (s DiscoverPollEndpointOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DiscoverPollEndpointOutput) GoString() string {
	return s.String()
}

// SetEndpoint sets the Endpoint field's value.
func (s *DiscoverPollEndpointOutput) SetEndpoint(v string) *DiscoverPollEndpointOutput {
	s.Endpoint = &v
	return s
}

// SetTelemetryEndpoint sets the TelemetryEndpoint field's value.
func (s *DiscoverPollEndpointOutput) SetTelemetryEndpoint(v string) *DiscoverPollEndpointOutput {
	s.TelemetryEndpoint = &v
	return s
}

type DockerVolumeConfiguration struct {
	_ struct{} `type:"structure"`

	Autoprovision *bool `locationName:"autoprovision" type:"boolean"`

	Driver *string `locationName:"driver" type:"string"`

	DriverOpts map[string]*string `locationName:"driverOpts" type:"map"`

	Labels map[string]*string `locationName:"labels" type:"map"`

	Scope *string `locationName:"scope" type:"string" enum:"Scope"`
}

// String returns the string representation
func (s DockerVolumeConfiguration) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DockerVolumeConfiguration) GoString() string {
	return s.String()
}

// SetAutoprovision sets the Autoprovision field's value.
func (s *DockerVolumeConfiguration) SetAutoprovision(v bool) *DockerVolumeConfiguration {
	s.Autoprovision = &v
	return s
}

// SetDriver sets the Driver field's value.
func (s *DockerVolumeConfiguration) SetDriver(v string) *DockerVolumeConfiguration {
	s.Driver = &v
	return s
}

// SetDriverOpts sets the DriverOpts field's value.
func (s *DockerVolumeConfiguration) SetDriverOpts(v map[string]*string) *DockerVolumeConfiguration {
	s.DriverOpts = v
	return s
}

// SetLabels sets the Labels field's value.
func (s *DockerVolumeConfiguration) SetLabels(v map[string]*string) *DockerVolumeConfiguration {
	s.Labels = v
	return s
}

// SetScope sets the Scope field's value.
func (s *DockerVolumeConfiguration) SetScope(v string) *DockerVolumeConfiguration {
	s.Scope = &v
	return s
}

type Failure struct {
	_ struct{} `type:"structure"`

	Arn *string `locationName:"arn" type:"string"`

	Reason *string `locationName:"reason" type:"string"`
}

// String returns the string representation
func (s Failure) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Failure) GoString() string {
	return s.String()
}

// SetArn sets the Arn field's value.
func (s *Failure) SetArn(v string) *Failure {
	s.Arn = &v
	return s
}

// SetReason sets the Reason field's value.
func (s *Failure) SetReason(v string) *Failure {
	s.Reason = &v
	return s
}

type HealthCheck struct {
	_ struct{} `type:"structure"`

	// Command is a required field
	Command []*string `locationName:"command" type:"list" required:"true"`

	Interval *int64 `locationName:"interval" type:"integer"`

	Retries *int64 `locationName:"retries" type:"integer"`

	StartPeriod *int64 `locationName:"startPeriod" type:"integer"`

	Timeout *int64 `locationName:"timeout" type:"integer"`
}

// String returns the string representation
func (s HealthCheck) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s HealthCheck) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *HealthCheck) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "HealthCheck"}
	if s.Command == nil {
		invalidParams.Add(request.NewErrParamRequired("Command"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCommand sets the Command field's value.
func (s *HealthCheck) SetCommand(v []*string) *HealthCheck {
	s.Command = v
	return s
}

// SetInterval sets the Interval field's value.
func (s *HealthCheck) SetInterval(v int64) *HealthCheck {
	s.Interval = &v
	return s
}

// SetRetries sets the Retries field's value.
func (s *HealthCheck) SetRetries(v int64) *HealthCheck {
	s.Retries = &v
	return s
}

// SetStartPeriod sets the StartPeriod field's value.
func (s *HealthCheck) SetStartPeriod(v int64) *HealthCheck {
	s.StartPeriod = &v
	return s
}

// SetTimeout sets the Timeout field's value.
func (s *HealthCheck) SetTimeout(v int64) *HealthCheck {
	s.Timeout = &v
	return s
}

type HostEntry struct {
	_ struct{} `type:"structure"`

	// Hostname is a required field
	Hostname *string `locationName:"hostname" type:"string" required:"true"`

	// IpAddress is a required field
	IpAddress *string `locationName:"ipAddress" type:"string" required:"true"`
}

// String returns the string representation
func (s HostEntry) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s HostEntry) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *HostEntry) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "HostEntry"}
	if s.Hostname == nil {
		invalidParams.Add(request.NewErrParamRequired("Hostname"))
	}
	if s.IpAddress == nil {
		invalidParams.Add(request.NewErrParamRequired("IpAddress"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetHostname sets the Hostname field's value.
func (s *HostEntry) SetHostname(v string) *HostEntry {
	s.Hostname = &v
	return s
}

// SetIpAddress sets the IpAddress field's value.
func (s *HostEntry) SetIpAddress(v string) *HostEntry {
	s.IpAddress = &v
	return s
}

type HostVolumeProperties struct {
	_ struct{} `type:"structure"`

	SourcePath *string `locationName:"sourcePath" type:"string"`
}

// String returns the string representation
func (s HostVolumeProperties) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s HostVolumeProperties) GoString() string {
	return s.String()
}

// SetSourcePath sets the SourcePath field's value.
func (s *HostVolumeProperties) SetSourcePath(v string) *HostVolumeProperties {
	s.SourcePath = &v
	return s
}

type InferenceAccelerator struct {
	_ struct{} `type:"structure"`

	// DeviceName is a required field
	DeviceName *string `locationName:"deviceName" type:"string" required:"true"`

	DevicePolicy *string `locationName:"devicePolicy" type:"string"`

	// DeviceType is a required field
	DeviceType *string `locationName:"deviceType" type:"string" required:"true"`
}

// String returns the string representation
func (s InferenceAccelerator) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s InferenceAccelerator) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *InferenceAccelerator) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "InferenceAccelerator"}
	if s.DeviceName == nil {
		invalidParams.Add(request.NewErrParamRequired("DeviceName"))
	}
	if s.DeviceType == nil {
		invalidParams.Add(request.NewErrParamRequired("DeviceType"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetDeviceName sets the DeviceName field's value.
func (s *InferenceAccelerator) SetDeviceName(v string) *InferenceAccelerator {
	s.DeviceName = &v
	return s
}

// SetDevicePolicy sets the DevicePolicy field's value.
func (s *InferenceAccelerator) SetDevicePolicy(v string) *InferenceAccelerator {
	s.DevicePolicy = &v
	return s
}

// SetDeviceType sets the DeviceType field's value.
func (s *InferenceAccelerator) SetDeviceType(v string) *InferenceAccelerator {
	s.DeviceType = &v
	return s
}

type KernelCapabilities struct {
	_ struct{} `type:"structure"`

	Add []*string `locationName:"add" type:"list"`

	Drop []*string `locationName:"drop" type:"list"`
}

// String returns the string representation
func (s KernelCapabilities) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s KernelCapabilities) GoString() string {
	return s.String()
}

// SetAdd sets the Add field's value.
func (s *KernelCapabilities) SetAdd(v []*string) *KernelCapabilities {
	s.Add = v
	return s
}

// SetDrop sets the Drop field's value.
func (s *KernelCapabilities) SetDrop(v []*string) *KernelCapabilities {
	s.Drop = v
	return s
}

type KeyValuePair struct {
	_ struct{} `type:"structure"`

	Name *string `locationName:"name" type:"string"`

	Value *string `locationName:"value" type:"string"`
}

// String returns the string representation
func (s KeyValuePair) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s KeyValuePair) GoString() string {
	return s.String()
}

// SetName sets the Name field's value.
func (s *KeyValuePair) SetName(v string) *KeyValuePair {
	s.Name = &v
	return s
}

// SetValue sets the Value field's value.
func (s *KeyValuePair) SetValue(v string) *KeyValuePair {
	s.Value = &v
	return s
}

type LinuxParameters struct {
	_ struct{} `type:"structure"`

	Capabilities *KernelCapabilities `locationName:"capabilities" type:"structure"`

	Devices []*Device `locationName:"devices" type:"list"`

	InitProcessEnabled *bool `locationName:"initProcessEnabled" type:"boolean"`

	SharedMemorySize *int64 `locationName:"sharedMemorySize" type:"integer"`

	Tmpfs []*Tmpfs `locationName:"tmpfs" type:"list"`
}

// String returns the string representation
func (s LinuxParameters) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s LinuxParameters) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *LinuxParameters) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "LinuxParameters"}
	if s.Devices != nil {
		for i, v := range s.Devices {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Devices", i), err.(request.ErrInvalidParams))
			}
		}
	}
	if s.Tmpfs != nil {
		for i, v := range s.Tmpfs {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Tmpfs", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCapabilities sets the Capabilities field's value.
func (s *LinuxParameters) SetCapabilities(v *KernelCapabilities) *LinuxParameters {
	s.Capabilities = v
	return s
}

// SetDevices sets the Devices field's value.
func (s *LinuxParameters) SetDevices(v []*Device) *LinuxParameters {
	s.Devices = v
	return s
}

// SetInitProcessEnabled sets the InitProcessEnabled field's value.
func (s *LinuxParameters) SetInitProcessEnabled(v bool) *LinuxParameters {
	s.InitProcessEnabled = &v
	return s
}

// SetSharedMemorySize sets the SharedMemorySize field's value.
func (s *LinuxParameters) SetSharedMemorySize(v int64) *LinuxParameters {
	s.SharedMemorySize = &v
	return s
}

// SetTmpfs sets the Tmpfs field's value.
func (s *LinuxParameters) SetTmpfs(v []*Tmpfs) *LinuxParameters {
	s.Tmpfs = v
	return s
}

type ListAccountSettingsInput struct {
	_ struct{} `type:"structure"`

	EffectiveSettings *bool `locationName:"effectiveSettings" type:"boolean"`

	MaxResults *int64 `locationName:"maxResults" type:"integer"`

	Name *string `locationName:"name" type:"string" enum:"SettingName"`

	NextToken *string `locationName:"nextToken" type:"string"`

	PrincipalArn *string `locationName:"principalArn" type:"string"`

	Value *string `locationName:"value" type:"string"`
}

// String returns the string representation
func (s ListAccountSettingsInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListAccountSettingsInput) GoString() string {
	return s.String()
}

// SetEffectiveSettings sets the EffectiveSettings field's value.
func (s *ListAccountSettingsInput) SetEffectiveSettings(v bool) *ListAccountSettingsInput {
	s.EffectiveSettings = &v
	return s
}

// SetMaxResults sets the MaxResults field's value.
func (s *ListAccountSettingsInput) SetMaxResults(v int64) *ListAccountSettingsInput {
	s.MaxResults = &v
	return s
}

// SetName sets the Name field's value.
func (s *ListAccountSettingsInput) SetName(v string) *ListAccountSettingsInput {
	s.Name = &v
	return s
}

// SetNextToken sets the NextToken field's value.
func (s *ListAccountSettingsInput) SetNextToken(v string) *ListAccountSettingsInput {
	s.NextToken = &v
	return s
}

// SetPrincipalArn sets the PrincipalArn field's value.
func (s *ListAccountSettingsInput) SetPrincipalArn(v string) *ListAccountSettingsInput {
	s.PrincipalArn = &v
	return s
}

// SetValue sets the Value field's value.
func (s *ListAccountSettingsInput) SetValue(v string) *ListAccountSettingsInput {
	s.Value = &v
	return s
}

type ListAccountSettingsOutput struct {
	_ struct{} `type:"structure"`

	NextToken *string `locationName:"nextToken" type:"string"`

	Settings []*Setting `locationName:"settings" type:"list"`
}

// String returns the string representation
func (s ListAccountSettingsOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListAccountSettingsOutput) GoString() string {
	return s.String()
}

// SetNextToken sets the NextToken field's value.
func (s *ListAccountSettingsOutput) SetNextToken(v string) *ListAccountSettingsOutput {
	s.NextToken = &v
	return s
}

// SetSettings sets the Settings field's value.
func (s *ListAccountSettingsOutput) SetSettings(v []*Setting) *ListAccountSettingsOutput {
	s.Settings = v
	return s
}

type ListAttributesInput struct {
	_ struct{} `type:"structure"`

	AttributeName *string `locationName:"attributeName" type:"string"`

	AttributeValue *string `locationName:"attributeValue" type:"string"`

	Cluster *string `locationName:"cluster" type:"string"`

	MaxResults *int64 `locationName:"maxResults" type:"integer"`

	NextToken *string `locationName:"nextToken" type:"string"`

	// TargetType is a required field
	TargetType *string `locationName:"targetType" type:"string" required:"true" enum:"TargetType"`
}

// String returns the string representation
func (s ListAttributesInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListAttributesInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *ListAttributesInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "ListAttributesInput"}
	if s.TargetType == nil {
		invalidParams.Add(request.NewErrParamRequired("TargetType"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetAttributeName sets the AttributeName field's value.
func (s *ListAttributesInput) SetAttributeName(v string) *ListAttributesInput {
	s.AttributeName = &v
	return s
}

// SetAttributeValue sets the AttributeValue field's value.
func (s *ListAttributesInput) SetAttributeValue(v string) *ListAttributesInput {
	s.AttributeValue = &v
	return s
}

// SetCluster sets the Cluster field's value.
func (s *ListAttributesInput) SetCluster(v string) *ListAttributesInput {
	s.Cluster = &v
	return s
}

// SetMaxResults sets the MaxResults field's value.
func (s *ListAttributesInput) SetMaxResults(v int64) *ListAttributesInput {
	s.MaxResults = &v
	return s
}

// SetNextToken sets the NextToken field's value.
func (s *ListAttributesInput) SetNextToken(v string) *ListAttributesInput {
	s.NextToken = &v
	return s
}

// SetTargetType sets the TargetType field's value.
func (s *ListAttributesInput) SetTargetType(v string) *ListAttributesInput {
	s.TargetType = &v
	return s
}

type ListAttributesOutput struct {
	_ struct{} `type:"structure"`

	Attributes []*Attribute `locationName:"attributes" type:"list"`

	NextToken *string `locationName:"nextToken" type:"string"`
}

// String returns the string representation
func (s ListAttributesOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListAttributesOutput) GoString() string {
	return s.String()
}

// SetAttributes sets the Attributes field's value.
func (s *ListAttributesOutput) SetAttributes(v []*Attribute) *ListAttributesOutput {
	s.Attributes = v
	return s
}

// SetNextToken sets the NextToken field's value.
func (s *ListAttributesOutput) SetNextToken(v string) *ListAttributesOutput {
	s.NextToken = &v
	return s
}

type ListClustersInput struct {
	_ struct{} `type:"structure"`

	MaxResults *int64 `locationName:"maxResults" type:"integer"`

	NextToken *string `locationName:"nextToken" type:"string"`
}

// String returns the string representation
func (s ListClustersInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListClustersInput) GoString() string {
	return s.String()
}

// SetMaxResults sets the MaxResults field's value.
func (s *ListClustersInput) SetMaxResults(v int64) *ListClustersInput {
	s.MaxResults = &v
	return s
}

// SetNextToken sets the NextToken field's value.
func (s *ListClustersInput) SetNextToken(v string) *ListClustersInput {
	s.NextToken = &v
	return s
}

type ListClustersOutput struct {
	_ struct{} `type:"structure"`

	ClusterArns []*string `locationName:"clusterArns" type:"list"`

	NextToken *string `locationName:"nextToken" type:"string"`
}

// String returns the string representation
func (s ListClustersOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListClustersOutput) GoString() string {
	return s.String()
}

// SetClusterArns sets the ClusterArns field's value.
func (s *ListClustersOutput) SetClusterArns(v []*string) *ListClustersOutput {
	s.ClusterArns = v
	return s
}

// SetNextToken sets the NextToken field's value.
func (s *ListClustersOutput) SetNextToken(v string) *ListClustersOutput {
	s.NextToken = &v
	return s
}

type ListContainerInstancesInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	Filter *string `locationName:"filter" type:"string"`

	MaxResults *int64 `locationName:"maxResults" type:"integer"`

	NextToken *string `locationName:"nextToken" type:"string"`

	Status *string `locationName:"status" type:"string" enum:"ContainerInstanceStatus"`
}

// String returns the string representation
func (s ListContainerInstancesInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListContainerInstancesInput) GoString() string {
	return s.String()
}

// SetCluster sets the Cluster field's value.
func (s *ListContainerInstancesInput) SetCluster(v string) *ListContainerInstancesInput {
	s.Cluster = &v
	return s
}

// SetFilter sets the Filter field's value.
func (s *ListContainerInstancesInput) SetFilter(v string) *ListContainerInstancesInput {
	s.Filter = &v
	return s
}

// SetMaxResults sets the MaxResults field's value.
func (s *ListContainerInstancesInput) SetMaxResults(v int64) *ListContainerInstancesInput {
	s.MaxResults = &v
	return s
}

// SetNextToken sets the NextToken field's value.
func (s *ListContainerInstancesInput) SetNextToken(v string) *ListContainerInstancesInput {
	s.NextToken = &v
	return s
}

// SetStatus sets the Status field's value.
func (s *ListContainerInstancesInput) SetStatus(v string) *ListContainerInstancesInput {
	s.Status = &v
	return s
}

type ListContainerInstancesOutput struct {
	_ struct{} `type:"structure"`

	ContainerInstanceArns []*string `locationName:"containerInstanceArns" type:"list"`

	NextToken *string `locationName:"nextToken" type:"string"`
}

// String returns the string representation
func (s ListContainerInstancesOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListContainerInstancesOutput) GoString() string {
	return s.String()
}

// SetContainerInstanceArns sets the ContainerInstanceArns field's value.
func (s *ListContainerInstancesOutput) SetContainerInstanceArns(v []*string) *ListContainerInstancesOutput {
	s.ContainerInstanceArns = v
	return s
}

// SetNextToken sets the NextToken field's value.
func (s *ListContainerInstancesOutput) SetNextToken(v string) *ListContainerInstancesOutput {
	s.NextToken = &v
	return s
}

type ListServicesInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	LaunchType *string `locationName:"launchType" type:"string" enum:"LaunchType"`

	MaxResults *int64 `locationName:"maxResults" type:"integer"`

	NextToken *string `locationName:"nextToken" type:"string"`

	SchedulingStrategy *string `locationName:"schedulingStrategy" type:"string" enum:"SchedulingStrategy"`
}

// String returns the string representation
func (s ListServicesInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListServicesInput) GoString() string {
	return s.String()
}

// SetCluster sets the Cluster field's value.
func (s *ListServicesInput) SetCluster(v string) *ListServicesInput {
	s.Cluster = &v
	return s
}

// SetLaunchType sets the LaunchType field's value.
func (s *ListServicesInput) SetLaunchType(v string) *ListServicesInput {
	s.LaunchType = &v
	return s
}

// SetMaxResults sets the MaxResults field's value.
func (s *ListServicesInput) SetMaxResults(v int64) *ListServicesInput {
	s.MaxResults = &v
	return s
}

// SetNextToken sets the NextToken field's value.
func (s *ListServicesInput) SetNextToken(v string) *ListServicesInput {
	s.NextToken = &v
	return s
}

// SetSchedulingStrategy sets the SchedulingStrategy field's value.
func (s *ListServicesInput) SetSchedulingStrategy(v string) *ListServicesInput {
	s.SchedulingStrategy = &v
	return s
}

type ListServicesOutput struct {
	_ struct{} `type:"structure"`

	NextToken *string `locationName:"nextToken" type:"string"`

	ServiceArns []*string `locationName:"serviceArns" type:"list"`
}

// String returns the string representation
func (s ListServicesOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListServicesOutput) GoString() string {
	return s.String()
}

// SetNextToken sets the NextToken field's value.
func (s *ListServicesOutput) SetNextToken(v string) *ListServicesOutput {
	s.NextToken = &v
	return s
}

// SetServiceArns sets the ServiceArns field's value.
func (s *ListServicesOutput) SetServiceArns(v []*string) *ListServicesOutput {
	s.ServiceArns = v
	return s
}

type ListTagsForResourceInput struct {
	_ struct{} `type:"structure"`

	// ResourceArn is a required field
	ResourceArn *string `locationName:"resourceArn" type:"string" required:"true"`
}

// String returns the string representation
func (s ListTagsForResourceInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListTagsForResourceInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *ListTagsForResourceInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "ListTagsForResourceInput"}
	if s.ResourceArn == nil {
		invalidParams.Add(request.NewErrParamRequired("ResourceArn"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetResourceArn sets the ResourceArn field's value.
func (s *ListTagsForResourceInput) SetResourceArn(v string) *ListTagsForResourceInput {
	s.ResourceArn = &v
	return s
}

type ListTagsForResourceOutput struct {
	_ struct{} `type:"structure"`

	Tags []*Tag `locationName:"tags" type:"list"`
}

// String returns the string representation
func (s ListTagsForResourceOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListTagsForResourceOutput) GoString() string {
	return s.String()
}

// SetTags sets the Tags field's value.
func (s *ListTagsForResourceOutput) SetTags(v []*Tag) *ListTagsForResourceOutput {
	s.Tags = v
	return s
}

type ListTaskDefinitionFamiliesInput struct {
	_ struct{} `type:"structure"`

	FamilyPrefix *string `locationName:"familyPrefix" type:"string"`

	MaxResults *int64 `locationName:"maxResults" type:"integer"`

	NextToken *string `locationName:"nextToken" type:"string"`

	Status *string `locationName:"status" type:"string" enum:"TaskDefinitionFamilyStatus"`
}

// String returns the string representation
func (s ListTaskDefinitionFamiliesInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListTaskDefinitionFamiliesInput) GoString() string {
	return s.String()
}

// SetFamilyPrefix sets the FamilyPrefix field's value.
func (s *ListTaskDefinitionFamiliesInput) SetFamilyPrefix(v string) *ListTaskDefinitionFamiliesInput {
	s.FamilyPrefix = &v
	return s
}

// SetMaxResults sets the MaxResults field's value.
func (s *ListTaskDefinitionFamiliesInput) SetMaxResults(v int64) *ListTaskDefinitionFamiliesInput {
	s.MaxResults = &v
	return s
}

// SetNextToken sets the NextToken field's value.
func (s *ListTaskDefinitionFamiliesInput) SetNextToken(v string) *ListTaskDefinitionFamiliesInput {
	s.NextToken = &v
	return s
}

// SetStatus sets the Status field's value.
func (s *ListTaskDefinitionFamiliesInput) SetStatus(v string) *ListTaskDefinitionFamiliesInput {
	s.Status = &v
	return s
}

type ListTaskDefinitionFamiliesOutput struct {
	_ struct{} `type:"structure"`

	Families []*string `locationName:"families" type:"list"`

	NextToken *string `locationName:"nextToken" type:"string"`
}

// String returns the string representation
func (s ListTaskDefinitionFamiliesOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListTaskDefinitionFamiliesOutput) GoString() string {
	return s.String()
}

// SetFamilies sets the Families field's value.
func (s *ListTaskDefinitionFamiliesOutput) SetFamilies(v []*string) *ListTaskDefinitionFamiliesOutput {
	s.Families = v
	return s
}

// SetNextToken sets the NextToken field's value.
func (s *ListTaskDefinitionFamiliesOutput) SetNextToken(v string) *ListTaskDefinitionFamiliesOutput {
	s.NextToken = &v
	return s
}

type ListTaskDefinitionsInput struct {
	_ struct{} `type:"structure"`

	FamilyPrefix *string `locationName:"familyPrefix" type:"string"`

	MaxResults *int64 `locationName:"maxResults" type:"integer"`

	NextToken *string `locationName:"nextToken" type:"string"`

	Sort *string `locationName:"sort" type:"string" enum:"SortOrder"`

	Status *string `locationName:"status" type:"string" enum:"TaskDefinitionStatus"`
}

// String returns the string representation
func (s ListTaskDefinitionsInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListTaskDefinitionsInput) GoString() string {
	return s.String()
}

// SetFamilyPrefix sets the FamilyPrefix field's value.
func (s *ListTaskDefinitionsInput) SetFamilyPrefix(v string) *ListTaskDefinitionsInput {
	s.FamilyPrefix = &v
	return s
}

// SetMaxResults sets the MaxResults field's value.
func (s *ListTaskDefinitionsInput) SetMaxResults(v int64) *ListTaskDefinitionsInput {
	s.MaxResults = &v
	return s
}

// SetNextToken sets the NextToken field's value.
func (s *ListTaskDefinitionsInput) SetNextToken(v string) *ListTaskDefinitionsInput {
	s.NextToken = &v
	return s
}

// SetSort sets the Sort field's value.
func (s *ListTaskDefinitionsInput) SetSort(v string) *ListTaskDefinitionsInput {
	s.Sort = &v
	return s
}

// SetStatus sets the Status field's value.
func (s *ListTaskDefinitionsInput) SetStatus(v string) *ListTaskDefinitionsInput {
	s.Status = &v
	return s
}

type ListTaskDefinitionsOutput struct {
	_ struct{} `type:"structure"`

	NextToken *string `locationName:"nextToken" type:"string"`

	TaskDefinitionArns []*string `locationName:"taskDefinitionArns" type:"list"`
}

// String returns the string representation
func (s ListTaskDefinitionsOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListTaskDefinitionsOutput) GoString() string {
	return s.String()
}

// SetNextToken sets the NextToken field's value.
func (s *ListTaskDefinitionsOutput) SetNextToken(v string) *ListTaskDefinitionsOutput {
	s.NextToken = &v
	return s
}

// SetTaskDefinitionArns sets the TaskDefinitionArns field's value.
func (s *ListTaskDefinitionsOutput) SetTaskDefinitionArns(v []*string) *ListTaskDefinitionsOutput {
	s.TaskDefinitionArns = v
	return s
}

type ListTasksInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	ContainerInstance *string `locationName:"containerInstance" type:"string"`

	DesiredStatus *string `locationName:"desiredStatus" type:"string" enum:"DesiredStatus"`

	Family *string `locationName:"family" type:"string"`

	LaunchType *string `locationName:"launchType" type:"string" enum:"LaunchType"`

	MaxResults *int64 `locationName:"maxResults" type:"integer"`

	NextToken *string `locationName:"nextToken" type:"string"`

	ServiceName *string `locationName:"serviceName" type:"string"`

	StartedBy *string `locationName:"startedBy" type:"string"`
}

// String returns the string representation
func (s ListTasksInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListTasksInput) GoString() string {
	return s.String()
}

// SetCluster sets the Cluster field's value.
func (s *ListTasksInput) SetCluster(v string) *ListTasksInput {
	s.Cluster = &v
	return s
}

// SetContainerInstance sets the ContainerInstance field's value.
func (s *ListTasksInput) SetContainerInstance(v string) *ListTasksInput {
	s.ContainerInstance = &v
	return s
}

// SetDesiredStatus sets the DesiredStatus field's value.
func (s *ListTasksInput) SetDesiredStatus(v string) *ListTasksInput {
	s.DesiredStatus = &v
	return s
}

// SetFamily sets the Family field's value.
func (s *ListTasksInput) SetFamily(v string) *ListTasksInput {
	s.Family = &v
	return s
}

// SetLaunchType sets the LaunchType field's value.
func (s *ListTasksInput) SetLaunchType(v string) *ListTasksInput {
	s.LaunchType = &v
	return s
}

// SetMaxResults sets the MaxResults field's value.
func (s *ListTasksInput) SetMaxResults(v int64) *ListTasksInput {
	s.MaxResults = &v
	return s
}

// SetNextToken sets the NextToken field's value.
func (s *ListTasksInput) SetNextToken(v string) *ListTasksInput {
	s.NextToken = &v
	return s
}

// SetServiceName sets the ServiceName field's value.
func (s *ListTasksInput) SetServiceName(v string) *ListTasksInput {
	s.ServiceName = &v
	return s
}

// SetStartedBy sets the StartedBy field's value.
func (s *ListTasksInput) SetStartedBy(v string) *ListTasksInput {
	s.StartedBy = &v
	return s
}

type ListTasksOutput struct {
	_ struct{} `type:"structure"`

	NextToken *string `locationName:"nextToken" type:"string"`

	TaskArns []*string `locationName:"taskArns" type:"list"`
}

// String returns the string representation
func (s ListTasksOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ListTasksOutput) GoString() string {
	return s.String()
}

// SetNextToken sets the NextToken field's value.
func (s *ListTasksOutput) SetNextToken(v string) *ListTasksOutput {
	s.NextToken = &v
	return s
}

// SetTaskArns sets the TaskArns field's value.
func (s *ListTasksOutput) SetTaskArns(v []*string) *ListTasksOutput {
	s.TaskArns = v
	return s
}

type LoadBalancer struct {
	_ struct{} `type:"structure"`

	ContainerName *string `locationName:"containerName" type:"string"`

	ContainerPort *int64 `locationName:"containerPort" type:"integer"`

	LoadBalancerName *string `locationName:"loadBalancerName" type:"string"`

	TargetGroupArn *string `locationName:"targetGroupArn" type:"string"`
}

// String returns the string representation
func (s LoadBalancer) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s LoadBalancer) GoString() string {
	return s.String()
}

// SetContainerName sets the ContainerName field's value.
func (s *LoadBalancer) SetContainerName(v string) *LoadBalancer {
	s.ContainerName = &v
	return s
}

// SetContainerPort sets the ContainerPort field's value.
func (s *LoadBalancer) SetContainerPort(v int64) *LoadBalancer {
	s.ContainerPort = &v
	return s
}

// SetLoadBalancerName sets the LoadBalancerName field's value.
func (s *LoadBalancer) SetLoadBalancerName(v string) *LoadBalancer {
	s.LoadBalancerName = &v
	return s
}

// SetTargetGroupArn sets the TargetGroupArn field's value.
func (s *LoadBalancer) SetTargetGroupArn(v string) *LoadBalancer {
	s.TargetGroupArn = &v
	return s
}

type LogConfiguration struct {
	_ struct{} `type:"structure"`

	// LogDriver is a required field
	LogDriver *string `locationName:"logDriver" type:"string" required:"true" enum:"LogDriver"`

	Options map[string]*string `locationName:"options" type:"map"`

	SecretOptions []*Secret `locationName:"secretOptions" type:"list"`
}

// String returns the string representation
func (s LogConfiguration) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s LogConfiguration) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *LogConfiguration) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "LogConfiguration"}
	if s.LogDriver == nil {
		invalidParams.Add(request.NewErrParamRequired("LogDriver"))
	}
	if s.SecretOptions != nil {
		for i, v := range s.SecretOptions {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "SecretOptions", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetLogDriver sets the LogDriver field's value.
func (s *LogConfiguration) SetLogDriver(v string) *LogConfiguration {
	s.LogDriver = &v
	return s
}

// SetOptions sets the Options field's value.
func (s *LogConfiguration) SetOptions(v map[string]*string) *LogConfiguration {
	s.Options = v
	return s
}

// SetSecretOptions sets the SecretOptions field's value.
func (s *LogConfiguration) SetSecretOptions(v []*Secret) *LogConfiguration {
	s.SecretOptions = v
	return s
}

type MountPoint struct {
	_ struct{} `type:"structure"`

	ContainerPath *string `locationName:"containerPath" type:"string"`

	ReadOnly *bool `locationName:"readOnly" type:"boolean"`

	SourceVolume *string `locationName:"sourceVolume" type:"string"`
}

// String returns the string representation
func (s MountPoint) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s MountPoint) GoString() string {
	return s.String()
}

// SetContainerPath sets the ContainerPath field's value.
func (s *MountPoint) SetContainerPath(v string) *MountPoint {
	s.ContainerPath = &v
	return s
}

// SetReadOnly sets the ReadOnly field's value.
func (s *MountPoint) SetReadOnly(v bool) *MountPoint {
	s.ReadOnly = &v
	return s
}

// SetSourceVolume sets the SourceVolume field's value.
func (s *MountPoint) SetSourceVolume(v string) *MountPoint {
	s.SourceVolume = &v
	return s
}

type NetworkBinding struct {
	_ struct{} `type:"structure"`

	BindIP *string `locationName:"bindIP" type:"string"`

	ContainerPort *int64 `locationName:"containerPort" type:"integer"`

	HostPort *int64 `locationName:"hostPort" type:"integer"`

	Protocol *string `locationName:"protocol" type:"string" enum:"TransportProtocol"`
}

// String returns the string representation
func (s NetworkBinding) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s NetworkBinding) GoString() string {
	return s.String()
}

// SetBindIP sets the BindIP field's value.
func (s *NetworkBinding) SetBindIP(v string) *NetworkBinding {
	s.BindIP = &v
	return s
}

// SetContainerPort sets the ContainerPort field's value.
func (s *NetworkBinding) SetContainerPort(v int64) *NetworkBinding {
	s.ContainerPort = &v
	return s
}

// SetHostPort sets the HostPort field's value.
func (s *NetworkBinding) SetHostPort(v int64) *NetworkBinding {
	s.HostPort = &v
	return s
}

// SetProtocol sets the Protocol field's value.
func (s *NetworkBinding) SetProtocol(v string) *NetworkBinding {
	s.Protocol = &v
	return s
}

type NetworkConfiguration struct {
	_ struct{} `type:"structure"`

	AwsvpcConfiguration *AwsVpcConfiguration `locationName:"awsvpcConfiguration" type:"structure"`
}

// String returns the string representation
func (s NetworkConfiguration) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s NetworkConfiguration) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *NetworkConfiguration) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "NetworkConfiguration"}
	if s.AwsvpcConfiguration != nil {
		if err := s.AwsvpcConfiguration.Validate(); err != nil {
			invalidParams.AddNested("AwsvpcConfiguration", err.(request.ErrInvalidParams))
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetAwsvpcConfiguration sets the AwsvpcConfiguration field's value.
func (s *NetworkConfiguration) SetAwsvpcConfiguration(v *AwsVpcConfiguration) *NetworkConfiguration {
	s.AwsvpcConfiguration = v
	return s
}

type NetworkInterface struct {
	_ struct{} `type:"structure"`

	AttachmentId *string `locationName:"attachmentId" type:"string"`

	Ipv6Address *string `locationName:"ipv6Address" type:"string"`

	PrivateIpv4Address *string `locationName:"privateIpv4Address" type:"string"`
}

// String returns the string representation
func (s NetworkInterface) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s NetworkInterface) GoString() string {
	return s.String()
}

// SetAttachmentId sets the AttachmentId field's value.
func (s *NetworkInterface) SetAttachmentId(v string) *NetworkInterface {
	s.AttachmentId = &v
	return s
}

// SetIpv6Address sets the Ipv6Address field's value.
func (s *NetworkInterface) SetIpv6Address(v string) *NetworkInterface {
	s.Ipv6Address = &v
	return s
}

// SetPrivateIpv4Address sets the PrivateIpv4Address field's value.
func (s *NetworkInterface) SetPrivateIpv4Address(v string) *NetworkInterface {
	s.PrivateIpv4Address = &v
	return s
}

type PlacementConstraint struct {
	_ struct{} `type:"structure"`

	Expression *string `locationName:"expression" type:"string"`

	Type *string `locationName:"type" type:"string" enum:"PlacementConstraintType"`
}

// String returns the string representation
func (s PlacementConstraint) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PlacementConstraint) GoString() string {
	return s.String()
}

// SetExpression sets the Expression field's value.
func (s *PlacementConstraint) SetExpression(v string) *PlacementConstraint {
	s.Expression = &v
	return s
}

// SetType sets the Type field's value.
func (s *PlacementConstraint) SetType(v string) *PlacementConstraint {
	s.Type = &v
	return s
}

type PlacementStrategy struct {
	_ struct{} `type:"structure"`

	Field *string `locationName:"field" type:"string"`

	Type *string `locationName:"type" type:"string" enum:"PlacementStrategyType"`
}

// String returns the string representation
func (s PlacementStrategy) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PlacementStrategy) GoString() string {
	return s.String()
}

// SetField sets the Field field's value.
func (s *PlacementStrategy) SetField(v string) *PlacementStrategy {
	s.Field = &v
	return s
}

// SetType sets the Type field's value.
func (s *PlacementStrategy) SetType(v string) *PlacementStrategy {
	s.Type = &v
	return s
}

type PlatformDevice struct {
	_ struct{} `type:"structure"`

	// Id is a required field
	Id *string `locationName:"id" type:"string" required:"true"`

	// Type is a required field
	Type *string `locationName:"type" type:"string" required:"true" enum:"PlatformDeviceType"`
}

// String returns the string representation
func (s PlatformDevice) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PlatformDevice) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *PlatformDevice) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "PlatformDevice"}
	if s.Id == nil {
		invalidParams.Add(request.NewErrParamRequired("Id"))
	}
	if s.Type == nil {
		invalidParams.Add(request.NewErrParamRequired("Type"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetId sets the Id field's value.
func (s *PlatformDevice) SetId(v string) *PlatformDevice {
	s.Id = &v
	return s
}

// SetType sets the Type field's value.
func (s *PlatformDevice) SetType(v string) *PlatformDevice {
	s.Type = &v
	return s
}

type PortMapping struct {
	_ struct{} `type:"structure"`

	ContainerPort *int64 `locationName:"containerPort" type:"integer"`

	HostPort *int64 `locationName:"hostPort" type:"integer"`

	Protocol *string `locationName:"protocol" type:"string" enum:"TransportProtocol"`
}

// String returns the string representation
func (s PortMapping) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PortMapping) GoString() string {
	return s.String()
}

// SetContainerPort sets the ContainerPort field's value.
func (s *PortMapping) SetContainerPort(v int64) *PortMapping {
	s.ContainerPort = &v
	return s
}

// SetHostPort sets the HostPort field's value.
func (s *PortMapping) SetHostPort(v int64) *PortMapping {
	s.HostPort = &v
	return s
}

// SetProtocol sets the Protocol field's value.
func (s *PortMapping) SetProtocol(v string) *PortMapping {
	s.Protocol = &v
	return s
}

type ProxyConfiguration struct {
	_ struct{} `type:"structure"`

	// ContainerName is a required field
	ContainerName *string `locationName:"containerName" type:"string" required:"true"`

	Properties []*KeyValuePair `locationName:"properties" type:"list"`

	Type *string `locationName:"type" type:"string" enum:"ProxyConfigurationType"`
}

// String returns the string representation
func (s ProxyConfiguration) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ProxyConfiguration) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *ProxyConfiguration) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "ProxyConfiguration"}
	if s.ContainerName == nil {
		invalidParams.Add(request.NewErrParamRequired("ContainerName"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetContainerName sets the ContainerName field's value.
func (s *ProxyConfiguration) SetContainerName(v string) *ProxyConfiguration {
	s.ContainerName = &v
	return s
}

// SetProperties sets the Properties field's value.
func (s *ProxyConfiguration) SetProperties(v []*KeyValuePair) *ProxyConfiguration {
	s.Properties = v
	return s
}

// SetType sets the Type field's value.
func (s *ProxyConfiguration) SetType(v string) *ProxyConfiguration {
	s.Type = &v
	return s
}

type PutAccountSettingDefaultInput struct {
	_ struct{} `type:"structure"`

	// Name is a required field
	Name *string `locationName:"name" type:"string" required:"true" enum:"SettingName"`

	// Value is a required field
	Value *string `locationName:"value" type:"string" required:"true"`
}

// String returns the string representation
func (s PutAccountSettingDefaultInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PutAccountSettingDefaultInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *PutAccountSettingDefaultInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "PutAccountSettingDefaultInput"}
	if s.Name == nil {
		invalidParams.Add(request.NewErrParamRequired("Name"))
	}
	if s.Value == nil {
		invalidParams.Add(request.NewErrParamRequired("Value"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetName sets the Name field's value.
func (s *PutAccountSettingDefaultInput) SetName(v string) *PutAccountSettingDefaultInput {
	s.Name = &v
	return s
}

// SetValue sets the Value field's value.
func (s *PutAccountSettingDefaultInput) SetValue(v string) *PutAccountSettingDefaultInput {
	s.Value = &v
	return s
}

type PutAccountSettingDefaultOutput struct {
	_ struct{} `type:"structure"`

	Setting *Setting `locationName:"setting" type:"structure"`
}

// String returns the string representation
func (s PutAccountSettingDefaultOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PutAccountSettingDefaultOutput) GoString() string {
	return s.String()
}

// SetSetting sets the Setting field's value.
func (s *PutAccountSettingDefaultOutput) SetSetting(v *Setting) *PutAccountSettingDefaultOutput {
	s.Setting = v
	return s
}

type PutAccountSettingInput struct {
	_ struct{} `type:"structure"`

	// Name is a required field
	Name *string `locationName:"name" type:"string" required:"true" enum:"SettingName"`

	PrincipalArn *string `locationName:"principalArn" type:"string"`

	// Value is a required field
	Value *string `locationName:"value" type:"string" required:"true"`
}

// String returns the string representation
func (s PutAccountSettingInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PutAccountSettingInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *PutAccountSettingInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "PutAccountSettingInput"}
	if s.Name == nil {
		invalidParams.Add(request.NewErrParamRequired("Name"))
	}
	if s.Value == nil {
		invalidParams.Add(request.NewErrParamRequired("Value"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetName sets the Name field's value.
func (s *PutAccountSettingInput) SetName(v string) *PutAccountSettingInput {
	s.Name = &v
	return s
}

// SetPrincipalArn sets the PrincipalArn field's value.
func (s *PutAccountSettingInput) SetPrincipalArn(v string) *PutAccountSettingInput {
	s.PrincipalArn = &v
	return s
}

// SetValue sets the Value field's value.
func (s *PutAccountSettingInput) SetValue(v string) *PutAccountSettingInput {
	s.Value = &v
	return s
}

type PutAccountSettingOutput struct {
	_ struct{} `type:"structure"`

	Setting *Setting `locationName:"setting" type:"structure"`
}

// String returns the string representation
func (s PutAccountSettingOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PutAccountSettingOutput) GoString() string {
	return s.String()
}

// SetSetting sets the Setting field's value.
func (s *PutAccountSettingOutput) SetSetting(v *Setting) *PutAccountSettingOutput {
	s.Setting = v
	return s
}

type PutAttributesInput struct {
	_ struct{} `type:"structure"`

	// Attributes is a required field
	Attributes []*Attribute `locationName:"attributes" type:"list" required:"true"`

	Cluster *string `locationName:"cluster" type:"string"`
}

// String returns the string representation
func (s PutAttributesInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PutAttributesInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *PutAttributesInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "PutAttributesInput"}
	if s.Attributes == nil {
		invalidParams.Add(request.NewErrParamRequired("Attributes"))
	}
	if s.Attributes != nil {
		for i, v := range s.Attributes {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Attributes", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetAttributes sets the Attributes field's value.
func (s *PutAttributesInput) SetAttributes(v []*Attribute) *PutAttributesInput {
	s.Attributes = v
	return s
}

// SetCluster sets the Cluster field's value.
func (s *PutAttributesInput) SetCluster(v string) *PutAttributesInput {
	s.Cluster = &v
	return s
}

type PutAttributesOutput struct {
	_ struct{} `type:"structure"`

	Attributes []*Attribute `locationName:"attributes" type:"list"`
}

// String returns the string representation
func (s PutAttributesOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PutAttributesOutput) GoString() string {
	return s.String()
}

// SetAttributes sets the Attributes field's value.
func (s *PutAttributesOutput) SetAttributes(v []*Attribute) *PutAttributesOutput {
	s.Attributes = v
	return s
}

type RegisterContainerInstanceInput struct {
	_ struct{} `type:"structure"`

	Attributes []*Attribute `locationName:"attributes" type:"list"`

	ClientToken *string `locationName:"clientToken" type:"string"`

	Cluster *string `locationName:"cluster" type:"string"`

	ContainerInstanceArn *string `locationName:"containerInstanceArn" type:"string"`

	InstanceIdentityDocument *string `locationName:"instanceIdentityDocument" type:"string"`

	InstanceIdentityDocumentSignature *string `locationName:"instanceIdentityDocumentSignature" type:"string"`

	PlatformDevices []*PlatformDevice `locationName:"platformDevices" type:"list"`

	Tags []*Tag `locationName:"tags" type:"list"`

	TotalResources []*Resource `locationName:"totalResources" type:"list"`

	VersionInfo *VersionInfo `locationName:"versionInfo" type:"structure"`
}

// String returns the string representation
func (s RegisterContainerInstanceInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s RegisterContainerInstanceInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *RegisterContainerInstanceInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "RegisterContainerInstanceInput"}
	if s.Attributes != nil {
		for i, v := range s.Attributes {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Attributes", i), err.(request.ErrInvalidParams))
			}
		}
	}
	if s.PlatformDevices != nil {
		for i, v := range s.PlatformDevices {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "PlatformDevices", i), err.(request.ErrInvalidParams))
			}
		}
	}
	if s.Tags != nil {
		for i, v := range s.Tags {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Tags", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetAttributes sets the Attributes field's value.
func (s *RegisterContainerInstanceInput) SetAttributes(v []*Attribute) *RegisterContainerInstanceInput {
	s.Attributes = v
	return s
}

// SetClientToken sets the ClientToken field's value.
func (s *RegisterContainerInstanceInput) SetClientToken(v string) *RegisterContainerInstanceInput {
	s.ClientToken = &v
	return s
}

// SetCluster sets the Cluster field's value.
func (s *RegisterContainerInstanceInput) SetCluster(v string) *RegisterContainerInstanceInput {
	s.Cluster = &v
	return s
}

// SetContainerInstanceArn sets the ContainerInstanceArn field's value.
func (s *RegisterContainerInstanceInput) SetContainerInstanceArn(v string) *RegisterContainerInstanceInput {
	s.ContainerInstanceArn = &v
	return s
}

// SetInstanceIdentityDocument sets the InstanceIdentityDocument field's value.
func (s *RegisterContainerInstanceInput) SetInstanceIdentityDocument(v string) *RegisterContainerInstanceInput {
	s.InstanceIdentityDocument = &v
	return s
}

// SetInstanceIdentityDocumentSignature sets the InstanceIdentityDocumentSignature field's value.
func (s *RegisterContainerInstanceInput) SetInstanceIdentityDocumentSignature(v string) *RegisterContainerInstanceInput {
	s.InstanceIdentityDocumentSignature = &v
	return s
}

// SetPlatformDevices sets the PlatformDevices field's value.
func (s *RegisterContainerInstanceInput) SetPlatformDevices(v []*PlatformDevice) *RegisterContainerInstanceInput {
	s.PlatformDevices = v
	return s
}

// SetTags sets the Tags field's value.
func (s *RegisterContainerInstanceInput) SetTags(v []*Tag) *RegisterContainerInstanceInput {
	s.Tags = v
	return s
}

// SetTotalResources sets the TotalResources field's value.
func (s *RegisterContainerInstanceInput) SetTotalResources(v []*Resource) *RegisterContainerInstanceInput {
	s.TotalResources = v
	return s
}

// SetVersionInfo sets the VersionInfo field's value.
func (s *RegisterContainerInstanceInput) SetVersionInfo(v *VersionInfo) *RegisterContainerInstanceInput {
	s.VersionInfo = v
	return s
}

type RegisterContainerInstanceOutput struct {
	_ struct{} `type:"structure"`

	ContainerInstance *ContainerInstance `locationName:"containerInstance" type:"structure"`
}

// String returns the string representation
func (s RegisterContainerInstanceOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s RegisterContainerInstanceOutput) GoString() string {
	return s.String()
}

// SetContainerInstance sets the ContainerInstance field's value.
func (s *RegisterContainerInstanceOutput) SetContainerInstance(v *ContainerInstance) *RegisterContainerInstanceOutput {
	s.ContainerInstance = v
	return s
}

type RegisterTaskDefinitionInput struct {
	_ struct{} `type:"structure"`

	// ContainerDefinitions is a required field
	ContainerDefinitions []*ContainerDefinition `locationName:"containerDefinitions" type:"list" required:"true"`

	Cpu *string `locationName:"cpu" type:"string"`

	ExecutionRoleArn *string `locationName:"executionRoleArn" type:"string"`

	// Family is a required field
	Family *string `locationName:"family" type:"string" required:"true"`

	InferenceAccelerators []*InferenceAccelerator `locationName:"inferenceAccelerators" type:"list"`

	IpcMode *string `locationName:"ipcMode" type:"string" enum:"IpcMode"`

	Memory *string `locationName:"memory" type:"string"`

	NetworkMode *string `locationName:"networkMode" type:"string" enum:"NetworkMode"`

	PidMode *string `locationName:"pidMode" type:"string" enum:"PidMode"`

	PlacementConstraints []*TaskDefinitionPlacementConstraint `locationName:"placementConstraints" type:"list"`

	ProxyConfiguration *ProxyConfiguration `locationName:"proxyConfiguration" type:"structure"`

	RequiresCompatibilities []*string `locationName:"requiresCompatibilities" type:"list"`

	Tags []*Tag `locationName:"tags" type:"list"`

	TaskRoleArn *string `locationName:"taskRoleArn" type:"string"`

	Volumes []*Volume `locationName:"volumes" type:"list"`
}

// String returns the string representation
func (s RegisterTaskDefinitionInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s RegisterTaskDefinitionInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *RegisterTaskDefinitionInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "RegisterTaskDefinitionInput"}
	if s.ContainerDefinitions == nil {
		invalidParams.Add(request.NewErrParamRequired("ContainerDefinitions"))
	}
	if s.Family == nil {
		invalidParams.Add(request.NewErrParamRequired("Family"))
	}
	if s.ContainerDefinitions != nil {
		for i, v := range s.ContainerDefinitions {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "ContainerDefinitions", i), err.(request.ErrInvalidParams))
			}
		}
	}
	if s.InferenceAccelerators != nil {
		for i, v := range s.InferenceAccelerators {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "InferenceAccelerators", i), err.(request.ErrInvalidParams))
			}
		}
	}
	if s.ProxyConfiguration != nil {
		if err := s.ProxyConfiguration.Validate(); err != nil {
			invalidParams.AddNested("ProxyConfiguration", err.(request.ErrInvalidParams))
		}
	}
	if s.Tags != nil {
		for i, v := range s.Tags {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Tags", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetContainerDefinitions sets the ContainerDefinitions field's value.
func (s *RegisterTaskDefinitionInput) SetContainerDefinitions(v []*ContainerDefinition) *RegisterTaskDefinitionInput {
	s.ContainerDefinitions = v
	return s
}

// SetCpu sets the Cpu field's value.
func (s *RegisterTaskDefinitionInput) SetCpu(v string) *RegisterTaskDefinitionInput {
	s.Cpu = &v
	return s
}

// SetExecutionRoleArn sets the ExecutionRoleArn field's value.
func (s *RegisterTaskDefinitionInput) SetExecutionRoleArn(v string) *RegisterTaskDefinitionInput {
	s.ExecutionRoleArn = &v
	return s
}

// SetFamily sets the Family field's value.
func (s *RegisterTaskDefinitionInput) SetFamily(v string) *RegisterTaskDefinitionInput {
	s.Family = &v
	return s
}

// SetInferenceAccelerators sets the InferenceAccelerators field's value.
func (s *RegisterTaskDefinitionInput) SetInferenceAccelerators(v []*InferenceAccelerator) *RegisterTaskDefinitionInput {
	s.InferenceAccelerators = v
	return s
}

// SetIpcMode sets the IpcMode field's value.
func (s *RegisterTaskDefinitionInput) SetIpcMode(v string) *RegisterTaskDefinitionInput {
	s.IpcMode = &v
	return s
}

// SetMemory sets the Memory field's value.
func (s *RegisterTaskDefinitionInput) SetMemory(v string) *RegisterTaskDefinitionInput {
	s.Memory = &v
	return s
}

// SetNetworkMode sets the NetworkMode field's value.
func (s *RegisterTaskDefinitionInput) SetNetworkMode(v string) *RegisterTaskDefinitionInput {
	s.NetworkMode = &v
	return s
}

// SetPidMode sets the PidMode field's value.
func (s *RegisterTaskDefinitionInput) SetPidMode(v string) *RegisterTaskDefinitionInput {
	s.PidMode = &v
	return s
}

// SetPlacementConstraints sets the PlacementConstraints field's value.
func (s *RegisterTaskDefinitionInput) SetPlacementConstraints(v []*TaskDefinitionPlacementConstraint) *RegisterTaskDefinitionInput {
	s.PlacementConstraints = v
	return s
}

// SetProxyConfiguration sets the ProxyConfiguration field's value.
func (s *RegisterTaskDefinitionInput) SetProxyConfiguration(v *ProxyConfiguration) *RegisterTaskDefinitionInput {
	s.ProxyConfiguration = v
	return s
}

// SetRequiresCompatibilities sets the RequiresCompatibilities field's value.
func (s *RegisterTaskDefinitionInput) SetRequiresCompatibilities(v []*string) *RegisterTaskDefinitionInput {
	s.RequiresCompatibilities = v
	return s
}

// SetTags sets the Tags field's value.
func (s *RegisterTaskDefinitionInput) SetTags(v []*Tag) *RegisterTaskDefinitionInput {
	s.Tags = v
	return s
}

// SetTaskRoleArn sets the TaskRoleArn field's value.
func (s *RegisterTaskDefinitionInput) SetTaskRoleArn(v string) *RegisterTaskDefinitionInput {
	s.TaskRoleArn = &v
	return s
}

// SetVolumes sets the Volumes field's value.
func (s *RegisterTaskDefinitionInput) SetVolumes(v []*Volume) *RegisterTaskDefinitionInput {
	s.Volumes = v
	return s
}

type RegisterTaskDefinitionOutput struct {
	_ struct{} `type:"structure"`

	Tags []*Tag `locationName:"tags" type:"list"`

	TaskDefinition *TaskDefinition `locationName:"taskDefinition" type:"structure"`
}

// String returns the string representation
func (s RegisterTaskDefinitionOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s RegisterTaskDefinitionOutput) GoString() string {
	return s.String()
}

// SetTags sets the Tags field's value.
func (s *RegisterTaskDefinitionOutput) SetTags(v []*Tag) *RegisterTaskDefinitionOutput {
	s.Tags = v
	return s
}

// SetTaskDefinition sets the TaskDefinition field's value.
func (s *RegisterTaskDefinitionOutput) SetTaskDefinition(v *TaskDefinition) *RegisterTaskDefinitionOutput {
	s.TaskDefinition = v
	return s
}

type RepositoryCredentials struct {
	_ struct{} `type:"structure"`

	// CredentialsParameter is a required field
	CredentialsParameter *string `locationName:"credentialsParameter" type:"string" required:"true"`
}

// String returns the string representation
func (s RepositoryCredentials) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s RepositoryCredentials) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *RepositoryCredentials) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "RepositoryCredentials"}
	if s.CredentialsParameter == nil {
		invalidParams.Add(request.NewErrParamRequired("CredentialsParameter"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCredentialsParameter sets the CredentialsParameter field's value.
func (s *RepositoryCredentials) SetCredentialsParameter(v string) *RepositoryCredentials {
	s.CredentialsParameter = &v
	return s
}

type Resource struct {
	_ struct{} `type:"structure"`

	DoubleValue *float64 `locationName:"doubleValue" type:"double"`

	IntegerValue *int64 `locationName:"integerValue" type:"integer"`

	LongValue *int64 `locationName:"longValue" type:"long"`

	Name *string `locationName:"name" type:"string"`

	StringSetValue []*string `locationName:"stringSetValue" type:"list"`

	Type *string `locationName:"type" type:"string"`
}

// String returns the string representation
func (s Resource) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Resource) GoString() string {
	return s.String()
}

// SetDoubleValue sets the DoubleValue field's value.
func (s *Resource) SetDoubleValue(v float64) *Resource {
	s.DoubleValue = &v
	return s
}

// SetIntegerValue sets the IntegerValue field's value.
func (s *Resource) SetIntegerValue(v int64) *Resource {
	s.IntegerValue = &v
	return s
}

// SetLongValue sets the LongValue field's value.
func (s *Resource) SetLongValue(v int64) *Resource {
	s.LongValue = &v
	return s
}

// SetName sets the Name field's value.
func (s *Resource) SetName(v string) *Resource {
	s.Name = &v
	return s
}

// SetStringSetValue sets the StringSetValue field's value.
func (s *Resource) SetStringSetValue(v []*string) *Resource {
	s.StringSetValue = v
	return s
}

// SetType sets the Type field's value.
func (s *Resource) SetType(v string) *Resource {
	s.Type = &v
	return s
}

type ResourceRequirement struct {
	_ struct{} `type:"structure"`

	// Type is a required field
	Type *string `locationName:"type" type:"string" required:"true" enum:"ResourceType"`

	// Value is a required field
	Value *string `locationName:"value" type:"string" required:"true"`
}

// String returns the string representation
func (s ResourceRequirement) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ResourceRequirement) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *ResourceRequirement) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "ResourceRequirement"}
	if s.Type == nil {
		invalidParams.Add(request.NewErrParamRequired("Type"))
	}
	if s.Value == nil {
		invalidParams.Add(request.NewErrParamRequired("Value"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetType sets the Type field's value.
func (s *ResourceRequirement) SetType(v string) *ResourceRequirement {
	s.Type = &v
	return s
}

// SetValue sets the Value field's value.
func (s *ResourceRequirement) SetValue(v string) *ResourceRequirement {
	s.Value = &v
	return s
}

type RunTaskInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	Count *int64 `locationName:"count" type:"integer"`

	EnableECSManagedTags *bool `locationName:"enableECSManagedTags" type:"boolean"`

	Group *string `locationName:"group" type:"string"`

	LaunchType *string `locationName:"launchType" type:"string" enum:"LaunchType"`

	NetworkConfiguration *NetworkConfiguration `locationName:"networkConfiguration" type:"structure"`

	Overrides *TaskOverride `locationName:"overrides" type:"structure"`

	PlacementConstraints []*PlacementConstraint `locationName:"placementConstraints" type:"list"`

	PlacementStrategy []*PlacementStrategy `locationName:"placementStrategy" type:"list"`

	PlatformVersion *string `locationName:"platformVersion" type:"string"`

	PropagateTags *string `locationName:"propagateTags" type:"string" enum:"PropagateTags"`

	StartedBy *string `locationName:"startedBy" type:"string"`

	Tags []*Tag `locationName:"tags" type:"list"`

	// TaskDefinition is a required field
	TaskDefinition *string `locationName:"taskDefinition" type:"string" required:"true"`
}

// String returns the string representation
func (s RunTaskInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s RunTaskInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *RunTaskInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "RunTaskInput"}
	if s.TaskDefinition == nil {
		invalidParams.Add(request.NewErrParamRequired("TaskDefinition"))
	}
	if s.NetworkConfiguration != nil {
		if err := s.NetworkConfiguration.Validate(); err != nil {
			invalidParams.AddNested("NetworkConfiguration", err.(request.ErrInvalidParams))
		}
	}
	if s.Overrides != nil {
		if err := s.Overrides.Validate(); err != nil {
			invalidParams.AddNested("Overrides", err.(request.ErrInvalidParams))
		}
	}
	if s.Tags != nil {
		for i, v := range s.Tags {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Tags", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCluster sets the Cluster field's value.
func (s *RunTaskInput) SetCluster(v string) *RunTaskInput {
	s.Cluster = &v
	return s
}

// SetCount sets the Count field's value.
func (s *RunTaskInput) SetCount(v int64) *RunTaskInput {
	s.Count = &v
	return s
}

// SetEnableECSManagedTags sets the EnableECSManagedTags field's value.
func (s *RunTaskInput) SetEnableECSManagedTags(v bool) *RunTaskInput {
	s.EnableECSManagedTags = &v
	return s
}

// SetGroup sets the Group field's value.
func (s *RunTaskInput) SetGroup(v string) *RunTaskInput {
	s.Group = &v
	return s
}

// SetLaunchType sets the LaunchType field's value.
func (s *RunTaskInput) SetLaunchType(v string) *RunTaskInput {
	s.LaunchType = &v
	return s
}

// SetNetworkConfiguration sets the NetworkConfiguration field's value.
func (s *RunTaskInput) SetNetworkConfiguration(v *NetworkConfiguration) *RunTaskInput {
	s.NetworkConfiguration = v
	return s
}

// SetOverrides sets the Overrides field's value.
func (s *RunTaskInput) SetOverrides(v *TaskOverride) *RunTaskInput {
	s.Overrides = v
	return s
}

// SetPlacementConstraints sets the PlacementConstraints field's value.
func (s *RunTaskInput) SetPlacementConstraints(v []*PlacementConstraint) *RunTaskInput {
	s.PlacementConstraints = v
	return s
}

// SetPlacementStrategy sets the PlacementStrategy field's value.
func (s *RunTaskInput) SetPlacementStrategy(v []*PlacementStrategy) *RunTaskInput {
	s.PlacementStrategy = v
	return s
}

// SetPlatformVersion sets the PlatformVersion field's value.
func (s *RunTaskInput) SetPlatformVersion(v string) *RunTaskInput {
	s.PlatformVersion = &v
	return s
}

// SetPropagateTags sets the PropagateTags field's value.
func (s *RunTaskInput) SetPropagateTags(v string) *RunTaskInput {
	s.PropagateTags = &v
	return s
}

// SetStartedBy sets the StartedBy field's value.
func (s *RunTaskInput) SetStartedBy(v string) *RunTaskInput {
	s.StartedBy = &v
	return s
}

// SetTags sets the Tags field's value.
func (s *RunTaskInput) SetTags(v []*Tag) *RunTaskInput {
	s.Tags = v
	return s
}

// SetTaskDefinition sets the TaskDefinition field's value.
func (s *RunTaskInput) SetTaskDefinition(v string) *RunTaskInput {
	s.TaskDefinition = &v
	return s
}

type RunTaskOutput struct {
	_ struct{} `type:"structure"`

	Failures []*Failure `locationName:"failures" type:"list"`

	Tasks []*Task `locationName:"tasks" type:"list"`
}

// String returns the string representation
func (s RunTaskOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s RunTaskOutput) GoString() string {
	return s.String()
}

// SetFailures sets the Failures field's value.
func (s *RunTaskOutput) SetFailures(v []*Failure) *RunTaskOutput {
	s.Failures = v
	return s
}

// SetTasks sets the Tasks field's value.
func (s *RunTaskOutput) SetTasks(v []*Task) *RunTaskOutput {
	s.Tasks = v
	return s
}

type Scale struct {
	_ struct{} `type:"structure"`

	Unit *string `locationName:"unit" type:"string" enum:"ScaleUnit"`

	Value *float64 `locationName:"value" type:"double"`
}

// String returns the string representation
func (s Scale) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Scale) GoString() string {
	return s.String()
}

// SetUnit sets the Unit field's value.
func (s *Scale) SetUnit(v string) *Scale {
	s.Unit = &v
	return s
}

// SetValue sets the Value field's value.
func (s *Scale) SetValue(v float64) *Scale {
	s.Value = &v
	return s
}

type Secret struct {
	_ struct{} `type:"structure"`

	// Name is a required field
	Name *string `locationName:"name" type:"string" required:"true"`

	// ValueFrom is a required field
	ValueFrom *string `locationName:"valueFrom" type:"string" required:"true"`
}

// String returns the string representation
func (s Secret) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Secret) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *Secret) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "Secret"}
	if s.Name == nil {
		invalidParams.Add(request.NewErrParamRequired("Name"))
	}
	if s.ValueFrom == nil {
		invalidParams.Add(request.NewErrParamRequired("ValueFrom"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetName sets the Name field's value.
func (s *Secret) SetName(v string) *Secret {
	s.Name = &v
	return s
}

// SetValueFrom sets the ValueFrom field's value.
func (s *Secret) SetValueFrom(v string) *Secret {
	s.ValueFrom = &v
	return s
}

type Service struct {
	_ struct{} `type:"structure"`

	ClusterArn *string `locationName:"clusterArn" type:"string"`

	CreatedAt *time.Time `locationName:"createdAt" type:"timestamp"`

	CreatedBy *string `locationName:"createdBy" type:"string"`

	DeploymentConfiguration *DeploymentConfiguration `locationName:"deploymentConfiguration" type:"structure"`

	DeploymentController *DeploymentController `locationName:"deploymentController" type:"structure"`

	Deployments []*Deployment `locationName:"deployments" type:"list"`

	DesiredCount *int64 `locationName:"desiredCount" type:"integer"`

	EnableECSManagedTags *bool `locationName:"enableECSManagedTags" type:"boolean"`

	Events []*ServiceEvent `locationName:"events" type:"list"`

	HealthCheckGracePeriodSeconds *int64 `locationName:"healthCheckGracePeriodSeconds" type:"integer"`

	LaunchType *string `locationName:"launchType" type:"string" enum:"LaunchType"`

	LoadBalancers []*LoadBalancer `locationName:"loadBalancers" type:"list"`

	NetworkConfiguration *NetworkConfiguration `locationName:"networkConfiguration" type:"structure"`

	PendingCount *int64 `locationName:"pendingCount" type:"integer"`

	PlacementConstraints []*PlacementConstraint `locationName:"placementConstraints" type:"list"`

	PlacementStrategy []*PlacementStrategy `locationName:"placementStrategy" type:"list"`

	PlatformVersion *string `locationName:"platformVersion" type:"string"`

	PropagateTags *string `locationName:"propagateTags" type:"string" enum:"PropagateTags"`

	RoleArn *string `locationName:"roleArn" type:"string"`

	RunningCount *int64 `locationName:"runningCount" type:"integer"`

	SchedulingStrategy *string `locationName:"schedulingStrategy" type:"string" enum:"SchedulingStrategy"`

	ServiceArn *string `locationName:"serviceArn" type:"string"`

	ServiceName *string `locationName:"serviceName" type:"string"`

	ServiceRegistries []*ServiceRegistry `locationName:"serviceRegistries" type:"list"`

	Status *string `locationName:"status" type:"string"`

	Tags []*Tag `locationName:"tags" type:"list"`

	TaskDefinition *string `locationName:"taskDefinition" type:"string"`

	TaskSets []*TaskSet `locationName:"taskSets" type:"list"`

	Version *int64 `locationName:"version" type:"long"`
}

// String returns the string representation
func (s Service) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Service) GoString() string {
	return s.String()
}

// SetClusterArn sets the ClusterArn field's value.
func (s *Service) SetClusterArn(v string) *Service {
	s.ClusterArn = &v
	return s
}

// SetCreatedAt sets the CreatedAt field's value.
func (s *Service) SetCreatedAt(v time.Time) *Service {
	s.CreatedAt = &v
	return s
}

// SetCreatedBy sets the CreatedBy field's value.
func (s *Service) SetCreatedBy(v string) *Service {
	s.CreatedBy = &v
	return s
}

// SetDeploymentConfiguration sets the DeploymentConfiguration field's value.
func (s *Service) SetDeploymentConfiguration(v *DeploymentConfiguration) *Service {
	s.DeploymentConfiguration = v
	return s
}

// SetDeploymentController sets the DeploymentController field's value.
func (s *Service) SetDeploymentController(v *DeploymentController) *Service {
	s.DeploymentController = v
	return s
}

// SetDeployments sets the Deployments field's value.
func (s *Service) SetDeployments(v []*Deployment) *Service {
	s.Deployments = v
	return s
}

// SetDesiredCount sets the DesiredCount field's value.
func (s *Service) SetDesiredCount(v int64) *Service {
	s.DesiredCount = &v
	return s
}

// SetEnableECSManagedTags sets the EnableECSManagedTags field's value.
func (s *Service) SetEnableECSManagedTags(v bool) *Service {
	s.EnableECSManagedTags = &v
	return s
}

// SetEvents sets the Events field's value.
func (s *Service) SetEvents(v []*ServiceEvent) *Service {
	s.Events = v
	return s
}

// SetHealthCheckGracePeriodSeconds sets the HealthCheckGracePeriodSeconds field's value.
func (s *Service) SetHealthCheckGracePeriodSeconds(v int64) *Service {
	s.HealthCheckGracePeriodSeconds = &v
	return s
}

// SetLaunchType sets the LaunchType field's value.
func (s *Service) SetLaunchType(v string) *Service {
	s.LaunchType = &v
	return s
}

// SetLoadBalancers sets the LoadBalancers field's value.
func (s *Service) SetLoadBalancers(v []*LoadBalancer) *Service {
	s.LoadBalancers = v
	return s
}

// SetNetworkConfiguration sets the NetworkConfiguration field's value.
func (s *Service) SetNetworkConfiguration(v *NetworkConfiguration) *Service {
	s.NetworkConfiguration = v
	return s
}

// SetPendingCount sets the PendingCount field's value.
func (s *Service) SetPendingCount(v int64) *Service {
	s.PendingCount = &v
	return s
}

// SetPlacementConstraints sets the PlacementConstraints field's value.
func (s *Service) SetPlacementConstraints(v []*PlacementConstraint) *Service {
	s.PlacementConstraints = v
	return s
}

// SetPlacementStrategy sets the PlacementStrategy field's value.
func (s *Service) SetPlacementStrategy(v []*PlacementStrategy) *Service {
	s.PlacementStrategy = v
	return s
}

// SetPlatformVersion sets the PlatformVersion field's value.
func (s *Service) SetPlatformVersion(v string) *Service {
	s.PlatformVersion = &v
	return s
}

// SetPropagateTags sets the PropagateTags field's value.
func (s *Service) SetPropagateTags(v string) *Service {
	s.PropagateTags = &v
	return s
}

// SetRoleArn sets the RoleArn field's value.
func (s *Service) SetRoleArn(v string) *Service {
	s.RoleArn = &v
	return s
}

// SetRunningCount sets the RunningCount field's value.
func (s *Service) SetRunningCount(v int64) *Service {
	s.RunningCount = &v
	return s
}

// SetSchedulingStrategy sets the SchedulingStrategy field's value.
func (s *Service) SetSchedulingStrategy(v string) *Service {
	s.SchedulingStrategy = &v
	return s
}

// SetServiceArn sets the ServiceArn field's value.
func (s *Service) SetServiceArn(v string) *Service {
	s.ServiceArn = &v
	return s
}

// SetServiceName sets the ServiceName field's value.
func (s *Service) SetServiceName(v string) *Service {
	s.ServiceName = &v
	return s
}

// SetServiceRegistries sets the ServiceRegistries field's value.
func (s *Service) SetServiceRegistries(v []*ServiceRegistry) *Service {
	s.ServiceRegistries = v
	return s
}

// SetStatus sets the Status field's value.
func (s *Service) SetStatus(v string) *Service {
	s.Status = &v
	return s
}

// SetTags sets the Tags field's value.
func (s *Service) SetTags(v []*Tag) *Service {
	s.Tags = v
	return s
}

// SetTaskDefinition sets the TaskDefinition field's value.
func (s *Service) SetTaskDefinition(v string) *Service {
	s.TaskDefinition = &v
	return s
}

// SetTaskSets sets the TaskSets field's value.
func (s *Service) SetTaskSets(v []*TaskSet) *Service {
	s.TaskSets = v
	return s
}

// SetVersion sets the Version field's value.
func (s *Service) SetVersion(v int64) *Service {
	s.Version = &v
	return s
}

type ServiceEvent struct {
	_ struct{} `type:"structure"`

	CreatedAt *time.Time `locationName:"createdAt" type:"timestamp"`

	Id *string `locationName:"id" type:"string"`

	Message *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s ServiceEvent) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ServiceEvent) GoString() string {
	return s.String()
}

// SetCreatedAt sets the CreatedAt field's value.
func (s *ServiceEvent) SetCreatedAt(v time.Time) *ServiceEvent {
	s.CreatedAt = &v
	return s
}

// SetId sets the Id field's value.
func (s *ServiceEvent) SetId(v string) *ServiceEvent {
	s.Id = &v
	return s
}

// SetMessage sets the Message field's value.
func (s *ServiceEvent) SetMessage(v string) *ServiceEvent {
	s.Message = &v
	return s
}

type ServiceRegistry struct {
	_ struct{} `type:"structure"`

	ContainerName *string `locationName:"containerName" type:"string"`

	ContainerPort *int64 `locationName:"containerPort" type:"integer"`

	Port *int64 `locationName:"port" type:"integer"`

	RegistryArn *string `locationName:"registryArn" type:"string"`
}

// String returns the string representation
func (s ServiceRegistry) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ServiceRegistry) GoString() string {
	return s.String()
}

// SetContainerName sets the ContainerName field's value.
func (s *ServiceRegistry) SetContainerName(v string) *ServiceRegistry {
	s.ContainerName = &v
	return s
}

// SetContainerPort sets the ContainerPort field's value.
func (s *ServiceRegistry) SetContainerPort(v int64) *ServiceRegistry {
	s.ContainerPort = &v
	return s
}

// SetPort sets the Port field's value.
func (s *ServiceRegistry) SetPort(v int64) *ServiceRegistry {
	s.Port = &v
	return s
}

// SetRegistryArn sets the RegistryArn field's value.
func (s *ServiceRegistry) SetRegistryArn(v string) *ServiceRegistry {
	s.RegistryArn = &v
	return s
}

type Setting struct {
	_ struct{} `type:"structure"`

	Name *string `locationName:"name" type:"string" enum:"SettingName"`

	PrincipalArn *string `locationName:"principalArn" type:"string"`

	Value *string `locationName:"value" type:"string"`
}

// String returns the string representation
func (s Setting) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Setting) GoString() string {
	return s.String()
}

// SetName sets the Name field's value.
func (s *Setting) SetName(v string) *Setting {
	s.Name = &v
	return s
}

// SetPrincipalArn sets the PrincipalArn field's value.
func (s *Setting) SetPrincipalArn(v string) *Setting {
	s.PrincipalArn = &v
	return s
}

// SetValue sets the Value field's value.
func (s *Setting) SetValue(v string) *Setting {
	s.Value = &v
	return s
}

type StartTaskInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	// ContainerInstances is a required field
	ContainerInstances []*string `locationName:"containerInstances" type:"list" required:"true"`

	EnableECSManagedTags *bool `locationName:"enableECSManagedTags" type:"boolean"`

	Group *string `locationName:"group" type:"string"`

	NetworkConfiguration *NetworkConfiguration `locationName:"networkConfiguration" type:"structure"`

	Overrides *TaskOverride `locationName:"overrides" type:"structure"`

	PropagateTags *string `locationName:"propagateTags" type:"string" enum:"PropagateTags"`

	Properties []*KeyValuePair `locationName:"properties" type:"list"`

	StartedBy *string `locationName:"startedBy" type:"string"`

	Tags []*Tag `locationName:"tags" type:"list"`

	// TaskDefinition is a required field
	TaskDefinition *string `locationName:"taskDefinition" type:"string" required:"true"`
}

// String returns the string representation
func (s StartTaskInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s StartTaskInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *StartTaskInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "StartTaskInput"}
	if s.ContainerInstances == nil {
		invalidParams.Add(request.NewErrParamRequired("ContainerInstances"))
	}
	if s.TaskDefinition == nil {
		invalidParams.Add(request.NewErrParamRequired("TaskDefinition"))
	}
	if s.NetworkConfiguration != nil {
		if err := s.NetworkConfiguration.Validate(); err != nil {
			invalidParams.AddNested("NetworkConfiguration", err.(request.ErrInvalidParams))
		}
	}
	if s.Overrides != nil {
		if err := s.Overrides.Validate(); err != nil {
			invalidParams.AddNested("Overrides", err.(request.ErrInvalidParams))
		}
	}
	if s.Tags != nil {
		for i, v := range s.Tags {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Tags", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCluster sets the Cluster field's value.
func (s *StartTaskInput) SetCluster(v string) *StartTaskInput {
	s.Cluster = &v
	return s
}

// SetContainerInstances sets the ContainerInstances field's value.
func (s *StartTaskInput) SetContainerInstances(v []*string) *StartTaskInput {
	s.ContainerInstances = v
	return s
}

// SetEnableECSManagedTags sets the EnableECSManagedTags field's value.
func (s *StartTaskInput) SetEnableECSManagedTags(v bool) *StartTaskInput {
	s.EnableECSManagedTags = &v
	return s
}

// SetGroup sets the Group field's value.
func (s *StartTaskInput) SetGroup(v string) *StartTaskInput {
	s.Group = &v
	return s
}

// SetNetworkConfiguration sets the NetworkConfiguration field's value.
func (s *StartTaskInput) SetNetworkConfiguration(v *NetworkConfiguration) *StartTaskInput {
	s.NetworkConfiguration = v
	return s
}

// SetOverrides sets the Overrides field's value.
func (s *StartTaskInput) SetOverrides(v *TaskOverride) *StartTaskInput {
	s.Overrides = v
	return s
}

// SetPropagateTags sets the PropagateTags field's value.
func (s *StartTaskInput) SetPropagateTags(v string) *StartTaskInput {
	s.PropagateTags = &v
	return s
}

// SetProperties sets the Properties field's value.
func (s *StartTaskInput) SetProperties(v []*KeyValuePair) *StartTaskInput {
	s.Properties = v
	return s
}

// SetStartedBy sets the StartedBy field's value.
func (s *StartTaskInput) SetStartedBy(v string) *StartTaskInput {
	s.StartedBy = &v
	return s
}

// SetTags sets the Tags field's value.
func (s *StartTaskInput) SetTags(v []*Tag) *StartTaskInput {
	s.Tags = v
	return s
}

// SetTaskDefinition sets the TaskDefinition field's value.
func (s *StartTaskInput) SetTaskDefinition(v string) *StartTaskInput {
	s.TaskDefinition = &v
	return s
}

type StartTaskOutput struct {
	_ struct{} `type:"structure"`

	Failures []*Failure `locationName:"failures" type:"list"`

	Tasks []*Task `locationName:"tasks" type:"list"`
}

// String returns the string representation
func (s StartTaskOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s StartTaskOutput) GoString() string {
	return s.String()
}

// SetFailures sets the Failures field's value.
func (s *StartTaskOutput) SetFailures(v []*Failure) *StartTaskOutput {
	s.Failures = v
	return s
}

// SetTasks sets the Tasks field's value.
func (s *StartTaskOutput) SetTasks(v []*Task) *StartTaskOutput {
	s.Tasks = v
	return s
}

type StopTaskInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	Reason *string `locationName:"reason" type:"string"`

	// Task is a required field
	Task *string `locationName:"task" type:"string" required:"true"`
}

// String returns the string representation
func (s StopTaskInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s StopTaskInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *StopTaskInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "StopTaskInput"}
	if s.Task == nil {
		invalidParams.Add(request.NewErrParamRequired("Task"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCluster sets the Cluster field's value.
func (s *StopTaskInput) SetCluster(v string) *StopTaskInput {
	s.Cluster = &v
	return s
}

// SetReason sets the Reason field's value.
func (s *StopTaskInput) SetReason(v string) *StopTaskInput {
	s.Reason = &v
	return s
}

// SetTask sets the Task field's value.
func (s *StopTaskInput) SetTask(v string) *StopTaskInput {
	s.Task = &v
	return s
}

type StopTaskOutput struct {
	_ struct{} `type:"structure"`

	Task *Task `locationName:"task" type:"structure"`
}

// String returns the string representation
func (s StopTaskOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s StopTaskOutput) GoString() string {
	return s.String()
}

// SetTask sets the Task field's value.
func (s *StopTaskOutput) SetTask(v *Task) *StopTaskOutput {
	s.Task = v
	return s
}

type SubmitAttachmentStateChangesInput struct {
	_ struct{} `type:"structure"`

	// Attachments is a required field
	Attachments []*AttachmentStateChange `locationName:"attachments" type:"list" required:"true"`

	Cluster *string `locationName:"cluster" type:"string"`
}

// String returns the string representation
func (s SubmitAttachmentStateChangesInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s SubmitAttachmentStateChangesInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *SubmitAttachmentStateChangesInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "SubmitAttachmentStateChangesInput"}
	if s.Attachments == nil {
		invalidParams.Add(request.NewErrParamRequired("Attachments"))
	}
	if s.Attachments != nil {
		for i, v := range s.Attachments {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Attachments", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetAttachments sets the Attachments field's value.
func (s *SubmitAttachmentStateChangesInput) SetAttachments(v []*AttachmentStateChange) *SubmitAttachmentStateChangesInput {
	s.Attachments = v
	return s
}

// SetCluster sets the Cluster field's value.
func (s *SubmitAttachmentStateChangesInput) SetCluster(v string) *SubmitAttachmentStateChangesInput {
	s.Cluster = &v
	return s
}

type SubmitAttachmentStateChangesOutput struct {
	_ struct{} `type:"structure"`

	Acknowledgment *string `locationName:"acknowledgment" type:"string"`
}

// String returns the string representation
func (s SubmitAttachmentStateChangesOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s SubmitAttachmentStateChangesOutput) GoString() string {
	return s.String()
}

// SetAcknowledgment sets the Acknowledgment field's value.
func (s *SubmitAttachmentStateChangesOutput) SetAcknowledgment(v string) *SubmitAttachmentStateChangesOutput {
	s.Acknowledgment = &v
	return s
}

type SubmitContainerStateChangeInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	ContainerName *string `locationName:"containerName" type:"string"`

	ExitCode *int64 `locationName:"exitCode" type:"integer"`

	NetworkBindings []*NetworkBinding `locationName:"networkBindings" type:"list"`

	Reason *string `locationName:"reason" type:"string"`

	Status *string `locationName:"status" type:"string"`

	Task *string `locationName:"task" type:"string"`
}

// String returns the string representation
func (s SubmitContainerStateChangeInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s SubmitContainerStateChangeInput) GoString() string {
	return s.String()
}

// SetCluster sets the Cluster field's value.
func (s *SubmitContainerStateChangeInput) SetCluster(v string) *SubmitContainerStateChangeInput {
	s.Cluster = &v
	return s
}

// SetContainerName sets the ContainerName field's value.
func (s *SubmitContainerStateChangeInput) SetContainerName(v string) *SubmitContainerStateChangeInput {
	s.ContainerName = &v
	return s
}

// SetExitCode sets the ExitCode field's value.
func (s *SubmitContainerStateChangeInput) SetExitCode(v int64) *SubmitContainerStateChangeInput {
	s.ExitCode = &v
	return s
}

// SetNetworkBindings sets the NetworkBindings field's value.
func (s *SubmitContainerStateChangeInput) SetNetworkBindings(v []*NetworkBinding) *SubmitContainerStateChangeInput {
	s.NetworkBindings = v
	return s
}

// SetReason sets the Reason field's value.
func (s *SubmitContainerStateChangeInput) SetReason(v string) *SubmitContainerStateChangeInput {
	s.Reason = &v
	return s
}

// SetStatus sets the Status field's value.
func (s *SubmitContainerStateChangeInput) SetStatus(v string) *SubmitContainerStateChangeInput {
	s.Status = &v
	return s
}

// SetTask sets the Task field's value.
func (s *SubmitContainerStateChangeInput) SetTask(v string) *SubmitContainerStateChangeInput {
	s.Task = &v
	return s
}

type SubmitContainerStateChangeOutput struct {
	_ struct{} `type:"structure"`

	Acknowledgment *string `locationName:"acknowledgment" type:"string"`
}

// String returns the string representation
func (s SubmitContainerStateChangeOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s SubmitContainerStateChangeOutput) GoString() string {
	return s.String()
}

// SetAcknowledgment sets the Acknowledgment field's value.
func (s *SubmitContainerStateChangeOutput) SetAcknowledgment(v string) *SubmitContainerStateChangeOutput {
	s.Acknowledgment = &v
	return s
}

type SubmitTaskStateChangeInput struct {
	_ struct{} `type:"structure"`

	Attachments []*AttachmentStateChange `locationName:"attachments" type:"list"`

	Cluster *string `locationName:"cluster" type:"string"`

	Containers []*ContainerStateChange `locationName:"containers" type:"list"`

	ExecutionStoppedAt *time.Time `locationName:"executionStoppedAt" type:"timestamp"`

	PullStartedAt *time.Time `locationName:"pullStartedAt" type:"timestamp"`

	PullStoppedAt *time.Time `locationName:"pullStoppedAt" type:"timestamp"`

	Reason *string `locationName:"reason" type:"string"`

	Status *string `locationName:"status" type:"string"`

	Task *string `locationName:"task" type:"string"`
}

// String returns the string representation
func (s SubmitTaskStateChangeInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s SubmitTaskStateChangeInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *SubmitTaskStateChangeInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "SubmitTaskStateChangeInput"}
	if s.Attachments != nil {
		for i, v := range s.Attachments {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Attachments", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetAttachments sets the Attachments field's value.
func (s *SubmitTaskStateChangeInput) SetAttachments(v []*AttachmentStateChange) *SubmitTaskStateChangeInput {
	s.Attachments = v
	return s
}

// SetCluster sets the Cluster field's value.
func (s *SubmitTaskStateChangeInput) SetCluster(v string) *SubmitTaskStateChangeInput {
	s.Cluster = &v
	return s
}

// SetContainers sets the Containers field's value.
func (s *SubmitTaskStateChangeInput) SetContainers(v []*ContainerStateChange) *SubmitTaskStateChangeInput {
	s.Containers = v
	return s
}

// SetExecutionStoppedAt sets the ExecutionStoppedAt field's value.
func (s *SubmitTaskStateChangeInput) SetExecutionStoppedAt(v time.Time) *SubmitTaskStateChangeInput {
	s.ExecutionStoppedAt = &v
	return s
}

// SetPullStartedAt sets the PullStartedAt field's value.
func (s *SubmitTaskStateChangeInput) SetPullStartedAt(v time.Time) *SubmitTaskStateChangeInput {
	s.PullStartedAt = &v
	return s
}

// SetPullStoppedAt sets the PullStoppedAt field's value.
func (s *SubmitTaskStateChangeInput) SetPullStoppedAt(v time.Time) *SubmitTaskStateChangeInput {
	s.PullStoppedAt = &v
	return s
}

// SetReason sets the Reason field's value.
func (s *SubmitTaskStateChangeInput) SetReason(v string) *SubmitTaskStateChangeInput {
	s.Reason = &v
	return s
}

// SetStatus sets the Status field's value.
func (s *SubmitTaskStateChangeInput) SetStatus(v string) *SubmitTaskStateChangeInput {
	s.Status = &v
	return s
}

// SetTask sets the Task field's value.
func (s *SubmitTaskStateChangeInput) SetTask(v string) *SubmitTaskStateChangeInput {
	s.Task = &v
	return s
}

type SubmitTaskStateChangeOutput struct {
	_ struct{} `type:"structure"`

	Acknowledgment *string `locationName:"acknowledgment" type:"string"`
}

// String returns the string representation
func (s SubmitTaskStateChangeOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s SubmitTaskStateChangeOutput) GoString() string {
	return s.String()
}

// SetAcknowledgment sets the Acknowledgment field's value.
func (s *SubmitTaskStateChangeOutput) SetAcknowledgment(v string) *SubmitTaskStateChangeOutput {
	s.Acknowledgment = &v
	return s
}

type SystemControl struct {
	_ struct{} `type:"structure"`

	Namespace *string `locationName:"namespace" type:"string"`

	Value *string `locationName:"value" type:"string"`
}

// String returns the string representation
func (s SystemControl) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s SystemControl) GoString() string {
	return s.String()
}

// SetNamespace sets the Namespace field's value.
func (s *SystemControl) SetNamespace(v string) *SystemControl {
	s.Namespace = &v
	return s
}

// SetValue sets the Value field's value.
func (s *SystemControl) SetValue(v string) *SystemControl {
	s.Value = &v
	return s
}

type Tag struct {
	_ struct{} `type:"structure"`

	Key *string `locationName:"key" min:"1" type:"string"`

	Value *string `locationName:"value" type:"string"`
}

// String returns the string representation
func (s Tag) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Tag) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *Tag) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "Tag"}
	if s.Key != nil && len(*s.Key) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("Key", 1))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetKey sets the Key field's value.
func (s *Tag) SetKey(v string) *Tag {
	s.Key = &v
	return s
}

// SetValue sets the Value field's value.
func (s *Tag) SetValue(v string) *Tag {
	s.Value = &v
	return s
}

type TagResourceInput struct {
	_ struct{} `type:"structure"`

	// ResourceArn is a required field
	ResourceArn *string `locationName:"resourceArn" type:"string" required:"true"`

	// Tags is a required field
	Tags []*Tag `locationName:"tags" type:"list" required:"true"`
}

// String returns the string representation
func (s TagResourceInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s TagResourceInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *TagResourceInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "TagResourceInput"}
	if s.ResourceArn == nil {
		invalidParams.Add(request.NewErrParamRequired("ResourceArn"))
	}
	if s.Tags == nil {
		invalidParams.Add(request.NewErrParamRequired("Tags"))
	}
	if s.Tags != nil {
		for i, v := range s.Tags {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Tags", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetResourceArn sets the ResourceArn field's value.
func (s *TagResourceInput) SetResourceArn(v string) *TagResourceInput {
	s.ResourceArn = &v
	return s
}

// SetTags sets the Tags field's value.
func (s *TagResourceInput) SetTags(v []*Tag) *TagResourceInput {
	s.Tags = v
	return s
}

type TagResourceOutput struct {
	_ struct{} `type:"structure"`
}

// String returns the string representation
func (s TagResourceOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s TagResourceOutput) GoString() string {
	return s.String()
}

type Task struct {
	_ struct{} `type:"structure"`

	Attachments []*Attachment `locationName:"attachments" type:"list"`

	Attributes []*Attribute `locationName:"attributes" type:"list"`

	ClusterArn *string `locationName:"clusterArn" type:"string"`

	Connectivity *string `locationName:"connectivity" type:"string" enum:"Connectivity"`

	ConnectivityAt *time.Time `locationName:"connectivityAt" type:"timestamp"`

	ContainerInstanceArn *string `locationName:"containerInstanceArn" type:"string"`

	Containers []*Container `locationName:"containers" type:"list"`

	Cpu *string `locationName:"cpu" type:"string"`

	CreatedAt *time.Time `locationName:"createdAt" type:"timestamp"`

	DesiredStatus *string `locationName:"desiredStatus" type:"string"`

	ExecutionStoppedAt *time.Time `locationName:"executionStoppedAt" type:"timestamp"`

	Group *string `locationName:"group" type:"string"`

	HealthStatus *string `locationName:"healthStatus" type:"string" enum:"HealthStatus"`

	LastStatus *string `locationName:"lastStatus" type:"string"`

	LaunchType *string `locationName:"launchType" type:"string" enum:"LaunchType"`

	Memory *string `locationName:"memory" type:"string"`

	Overrides *TaskOverride `locationName:"overrides" type:"structure"`

	PlatformVersion *string `locationName:"platformVersion" type:"string"`

	PullStartedAt *time.Time `locationName:"pullStartedAt" type:"timestamp"`

	PullStoppedAt *time.Time `locationName:"pullStoppedAt" type:"timestamp"`

	StartedAt *time.Time `locationName:"startedAt" type:"timestamp"`

	StartedBy *string `locationName:"startedBy" type:"string"`

	StopCode *string `locationName:"stopCode" type:"string" enum:"TaskStopCode"`

	StoppedAt *time.Time `locationName:"stoppedAt" type:"timestamp"`

	StoppedReason *string `locationName:"stoppedReason" type:"string"`

	StoppingAt *time.Time `locationName:"stoppingAt" type:"timestamp"`

	Tags []*Tag `locationName:"tags" type:"list"`

	TaskArn *string `locationName:"taskArn" type:"string"`

	TaskDefinitionArn *string `locationName:"taskDefinitionArn" type:"string"`

	Version *int64 `locationName:"version" type:"long"`
}

// String returns the string representation
func (s Task) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Task) GoString() string {
	return s.String()
}

// SetAttachments sets the Attachments field's value.
func (s *Task) SetAttachments(v []*Attachment) *Task {
	s.Attachments = v
	return s
}

// SetAttributes sets the Attributes field's value.
func (s *Task) SetAttributes(v []*Attribute) *Task {
	s.Attributes = v
	return s
}

// SetClusterArn sets the ClusterArn field's value.
func (s *Task) SetClusterArn(v string) *Task {
	s.ClusterArn = &v
	return s
}

// SetConnectivity sets the Connectivity field's value.
func (s *Task) SetConnectivity(v string) *Task {
	s.Connectivity = &v
	return s
}

// SetConnectivityAt sets the ConnectivityAt field's value.
func (s *Task) SetConnectivityAt(v time.Time) *Task {
	s.ConnectivityAt = &v
	return s
}

// SetContainerInstanceArn sets the ContainerInstanceArn field's value.
func (s *Task) SetContainerInstanceArn(v string) *Task {
	s.ContainerInstanceArn = &v
	return s
}

// SetContainers sets the Containers field's value.
func (s *Task) SetContainers(v []*Container) *Task {
	s.Containers = v
	return s
}

// SetCpu sets the Cpu field's value.
func (s *Task) SetCpu(v string) *Task {
	s.Cpu = &v
	return s
}

// SetCreatedAt sets the CreatedAt field's value.
func (s *Task) SetCreatedAt(v time.Time) *Task {
	s.CreatedAt = &v
	return s
}

// SetDesiredStatus sets the DesiredStatus field's value.
func (s *Task) SetDesiredStatus(v string) *Task {
	s.DesiredStatus = &v
	return s
}

// SetExecutionStoppedAt sets the ExecutionStoppedAt field's value.
func (s *Task) SetExecutionStoppedAt(v time.Time) *Task {
	s.ExecutionStoppedAt = &v
	return s
}

// SetGroup sets the Group field's value.
func (s *Task) SetGroup(v string) *Task {
	s.Group = &v
	return s
}

// SetHealthStatus sets the HealthStatus field's value.
func (s *Task) SetHealthStatus(v string) *Task {
	s.HealthStatus = &v
	return s
}

// SetLastStatus sets the LastStatus field's value.
func (s *Task) SetLastStatus(v string) *Task {
	s.LastStatus = &v
	return s
}

// SetLaunchType sets the LaunchType field's value.
func (s *Task) SetLaunchType(v string) *Task {
	s.LaunchType = &v
	return s
}

// SetMemory sets the Memory field's value.
func (s *Task) SetMemory(v string) *Task {
	s.Memory = &v
	return s
}

// SetOverrides sets the Overrides field's value.
func (s *Task) SetOverrides(v *TaskOverride) *Task {
	s.Overrides = v
	return s
}

// SetPlatformVersion sets the PlatformVersion field's value.
func (s *Task) SetPlatformVersion(v string) *Task {
	s.PlatformVersion = &v
	return s
}

// SetPullStartedAt sets the PullStartedAt field's value.
func (s *Task) SetPullStartedAt(v time.Time) *Task {
	s.PullStartedAt = &v
	return s
}

// SetPullStoppedAt sets the PullStoppedAt field's value.
func (s *Task) SetPullStoppedAt(v time.Time) *Task {
	s.PullStoppedAt = &v
	return s
}

// SetStartedAt sets the StartedAt field's value.
func (s *Task) SetStartedAt(v time.Time) *Task {
	s.StartedAt = &v
	return s
}

// SetStartedBy sets the StartedBy field's value.
func (s *Task) SetStartedBy(v string) *Task {
	s.StartedBy = &v
	return s
}

// SetStopCode sets the StopCode field's value.
func (s *Task) SetStopCode(v string) *Task {
	s.StopCode = &v
	return s
}

// SetStoppedAt sets the StoppedAt field's value.
func (s *Task) SetStoppedAt(v time.Time) *Task {
	s.StoppedAt = &v
	return s
}

// SetStoppedReason sets the StoppedReason field's value.
func (s *Task) SetStoppedReason(v string) *Task {
	s.StoppedReason = &v
	return s
}

// SetStoppingAt sets the StoppingAt field's value.
func (s *Task) SetStoppingAt(v time.Time) *Task {
	s.StoppingAt = &v
	return s
}

// SetTags sets the Tags field's value.
func (s *Task) SetTags(v []*Tag) *Task {
	s.Tags = v
	return s
}

// SetTaskArn sets the TaskArn field's value.
func (s *Task) SetTaskArn(v string) *Task {
	s.TaskArn = &v
	return s
}

// SetTaskDefinitionArn sets the TaskDefinitionArn field's value.
func (s *Task) SetTaskDefinitionArn(v string) *Task {
	s.TaskDefinitionArn = &v
	return s
}

// SetVersion sets the Version field's value.
func (s *Task) SetVersion(v int64) *Task {
	s.Version = &v
	return s
}

type TaskDefinition struct {
	_ struct{} `type:"structure"`

	Compatibilities []*string `locationName:"compatibilities" type:"list"`

	ContainerDefinitions []*ContainerDefinition `locationName:"containerDefinitions" type:"list"`

	Cpu *string `locationName:"cpu" type:"string"`

	ExecutionRoleArn *string `locationName:"executionRoleArn" type:"string"`

	Family *string `locationName:"family" type:"string"`

	InferenceAccelerators []*InferenceAccelerator `locationName:"inferenceAccelerators" type:"list"`

	IpcMode *string `locationName:"ipcMode" type:"string" enum:"IpcMode"`

	Memory *string `locationName:"memory" type:"string"`

	NetworkMode *string `locationName:"networkMode" type:"string" enum:"NetworkMode"`

	PidMode *string `locationName:"pidMode" type:"string" enum:"PidMode"`

	PlacementConstraints []*TaskDefinitionPlacementConstraint `locationName:"placementConstraints" type:"list"`

	ProxyConfiguration *ProxyConfiguration `locationName:"proxyConfiguration" type:"structure"`

	RequiresAttributes []*Attribute `locationName:"requiresAttributes" type:"list"`

	RequiresCompatibilities []*string `locationName:"requiresCompatibilities" type:"list"`

	Revision *int64 `locationName:"revision" type:"integer"`

	Status *string `locationName:"status" type:"string" enum:"TaskDefinitionStatus"`

	TaskDefinitionArn *string `locationName:"taskDefinitionArn" type:"string"`

	TaskRoleArn *string `locationName:"taskRoleArn" type:"string"`

	Volumes []*Volume `locationName:"volumes" type:"list"`
}

// String returns the string representation
func (s TaskDefinition) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s TaskDefinition) GoString() string {
	return s.String()
}

// SetCompatibilities sets the Compatibilities field's value.
func (s *TaskDefinition) SetCompatibilities(v []*string) *TaskDefinition {
	s.Compatibilities = v
	return s
}

// SetContainerDefinitions sets the ContainerDefinitions field's value.
func (s *TaskDefinition) SetContainerDefinitions(v []*ContainerDefinition) *TaskDefinition {
	s.ContainerDefinitions = v
	return s
}

// SetCpu sets the Cpu field's value.
func (s *TaskDefinition) SetCpu(v string) *TaskDefinition {
	s.Cpu = &v
	return s
}

// SetExecutionRoleArn sets the ExecutionRoleArn field's value.
func (s *TaskDefinition) SetExecutionRoleArn(v string) *TaskDefinition {
	s.ExecutionRoleArn = &v
	return s
}

// SetFamily sets the Family field's value.
func (s *TaskDefinition) SetFamily(v string) *TaskDefinition {
	s.Family = &v
	return s
}

// SetInferenceAccelerators sets the InferenceAccelerators field's value.
func (s *TaskDefinition) SetInferenceAccelerators(v []*InferenceAccelerator) *TaskDefinition {
	s.InferenceAccelerators = v
	return s
}

// SetIpcMode sets the IpcMode field's value.
func (s *TaskDefinition) SetIpcMode(v string) *TaskDefinition {
	s.IpcMode = &v
	return s
}

// SetMemory sets the Memory field's value.
func (s *TaskDefinition) SetMemory(v string) *TaskDefinition {
	s.Memory = &v
	return s
}

// SetNetworkMode sets the NetworkMode field's value.
func (s *TaskDefinition) SetNetworkMode(v string) *TaskDefinition {
	s.NetworkMode = &v
	return s
}

// SetPidMode sets the PidMode field's value.
func (s *TaskDefinition) SetPidMode(v string) *TaskDefinition {
	s.PidMode = &v
	return s
}

// SetPlacementConstraints sets the PlacementConstraints field's value.
func (s *TaskDefinition) SetPlacementConstraints(v []*TaskDefinitionPlacementConstraint) *TaskDefinition {
	s.PlacementConstraints = v
	return s
}

// SetProxyConfiguration sets the ProxyConfiguration field's value.
func (s *TaskDefinition) SetProxyConfiguration(v *ProxyConfiguration) *TaskDefinition {
	s.ProxyConfiguration = v
	return s
}

// SetRequiresAttributes sets the RequiresAttributes field's value.
func (s *TaskDefinition) SetRequiresAttributes(v []*Attribute) *TaskDefinition {
	s.RequiresAttributes = v
	return s
}

// SetRequiresCompatibilities sets the RequiresCompatibilities field's value.
func (s *TaskDefinition) SetRequiresCompatibilities(v []*string) *TaskDefinition {
	s.RequiresCompatibilities = v
	return s
}

// SetRevision sets the Revision field's value.
func (s *TaskDefinition) SetRevision(v int64) *TaskDefinition {
	s.Revision = &v
	return s
}

// SetStatus sets the Status field's value.
func (s *TaskDefinition) SetStatus(v string) *TaskDefinition {
	s.Status = &v
	return s
}

// SetTaskDefinitionArn sets the TaskDefinitionArn field's value.
func (s *TaskDefinition) SetTaskDefinitionArn(v string) *TaskDefinition {
	s.TaskDefinitionArn = &v
	return s
}

// SetTaskRoleArn sets the TaskRoleArn field's value.
func (s *TaskDefinition) SetTaskRoleArn(v string) *TaskDefinition {
	s.TaskRoleArn = &v
	return s
}

// SetVolumes sets the Volumes field's value.
func (s *TaskDefinition) SetVolumes(v []*Volume) *TaskDefinition {
	s.Volumes = v
	return s
}

type TaskDefinitionPlacementConstraint struct {
	_ struct{} `type:"structure"`

	Expression *string `locationName:"expression" type:"string"`

	Type *string `locationName:"type" type:"string" enum:"TaskDefinitionPlacementConstraintType"`
}

// String returns the string representation
func (s TaskDefinitionPlacementConstraint) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s TaskDefinitionPlacementConstraint) GoString() string {
	return s.String()
}

// SetExpression sets the Expression field's value.
func (s *TaskDefinitionPlacementConstraint) SetExpression(v string) *TaskDefinitionPlacementConstraint {
	s.Expression = &v
	return s
}

// SetType sets the Type field's value.
func (s *TaskDefinitionPlacementConstraint) SetType(v string) *TaskDefinitionPlacementConstraint {
	s.Type = &v
	return s
}

type TaskOverride struct {
	_ struct{} `type:"structure"`

	ContainerOverrides []*ContainerOverride `locationName:"containerOverrides" type:"list"`

	ExecutionRoleArn *string `locationName:"executionRoleArn" type:"string"`

	InfrastructureBlockDeviceConfig *string `locationName:"infrastructureBlockDeviceConfig" type:"string"`

	TaskRoleArn *string `locationName:"taskRoleArn" type:"string"`
}

// String returns the string representation
func (s TaskOverride) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s TaskOverride) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *TaskOverride) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "TaskOverride"}
	if s.ContainerOverrides != nil {
		for i, v := range s.ContainerOverrides {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "ContainerOverrides", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetContainerOverrides sets the ContainerOverrides field's value.
func (s *TaskOverride) SetContainerOverrides(v []*ContainerOverride) *TaskOverride {
	s.ContainerOverrides = v
	return s
}

// SetExecutionRoleArn sets the ExecutionRoleArn field's value.
func (s *TaskOverride) SetExecutionRoleArn(v string) *TaskOverride {
	s.ExecutionRoleArn = &v
	return s
}

// SetInfrastructureBlockDeviceConfig sets the InfrastructureBlockDeviceConfig field's value.
func (s *TaskOverride) SetInfrastructureBlockDeviceConfig(v string) *TaskOverride {
	s.InfrastructureBlockDeviceConfig = &v
	return s
}

// SetTaskRoleArn sets the TaskRoleArn field's value.
func (s *TaskOverride) SetTaskRoleArn(v string) *TaskOverride {
	s.TaskRoleArn = &v
	return s
}

type TaskSet struct {
	_ struct{} `type:"structure"`

	ClusterArn *string `locationName:"clusterArn" type:"string"`

	ComputedDesiredCount *int64 `locationName:"computedDesiredCount" type:"integer"`

	CreatedAt *time.Time `locationName:"createdAt" type:"timestamp"`

	ExternalId *string `locationName:"externalId" type:"string"`

	Id *string `locationName:"id" type:"string"`

	LaunchType *string `locationName:"launchType" type:"string" enum:"LaunchType"`

	LoadBalancers []*LoadBalancer `locationName:"loadBalancers" type:"list"`

	NetworkConfiguration *NetworkConfiguration `locationName:"networkConfiguration" type:"structure"`

	PendingCount *int64 `locationName:"pendingCount" type:"integer"`

	PlatformVersion *string `locationName:"platformVersion" type:"string"`

	RunningCount *int64 `locationName:"runningCount" type:"integer"`

	Scale *Scale `locationName:"scale" type:"structure"`

	ServiceArn *string `locationName:"serviceArn" type:"string"`

	ServiceRegistries []*ServiceRegistry `locationName:"serviceRegistries" type:"list"`

	StabilityStatus *string `locationName:"stabilityStatus" type:"string" enum:"StabilityStatus"`

	StabilityStatusAt *time.Time `locationName:"stabilityStatusAt" type:"timestamp"`

	StartedBy *string `locationName:"startedBy" type:"string"`

	Status *string `locationName:"status" type:"string"`

	TaskDefinition *string `locationName:"taskDefinition" type:"string"`

	TaskSetArn *string `locationName:"taskSetArn" type:"string"`

	UpdatedAt *time.Time `locationName:"updatedAt" type:"timestamp"`
}

// String returns the string representation
func (s TaskSet) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s TaskSet) GoString() string {
	return s.String()
}

// SetClusterArn sets the ClusterArn field's value.
func (s *TaskSet) SetClusterArn(v string) *TaskSet {
	s.ClusterArn = &v
	return s
}

// SetComputedDesiredCount sets the ComputedDesiredCount field's value.
func (s *TaskSet) SetComputedDesiredCount(v int64) *TaskSet {
	s.ComputedDesiredCount = &v
	return s
}

// SetCreatedAt sets the CreatedAt field's value.
func (s *TaskSet) SetCreatedAt(v time.Time) *TaskSet {
	s.CreatedAt = &v
	return s
}

// SetExternalId sets the ExternalId field's value.
func (s *TaskSet) SetExternalId(v string) *TaskSet {
	s.ExternalId = &v
	return s
}

// SetId sets the Id field's value.
func (s *TaskSet) SetId(v string) *TaskSet {
	s.Id = &v
	return s
}

// SetLaunchType sets the LaunchType field's value.
func (s *TaskSet) SetLaunchType(v string) *TaskSet {
	s.LaunchType = &v
	return s
}

// SetLoadBalancers sets the LoadBalancers field's value.
func (s *TaskSet) SetLoadBalancers(v []*LoadBalancer) *TaskSet {
	s.LoadBalancers = v
	return s
}

// SetNetworkConfiguration sets the NetworkConfiguration field's value.
func (s *TaskSet) SetNetworkConfiguration(v *NetworkConfiguration) *TaskSet {
	s.NetworkConfiguration = v
	return s
}

// SetPendingCount sets the PendingCount field's value.
func (s *TaskSet) SetPendingCount(v int64) *TaskSet {
	s.PendingCount = &v
	return s
}

// SetPlatformVersion sets the PlatformVersion field's value.
func (s *TaskSet) SetPlatformVersion(v string) *TaskSet {
	s.PlatformVersion = &v
	return s
}

// SetRunningCount sets the RunningCount field's value.
func (s *TaskSet) SetRunningCount(v int64) *TaskSet {
	s.RunningCount = &v
	return s
}

// SetScale sets the Scale field's value.
func (s *TaskSet) SetScale(v *Scale) *TaskSet {
	s.Scale = v
	return s
}

// SetServiceArn sets the ServiceArn field's value.
func (s *TaskSet) SetServiceArn(v string) *TaskSet {
	s.ServiceArn = &v
	return s
}

// SetServiceRegistries sets the ServiceRegistries field's value.
func (s *TaskSet) SetServiceRegistries(v []*ServiceRegistry) *TaskSet {
	s.ServiceRegistries = v
	return s
}

// SetStabilityStatus sets the StabilityStatus field's value.
func (s *TaskSet) SetStabilityStatus(v string) *TaskSet {
	s.StabilityStatus = &v
	return s
}

// SetStabilityStatusAt sets the StabilityStatusAt field's value.
func (s *TaskSet) SetStabilityStatusAt(v time.Time) *TaskSet {
	s.StabilityStatusAt = &v
	return s
}

// SetStartedBy sets the StartedBy field's value.
func (s *TaskSet) SetStartedBy(v string) *TaskSet {
	s.StartedBy = &v
	return s
}

// SetStatus sets the Status field's value.
func (s *TaskSet) SetStatus(v string) *TaskSet {
	s.Status = &v
	return s
}

// SetTaskDefinition sets the TaskDefinition field's value.
func (s *TaskSet) SetTaskDefinition(v string) *TaskSet {
	s.TaskDefinition = &v
	return s
}

// SetTaskSetArn sets the TaskSetArn field's value.
func (s *TaskSet) SetTaskSetArn(v string) *TaskSet {
	s.TaskSetArn = &v
	return s
}

// SetUpdatedAt sets the UpdatedAt field's value.
func (s *TaskSet) SetUpdatedAt(v time.Time) *TaskSet {
	s.UpdatedAt = &v
	return s
}

type Tmpfs struct {
	_ struct{} `type:"structure"`

	// ContainerPath is a required field
	ContainerPath *string `locationName:"containerPath" type:"string" required:"true"`

	MountOptions []*string `locationName:"mountOptions" type:"list"`

	// Size is a required field
	Size *int64 `locationName:"size" type:"integer" required:"true"`
}

// String returns the string representation
func (s Tmpfs) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Tmpfs) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *Tmpfs) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "Tmpfs"}
	if s.ContainerPath == nil {
		invalidParams.Add(request.NewErrParamRequired("ContainerPath"))
	}
	if s.Size == nil {
		invalidParams.Add(request.NewErrParamRequired("Size"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetContainerPath sets the ContainerPath field's value.
func (s *Tmpfs) SetContainerPath(v string) *Tmpfs {
	s.ContainerPath = &v
	return s
}

// SetMountOptions sets the MountOptions field's value.
func (s *Tmpfs) SetMountOptions(v []*string) *Tmpfs {
	s.MountOptions = v
	return s
}

// SetSize sets the Size field's value.
func (s *Tmpfs) SetSize(v int64) *Tmpfs {
	s.Size = &v
	return s
}

type Ulimit struct {
	_ struct{} `type:"structure"`

	// HardLimit is a required field
	HardLimit *int64 `locationName:"hardLimit" type:"integer" required:"true"`

	// Name is a required field
	Name *string `locationName:"name" type:"string" required:"true" enum:"UlimitName"`

	// SoftLimit is a required field
	SoftLimit *int64 `locationName:"softLimit" type:"integer" required:"true"`
}

// String returns the string representation
func (s Ulimit) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Ulimit) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *Ulimit) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "Ulimit"}
	if s.HardLimit == nil {
		invalidParams.Add(request.NewErrParamRequired("HardLimit"))
	}
	if s.Name == nil {
		invalidParams.Add(request.NewErrParamRequired("Name"))
	}
	if s.SoftLimit == nil {
		invalidParams.Add(request.NewErrParamRequired("SoftLimit"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetHardLimit sets the HardLimit field's value.
func (s *Ulimit) SetHardLimit(v int64) *Ulimit {
	s.HardLimit = &v
	return s
}

// SetName sets the Name field's value.
func (s *Ulimit) SetName(v string) *Ulimit {
	s.Name = &v
	return s
}

// SetSoftLimit sets the SoftLimit field's value.
func (s *Ulimit) SetSoftLimit(v int64) *Ulimit {
	s.SoftLimit = &v
	return s
}

type UntagResourceInput struct {
	_ struct{} `type:"structure"`

	// ResourceArn is a required field
	ResourceArn *string `locationName:"resourceArn" type:"string" required:"true"`

	// TagKeys is a required field
	TagKeys []*string `locationName:"tagKeys" type:"list" required:"true"`
}

// String returns the string representation
func (s UntagResourceInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UntagResourceInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *UntagResourceInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "UntagResourceInput"}
	if s.ResourceArn == nil {
		invalidParams.Add(request.NewErrParamRequired("ResourceArn"))
	}
	if s.TagKeys == nil {
		invalidParams.Add(request.NewErrParamRequired("TagKeys"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetResourceArn sets the ResourceArn field's value.
func (s *UntagResourceInput) SetResourceArn(v string) *UntagResourceInput {
	s.ResourceArn = &v
	return s
}

// SetTagKeys sets the TagKeys field's value.
func (s *UntagResourceInput) SetTagKeys(v []*string) *UntagResourceInput {
	s.TagKeys = v
	return s
}

type UntagResourceOutput struct {
	_ struct{} `type:"structure"`
}

// String returns the string representation
func (s UntagResourceOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UntagResourceOutput) GoString() string {
	return s.String()
}

type UpdateContainerAgentInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	// ContainerInstance is a required field
	ContainerInstance *string `locationName:"containerInstance" type:"string" required:"true"`
}

// String returns the string representation
func (s UpdateContainerAgentInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UpdateContainerAgentInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *UpdateContainerAgentInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "UpdateContainerAgentInput"}
	if s.ContainerInstance == nil {
		invalidParams.Add(request.NewErrParamRequired("ContainerInstance"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCluster sets the Cluster field's value.
func (s *UpdateContainerAgentInput) SetCluster(v string) *UpdateContainerAgentInput {
	s.Cluster = &v
	return s
}

// SetContainerInstance sets the ContainerInstance field's value.
func (s *UpdateContainerAgentInput) SetContainerInstance(v string) *UpdateContainerAgentInput {
	s.ContainerInstance = &v
	return s
}

type UpdateContainerAgentOutput struct {
	_ struct{} `type:"structure"`

	ContainerInstance *ContainerInstance `locationName:"containerInstance" type:"structure"`
}

// String returns the string representation
func (s UpdateContainerAgentOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UpdateContainerAgentOutput) GoString() string {
	return s.String()
}

// SetContainerInstance sets the ContainerInstance field's value.
func (s *UpdateContainerAgentOutput) SetContainerInstance(v *ContainerInstance) *UpdateContainerAgentOutput {
	s.ContainerInstance = v
	return s
}

type UpdateContainerInstancesStateInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	// ContainerInstances is a required field
	ContainerInstances []*string `locationName:"containerInstances" type:"list" required:"true"`

	// Status is a required field
	Status *string `locationName:"status" type:"string" required:"true" enum:"ContainerInstanceStatus"`
}

// String returns the string representation
func (s UpdateContainerInstancesStateInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UpdateContainerInstancesStateInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *UpdateContainerInstancesStateInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "UpdateContainerInstancesStateInput"}
	if s.ContainerInstances == nil {
		invalidParams.Add(request.NewErrParamRequired("ContainerInstances"))
	}
	if s.Status == nil {
		invalidParams.Add(request.NewErrParamRequired("Status"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCluster sets the Cluster field's value.
func (s *UpdateContainerInstancesStateInput) SetCluster(v string) *UpdateContainerInstancesStateInput {
	s.Cluster = &v
	return s
}

// SetContainerInstances sets the ContainerInstances field's value.
func (s *UpdateContainerInstancesStateInput) SetContainerInstances(v []*string) *UpdateContainerInstancesStateInput {
	s.ContainerInstances = v
	return s
}

// SetStatus sets the Status field's value.
func (s *UpdateContainerInstancesStateInput) SetStatus(v string) *UpdateContainerInstancesStateInput {
	s.Status = &v
	return s
}

type UpdateContainerInstancesStateOutput struct {
	_ struct{} `type:"structure"`

	ContainerInstances []*ContainerInstance `locationName:"containerInstances" type:"list"`

	Failures []*Failure `locationName:"failures" type:"list"`
}

// String returns the string representation
func (s UpdateContainerInstancesStateOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UpdateContainerInstancesStateOutput) GoString() string {
	return s.String()
}

// SetContainerInstances sets the ContainerInstances field's value.
func (s *UpdateContainerInstancesStateOutput) SetContainerInstances(v []*ContainerInstance) *UpdateContainerInstancesStateOutput {
	s.ContainerInstances = v
	return s
}

// SetFailures sets the Failures field's value.
func (s *UpdateContainerInstancesStateOutput) SetFailures(v []*Failure) *UpdateContainerInstancesStateOutput {
	s.Failures = v
	return s
}

type UpdateServiceInput struct {
	_ struct{} `type:"structure"`

	Cluster *string `locationName:"cluster" type:"string"`

	DeploymentConfiguration *DeploymentConfiguration `locationName:"deploymentConfiguration" type:"structure"`

	DesiredCount *int64 `locationName:"desiredCount" type:"integer"`

	ForceNewDeployment *bool `locationName:"forceNewDeployment" type:"boolean"`

	HealthCheckGracePeriodSeconds *int64 `locationName:"healthCheckGracePeriodSeconds" type:"integer"`

	NetworkConfiguration *NetworkConfiguration `locationName:"networkConfiguration" type:"structure"`

	PlatformVersion *string `locationName:"platformVersion" type:"string"`

	// Service is a required field
	Service *string `locationName:"service" type:"string" required:"true"`

	TaskDefinition *string `locationName:"taskDefinition" type:"string"`
}

// String returns the string representation
func (s UpdateServiceInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UpdateServiceInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *UpdateServiceInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "UpdateServiceInput"}
	if s.Service == nil {
		invalidParams.Add(request.NewErrParamRequired("Service"))
	}
	if s.NetworkConfiguration != nil {
		if err := s.NetworkConfiguration.Validate(); err != nil {
			invalidParams.AddNested("NetworkConfiguration", err.(request.ErrInvalidParams))
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCluster sets the Cluster field's value.
func (s *UpdateServiceInput) SetCluster(v string) *UpdateServiceInput {
	s.Cluster = &v
	return s
}

// SetDeploymentConfiguration sets the DeploymentConfiguration field's value.
func (s *UpdateServiceInput) SetDeploymentConfiguration(v *DeploymentConfiguration) *UpdateServiceInput {
	s.DeploymentConfiguration = v
	return s
}

// SetDesiredCount sets the DesiredCount field's value.
func (s *UpdateServiceInput) SetDesiredCount(v int64) *UpdateServiceInput {
	s.DesiredCount = &v
	return s
}

// SetForceNewDeployment sets the ForceNewDeployment field's value.
func (s *UpdateServiceInput) SetForceNewDeployment(v bool) *UpdateServiceInput {
	s.ForceNewDeployment = &v
	return s
}

// SetHealthCheckGracePeriodSeconds sets the HealthCheckGracePeriodSeconds field's value.
func (s *UpdateServiceInput) SetHealthCheckGracePeriodSeconds(v int64) *UpdateServiceInput {
	s.HealthCheckGracePeriodSeconds = &v
	return s
}

// SetNetworkConfiguration sets the NetworkConfiguration field's value.
func (s *UpdateServiceInput) SetNetworkConfiguration(v *NetworkConfiguration) *UpdateServiceInput {
	s.NetworkConfiguration = v
	return s
}

// SetPlatformVersion sets the PlatformVersion field's value.
func (s *UpdateServiceInput) SetPlatformVersion(v string) *UpdateServiceInput {
	s.PlatformVersion = &v
	return s
}

// SetService sets the Service field's value.
func (s *UpdateServiceInput) SetService(v string) *UpdateServiceInput {
	s.Service = &v
	return s
}

// SetTaskDefinition sets the TaskDefinition field's value.
func (s *UpdateServiceInput) SetTaskDefinition(v string) *UpdateServiceInput {
	s.TaskDefinition = &v
	return s
}

type UpdateServiceOutput struct {
	_ struct{} `type:"structure"`

	Service *Service `locationName:"service" type:"structure"`
}

// String returns the string representation
func (s UpdateServiceOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UpdateServiceOutput) GoString() string {
	return s.String()
}

// SetService sets the Service field's value.
func (s *UpdateServiceOutput) SetService(v *Service) *UpdateServiceOutput {
	s.Service = v
	return s
}

type UpdateServicePrimaryTaskSetInput struct {
	_ struct{} `type:"structure"`

	// Cluster is a required field
	Cluster *string `locationName:"cluster" type:"string" required:"true"`

	// PrimaryTaskSet is a required field
	PrimaryTaskSet *string `locationName:"primaryTaskSet" type:"string" required:"true"`

	// Service is a required field
	Service *string `locationName:"service" type:"string" required:"true"`
}

// String returns the string representation
func (s UpdateServicePrimaryTaskSetInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UpdateServicePrimaryTaskSetInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *UpdateServicePrimaryTaskSetInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "UpdateServicePrimaryTaskSetInput"}
	if s.Cluster == nil {
		invalidParams.Add(request.NewErrParamRequired("Cluster"))
	}
	if s.PrimaryTaskSet == nil {
		invalidParams.Add(request.NewErrParamRequired("PrimaryTaskSet"))
	}
	if s.Service == nil {
		invalidParams.Add(request.NewErrParamRequired("Service"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCluster sets the Cluster field's value.
func (s *UpdateServicePrimaryTaskSetInput) SetCluster(v string) *UpdateServicePrimaryTaskSetInput {
	s.Cluster = &v
	return s
}

// SetPrimaryTaskSet sets the PrimaryTaskSet field's value.
func (s *UpdateServicePrimaryTaskSetInput) SetPrimaryTaskSet(v string) *UpdateServicePrimaryTaskSetInput {
	s.PrimaryTaskSet = &v
	return s
}

// SetService sets the Service field's value.
func (s *UpdateServicePrimaryTaskSetInput) SetService(v string) *UpdateServicePrimaryTaskSetInput {
	s.Service = &v
	return s
}

type UpdateServicePrimaryTaskSetOutput struct {
	_ struct{} `type:"structure"`

	TaskSet *TaskSet `locationName:"taskSet" type:"structure"`
}

// String returns the string representation
func (s UpdateServicePrimaryTaskSetOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UpdateServicePrimaryTaskSetOutput) GoString() string {
	return s.String()
}

// SetTaskSet sets the TaskSet field's value.
func (s *UpdateServicePrimaryTaskSetOutput) SetTaskSet(v *TaskSet) *UpdateServicePrimaryTaskSetOutput {
	s.TaskSet = v
	return s
}

type UpdateTaskSetInput struct {
	_ struct{} `type:"structure"`

	// Cluster is a required field
	Cluster *string `locationName:"cluster" type:"string" required:"true"`

	// Scale is a required field
	Scale *Scale `locationName:"scale" type:"structure" required:"true"`

	// Service is a required field
	Service *string `locationName:"service" type:"string" required:"true"`

	// TaskSet is a required field
	TaskSet *string `locationName:"taskSet" type:"string" required:"true"`
}

// String returns the string representation
func (s UpdateTaskSetInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UpdateTaskSetInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *UpdateTaskSetInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "UpdateTaskSetInput"}
	if s.Cluster == nil {
		invalidParams.Add(request.NewErrParamRequired("Cluster"))
	}
	if s.Scale == nil {
		invalidParams.Add(request.NewErrParamRequired("Scale"))
	}
	if s.Service == nil {
		invalidParams.Add(request.NewErrParamRequired("Service"))
	}
	if s.TaskSet == nil {
		invalidParams.Add(request.NewErrParamRequired("TaskSet"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetCluster sets the Cluster field's value.
func (s *UpdateTaskSetInput) SetCluster(v string) *UpdateTaskSetInput {
	s.Cluster = &v
	return s
}

// SetScale sets the Scale field's value.
func (s *UpdateTaskSetInput) SetScale(v *Scale) *UpdateTaskSetInput {
	s.Scale = v
	return s
}

// SetService sets the Service field's value.
func (s *UpdateTaskSetInput) SetService(v string) *UpdateTaskSetInput {
	s.Service = &v
	return s
}

// SetTaskSet sets the TaskSet field's value.
func (s *UpdateTaskSetInput) SetTaskSet(v string) *UpdateTaskSetInput {
	s.TaskSet = &v
	return s
}

type UpdateTaskSetOutput struct {
	_ struct{} `type:"structure"`

	TaskSet *TaskSet `locationName:"taskSet" type:"structure"`
}

// String returns the string representation
func (s UpdateTaskSetOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UpdateTaskSetOutput) GoString() string {
	return s.String()
}

// SetTaskSet sets the TaskSet field's value.
func (s *UpdateTaskSetOutput) SetTaskSet(v *TaskSet) *UpdateTaskSetOutput {
	s.TaskSet = v
	return s
}

type VersionInfo struct {
	_ struct{} `type:"structure"`

	AgentHash *string `locationName:"agentHash" type:"string"`

	AgentVersion *string `locationName:"agentVersion" type:"string"`

	DockerVersion *string `locationName:"dockerVersion" type:"string"`
}

// String returns the string representation
func (s VersionInfo) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s VersionInfo) GoString() string {
	return s.String()
}

// SetAgentHash sets the AgentHash field's value.
func (s *VersionInfo) SetAgentHash(v string) *VersionInfo {
	s.AgentHash = &v
	return s
}

// SetAgentVersion sets the AgentVersion field's value.
func (s *VersionInfo) SetAgentVersion(v string) *VersionInfo {
	s.AgentVersion = &v
	return s
}

// SetDockerVersion sets the DockerVersion field's value.
func (s *VersionInfo) SetDockerVersion(v string) *VersionInfo {
	s.DockerVersion = &v
	return s
}

type Volume struct {
	_ struct{} `type:"structure"`

	DockerVolumeConfiguration *DockerVolumeConfiguration `locationName:"dockerVolumeConfiguration" type:"structure"`

	Host *HostVolumeProperties `locationName:"host" type:"structure"`

	Name *string `locationName:"name" type:"string"`
}

// String returns the string representation
func (s Volume) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Volume) GoString() string {
	return s.String()
}

// SetDockerVolumeConfiguration sets the DockerVolumeConfiguration field's value.
func (s *Volume) SetDockerVolumeConfiguration(v *DockerVolumeConfiguration) *Volume {
	s.DockerVolumeConfiguration = v
	return s
}

// SetHost sets the Host field's value.
func (s *Volume) SetHost(v *HostVolumeProperties) *Volume {
	s.Host = v
	return s
}

// SetName sets the Name field's value.
func (s *Volume) SetName(v string) *Volume {
	s.Name = &v
	return s
}

type VolumeFrom struct {
	_ struct{} `type:"structure"`

	ReadOnly *bool `locationName:"readOnly" type:"boolean"`

	SourceContainer *string `locationName:"sourceContainer" type:"string"`
}

// String returns the string representation
func (s VolumeFrom) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s VolumeFrom) GoString() string {
	return s.String()
}

// SetReadOnly sets the ReadOnly field's value.
func (s *VolumeFrom) SetReadOnly(v bool) *VolumeFrom {
	s.ReadOnly = &v
	return s
}

// SetSourceContainer sets the SourceContainer field's value.
func (s *VolumeFrom) SetSourceContainer(v string) *VolumeFrom {
	s.SourceContainer = &v
	return s
}

const (
	// AgentUpdateStatusPending is a AgentUpdateStatus enum value
	AgentUpdateStatusPending = "PENDING"

	// AgentUpdateStatusStaging is a AgentUpdateStatus enum value
	AgentUpdateStatusStaging = "STAGING"

	// AgentUpdateStatusStaged is a AgentUpdateStatus enum value
	AgentUpdateStatusStaged = "STAGED"

	// AgentUpdateStatusUpdating is a AgentUpdateStatus enum value
	AgentUpdateStatusUpdating = "UPDATING"

	// AgentUpdateStatusUpdated is a AgentUpdateStatus enum value
	AgentUpdateStatusUpdated = "UPDATED"

	// AgentUpdateStatusFailed is a AgentUpdateStatus enum value
	AgentUpdateStatusFailed = "FAILED"
)

const (
	// AssignPublicIpEnabled is a AssignPublicIp enum value
	AssignPublicIpEnabled = "ENABLED"

	// AssignPublicIpDisabled is a AssignPublicIp enum value
	AssignPublicIpDisabled = "DISABLED"
)

const (
	// ClusterFieldStatistics is a ClusterField enum value
	ClusterFieldStatistics = "STATISTICS"

	// ClusterFieldTags is a ClusterField enum value
	ClusterFieldTags = "TAGS"
)

const (
	// CompatibilityEc2 is a Compatibility enum value
	CompatibilityEc2 = "EC2"

	// CompatibilityFargate is a Compatibility enum value
	CompatibilityFargate = "FARGATE"
)

const (
	// ConnectivityConnected is a Connectivity enum value
	ConnectivityConnected = "CONNECTED"

	// ConnectivityDisconnected is a Connectivity enum value
	ConnectivityDisconnected = "DISCONNECTED"
)

const (
	// ContainerConditionStart is a ContainerCondition enum value
	ContainerConditionStart = "START"

	// ContainerConditionComplete is a ContainerCondition enum value
	ContainerConditionComplete = "COMPLETE"

	// ContainerConditionSuccess is a ContainerCondition enum value
	ContainerConditionSuccess = "SUCCESS"

	// ContainerConditionHealthy is a ContainerCondition enum value
	ContainerConditionHealthy = "HEALTHY"
)

const (
	// ContainerInstanceFieldTags is a ContainerInstanceField enum value
	ContainerInstanceFieldTags = "TAGS"
)

const (
	// ContainerInstanceStatusActive is a ContainerInstanceStatus enum value
	ContainerInstanceStatusActive = "ACTIVE"

	// ContainerInstanceStatusDraining is a ContainerInstanceStatus enum value
	ContainerInstanceStatusDraining = "DRAINING"
)

const (
	// DeploymentControllerTypeEcs is a DeploymentControllerType enum value
	DeploymentControllerTypeEcs = "ECS"

	// DeploymentControllerTypeCodeDeploy is a DeploymentControllerType enum value
	DeploymentControllerTypeCodeDeploy = "CODE_DEPLOY"

	// DeploymentControllerTypeExternal is a DeploymentControllerType enum value
	DeploymentControllerTypeExternal = "EXTERNAL"
)

const (
	// DesiredStatusRunning is a DesiredStatus enum value
	DesiredStatusRunning = "RUNNING"

	// DesiredStatusPending is a DesiredStatus enum value
	DesiredStatusPending = "PENDING"

	// DesiredStatusStopped is a DesiredStatus enum value
	DesiredStatusStopped = "STOPPED"
)

const (
	// DeviceCgroupPermissionRead is a DeviceCgroupPermission enum value
	DeviceCgroupPermissionRead = "read"

	// DeviceCgroupPermissionWrite is a DeviceCgroupPermission enum value
	DeviceCgroupPermissionWrite = "write"

	// DeviceCgroupPermissionMknod is a DeviceCgroupPermission enum value
	DeviceCgroupPermissionMknod = "mknod"
)

const (
	// HealthStatusHealthy is a HealthStatus enum value
	HealthStatusHealthy = "HEALTHY"

	// HealthStatusUnhealthy is a HealthStatus enum value
	HealthStatusUnhealthy = "UNHEALTHY"

	// HealthStatusUnknown is a HealthStatus enum value
	HealthStatusUnknown = "UNKNOWN"
)

const (
	// IpcModeHost is a IpcMode enum value
	IpcModeHost = "host"

	// IpcModeTask is a IpcMode enum value
	IpcModeTask = "task"

	// IpcModeNone is a IpcMode enum value
	IpcModeNone = "none"
)

const (
	// LaunchTypeEc2 is a LaunchType enum value
	LaunchTypeEc2 = "EC2"

	// LaunchTypeFargate is a LaunchType enum value
	LaunchTypeFargate = "FARGATE"
)

const (
	// LogDriverJsonFile is a LogDriver enum value
	LogDriverJsonFile = "json-file"

	// LogDriverSyslog is a LogDriver enum value
	LogDriverSyslog = "syslog"

	// LogDriverJournald is a LogDriver enum value
	LogDriverJournald = "journald"

	// LogDriverGelf is a LogDriver enum value
	LogDriverGelf = "gelf"

	// LogDriverFluentd is a LogDriver enum value
	LogDriverFluentd = "fluentd"

	// LogDriverAwslogs is a LogDriver enum value
	LogDriverAwslogs = "awslogs"

	// LogDriverSplunk is a LogDriver enum value
	LogDriverSplunk = "splunk"
)

const (
	// NetworkModeBridge is a NetworkMode enum value
	NetworkModeBridge = "bridge"

	// NetworkModeHost is a NetworkMode enum value
	NetworkModeHost = "host"

	// NetworkModeAwsvpc is a NetworkMode enum value
	NetworkModeAwsvpc = "awsvpc"

	// NetworkModeNone is a NetworkMode enum value
	NetworkModeNone = "none"
)

const (
	// PidModeHost is a PidMode enum value
	PidModeHost = "host"

	// PidModeTask is a PidMode enum value
	PidModeTask = "task"
)

const (
	// PlacementConstraintTypeDistinctInstance is a PlacementConstraintType enum value
	PlacementConstraintTypeDistinctInstance = "distinctInstance"

	// PlacementConstraintTypeMemberOf is a PlacementConstraintType enum value
	PlacementConstraintTypeMemberOf = "memberOf"
)

const (
	// PlacementStrategyTypeRandom is a PlacementStrategyType enum value
	PlacementStrategyTypeRandom = "random"

	// PlacementStrategyTypeSpread is a PlacementStrategyType enum value
	PlacementStrategyTypeSpread = "spread"

	// PlacementStrategyTypeBinpack is a PlacementStrategyType enum value
	PlacementStrategyTypeBinpack = "binpack"
)

const (
	// PlatformDeviceTypeGpu is a PlatformDeviceType enum value
	PlatformDeviceTypeGpu = "GPU"
)

const (
	// PropagateTagsTaskDefinition is a PropagateTags enum value
	PropagateTagsTaskDefinition = "TASK_DEFINITION"

	// PropagateTagsService is a PropagateTags enum value
	PropagateTagsService = "SERVICE"
)

const (
	// ProxyConfigurationTypeAppmesh is a ProxyConfigurationType enum value
	ProxyConfigurationTypeAppmesh = "APPMESH"
)

const (
	// ResourceTypeGpu is a ResourceType enum value
	ResourceTypeGpu = "GPU"
)

const (
	// ScaleUnitPercent is a ScaleUnit enum value
	ScaleUnitPercent = "PERCENT"
)

const (
	// SchedulingStrategyReplica is a SchedulingStrategy enum value
	SchedulingStrategyReplica = "REPLICA"

	// SchedulingStrategyDaemon is a SchedulingStrategy enum value
	SchedulingStrategyDaemon = "DAEMON"
)

const (
	// ScopeTask is a Scope enum value
	ScopeTask = "task"

	// ScopeShared is a Scope enum value
	ScopeShared = "shared"
)

const (
	// ServiceFieldTags is a ServiceField enum value
	ServiceFieldTags = "TAGS"
)

const (
	// SettingNameServiceLongArnFormat is a SettingName enum value
	SettingNameServiceLongArnFormat = "serviceLongArnFormat"

	// SettingNameTaskLongArnFormat is a SettingName enum value
	SettingNameTaskLongArnFormat = "taskLongArnFormat"

	// SettingNameContainerInstanceLongArnFormat is a SettingName enum value
	SettingNameContainerInstanceLongArnFormat = "containerInstanceLongArnFormat"
)

const (
	// SortOrderAsc is a SortOrder enum value
	SortOrderAsc = "ASC"

	// SortOrderDesc is a SortOrder enum value
	SortOrderDesc = "DESC"
)

const (
	// StabilityStatusSteadyState is a StabilityStatus enum value
	StabilityStatusSteadyState = "STEADY_STATE"

	// StabilityStatusStabilizing is a StabilityStatus enum value
	StabilityStatusStabilizing = "STABILIZING"
)

const (
	// TargetTypeContainerInstance is a TargetType enum value
	TargetTypeContainerInstance = "container-instance"
)

const (
	// TaskDefinitionFamilyStatusActive is a TaskDefinitionFamilyStatus enum value
	TaskDefinitionFamilyStatusActive = "ACTIVE"

	// TaskDefinitionFamilyStatusInactive is a TaskDefinitionFamilyStatus enum value
	TaskDefinitionFamilyStatusInactive = "INACTIVE"

	// TaskDefinitionFamilyStatusAll is a TaskDefinitionFamilyStatus enum value
	TaskDefinitionFamilyStatusAll = "ALL"
)

const (
	// TaskDefinitionFieldTags is a TaskDefinitionField enum value
	TaskDefinitionFieldTags = "TAGS"
)

const (
	// TaskDefinitionPlacementConstraintTypeMemberOf is a TaskDefinitionPlacementConstraintType enum value
	TaskDefinitionPlacementConstraintTypeMemberOf = "memberOf"
)

const (
	// TaskDefinitionStatusActive is a TaskDefinitionStatus enum value
	TaskDefinitionStatusActive = "ACTIVE"

	// TaskDefinitionStatusInactive is a TaskDefinitionStatus enum value
	TaskDefinitionStatusInactive = "INACTIVE"
)

const (
	// TaskFieldTags is a TaskField enum value
	TaskFieldTags = "TAGS"
)

const (
	// TaskSetFieldTags is a TaskSetField enum value
	TaskSetFieldTags = "TAGS"
)

const (
	// TaskStopCodeTaskFailedToStart is a TaskStopCode enum value
	TaskStopCodeTaskFailedToStart = "TaskFailedToStart"

	// TaskStopCodeEssentialContainerExited is a TaskStopCode enum value
	TaskStopCodeEssentialContainerExited = "EssentialContainerExited"

	// TaskStopCodeUserInitiated is a TaskStopCode enum value
	TaskStopCodeUserInitiated = "UserInitiated"
)

const (
	// TransportProtocolTcp is a TransportProtocol enum value
	TransportProtocolTcp = "tcp"

	// TransportProtocolUdp is a TransportProtocol enum value
	TransportProtocolUdp = "udp"
)

const (
	// UlimitNameCore is a UlimitName enum value
	UlimitNameCore = "core"

	// UlimitNameCpu is a UlimitName enum value
	UlimitNameCpu = "cpu"

	// UlimitNameData is a UlimitName enum value
	UlimitNameData = "data"

	// UlimitNameFsize is a UlimitName enum value
	UlimitNameFsize = "fsize"

	// UlimitNameLocks is a UlimitName enum value
	UlimitNameLocks = "locks"

	// UlimitNameMemlock is a UlimitName enum value
	UlimitNameMemlock = "memlock"

	// UlimitNameMsgqueue is a UlimitName enum value
	UlimitNameMsgqueue = "msgqueue"

	// UlimitNameNice is a UlimitName enum value
	UlimitNameNice = "nice"

	// UlimitNameNofile is a UlimitName enum value
	UlimitNameNofile = "nofile"

	// UlimitNameNproc is a UlimitName enum value
	UlimitNameNproc = "nproc"

	// UlimitNameRss is a UlimitName enum value
	UlimitNameRss = "rss"

	// UlimitNameRtprio is a UlimitName enum value
	UlimitNameRtprio = "rtprio"

	// UlimitNameRttime is a UlimitName enum value
	UlimitNameRttime = "rttime"

	// UlimitNameSigpending is a UlimitName enum value
	UlimitNameSigpending = "sigpending"

	// UlimitNameStack is a UlimitName enum value
	UlimitNameStack = "stack"
)
