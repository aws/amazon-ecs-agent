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

package types

import (
	"encoding/json"
	"fmt"
)

const (
	BlackHolePortFaultType    = "network-blackhole-port"
	LatencyFaultType          = "network-latency"
	PacketLossFaultType       = "network-packet-loss"
	missingRequiredFieldError = "required parameter %s is missing"
	invalidValueError         = "invalid value %s for parameter %s"
)

type NetworkFaultRequest interface {
	ValidateRequest() error
	ToString() string
}

type NetworkBlackholePortRequest struct {
	Port        *uint16 `json:"Port"`
	Protocol    *string `json:"Protocol"`
	TrafficType *string `json:"TrafficType"`
}

type NetworkFaultInjectionResponse struct {
	Status string `json:"Status,omitempty"`
	Error  string `json:"Error,omitempty"`
}

func (request NetworkBlackholePortRequest) ValidateRequest() error {
	if request.Port == nil {
		return fmt.Errorf(missingRequiredFieldError, "Port")
	}
	if request.Protocol == nil || *request.Protocol == "" {
		return fmt.Errorf(missingRequiredFieldError, "Protocol")
	}
	if request.TrafficType == nil || *request.TrafficType == "" {
		return fmt.Errorf(missingRequiredFieldError, "TrafficType")
	}

	if *request.Protocol != "tcp" && *request.Protocol != "udp" {
		return fmt.Errorf(invalidValueError, *request.Protocol, "Protocol")
	}

	if *request.TrafficType != "ingress" && *request.TrafficType != "egress" {
		return fmt.Errorf(invalidValueError, *request.TrafficType, "TrafficType")
	}

	return nil
}

func (request NetworkBlackholePortRequest) ToString() string {
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Sprintf("Error: Unable to parse %s request with error %v.", BlackHolePortFaultType, err)
	}
	return string(data)
}

func (response NetworkFaultInjectionResponse) ToString() string {
	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Sprintf("Error: Unable to parse network fault injection response with error %v.", err)
	}
	return string(data)
}

func NewNetworkFaultInjectionSuccessResponse(status string) NetworkFaultInjectionResponse {
	return NetworkFaultInjectionResponse{
		Status: status,
	}
}

func NewNetworkFaultInjectionErrorResponse(err string) NetworkFaultInjectionResponse {
	return NetworkFaultInjectionResponse{
		Error: err,
	}
}
