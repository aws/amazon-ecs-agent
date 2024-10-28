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
	"net"
	"strconv"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/aws-sdk-go/aws"
)

const (
	BlackHolePortFaultType    = "network-blackhole-port"
	LatencyFaultType          = "network-latency"
	PacketLossFaultType       = "network-packet-loss"
	StartNetworkFaultPostfix  = "start"
	StopNetworkFaultPostfix   = "stop"
	CheckNetworkFaultPostfix  = "status"
	missingRequiredFieldError = "required parameter %s is missing"
	MissingRequestBodyError   = "required request body is missing"
	invalidValueError         = "invalid value %s for parameter %s"
	TrafficTypeIngress        = "ingress"
	TrafficTypeEgress         = "egress"
)

type NetworkFaultRequest interface {
	ValidateRequest() error
	ToString() string
}

type NetworkBlackholePortRequest struct {
	Port        *uint16 `json:"Port"`
	Protocol    *string `json:"Protocol"`
	TrafficType *string `json:"TrafficType"`
	// SourcesToFilter is a list including IPv4 addresses or IPv4 CIDR blocks that will be excluded
	// from the fault.
	SourcesToFilter []*string `json:"SourcesToFilter,omitempty"`
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

	if *request.TrafficType != TrafficTypeIngress && *request.TrafficType != TrafficTypeEgress {
		return fmt.Errorf(invalidValueError, *request.TrafficType, "TrafficType")
	}
	if err := validateNetworkFaultRequestSources(request.SourcesToFilter, "SourcesToFilter"); err != nil {
		return err
	}

	return nil
}

// Adds a source to SourcesToFilter
func (request *NetworkBlackholePortRequest) AddSourceToFilterIfNotAlready(source string) {
	if request.SourcesToFilter == nil {
		request.SourcesToFilter = []*string{}
	}
	for _, src := range request.SourcesToFilter {
		if aws.StringValue(src) == source {
			return
		}
	}
	request.SourcesToFilter = append(request.SourcesToFilter, aws.String(source))
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

// NetworkLatencyRequest is struct for the network latency fault request.
type NetworkLatencyRequest struct {
	DelayMilliseconds  *uint64 `json:"DelayMilliseconds"`
	JitterMilliseconds *uint64 `json:"JitterMilliseconds"`
	// Sources is a list including IPv4 addresses or IPv4 CIDR blocks.
	Sources []*string `json:"Sources"`
	// SourcesToFilter is a list including IPv4 addresses or IPv4 CIDR blocks that will be excluded from the
	// network latency fault.
	SourcesToFilter []*string `json:"SourcesToFilter,omitempty"`
}

// ValidateRequest validates required fields are present and its value.
func (request NetworkLatencyRequest) ValidateRequest() error {
	if request.DelayMilliseconds == nil {
		return fmt.Errorf(missingRequiredFieldError, "DelayMilliseconds")
	}
	if request.JitterMilliseconds == nil {
		return fmt.Errorf(missingRequiredFieldError, "JitterMilliseconds")
	}
	if request.Sources == nil || len(request.Sources) == 0 {
		return fmt.Errorf(missingRequiredFieldError, "Sources")
	}
	if err := validateNetworkFaultRequestSources(request.Sources, "Sources"); err != nil {
		return err
	}
	if err := validateNetworkFaultRequestSources(request.SourcesToFilter, "SourcesToFilter"); err != nil {
		return err
	}
	return nil
}

func (request NetworkLatencyRequest) ToString() string {
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Sprintf("Error: Unable to parse %s request with error %v.", LatencyFaultType, err)
	}
	return string(data)
}

// NetworkPacketLossRequest is struct for the network packet loss fault request.
type NetworkPacketLossRequest struct {
	LossPercent *uint64 `json:"LossPercent"`
	// Sources is a list including IPv4 addresses or IPv4 CIDR blocks.
	Sources []*string `json:"Sources"`
	// SourcesToFilter is a list including IPv4 addresses or IPv4 CIDR blocks that will be excluded from the
	// network packet loss fault.
	SourcesToFilter []*string `json:"SourcesToFilter,omitempty"`
}

// ValidateRequest validates required fields are present and its value.
func (request NetworkPacketLossRequest) ValidateRequest() error {
	if request.LossPercent == nil {
		return fmt.Errorf(missingRequiredFieldError, "LossPercent")
	}
	// request.LossPercent should be an integer between 1 and 100 (inclusive).
	if *request.LossPercent < 1 || *request.LossPercent > 100 {
		return fmt.Errorf(invalidValueError, strconv.Itoa(int(*request.LossPercent)), "LossPercent")
	}
	if request.Sources == nil || len(request.Sources) == 0 {
		return fmt.Errorf(missingRequiredFieldError, "Sources")
	}
	if err := validateNetworkFaultRequestSources(request.Sources, "Sources"); err != nil {
		return err
	}
	if err := validateNetworkFaultRequestSources(request.SourcesToFilter, "SourcesToFilter"); err != nil {
		return err
	}
	return nil
}

func (request NetworkPacketLossRequest) ToString() string {
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Sprintf("Error: Unable to parse %s request with error %v.", PacketLossFaultType, err)
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

func validateNetworkFaultRequestSources(sources []*string, sourcesType string) error {
	for _, element := range sources {
		if err := validateNetworkFaultRequestSource(aws.StringValue(element), sourcesType); err != nil {
			return err
		}
	}
	return nil
}

func validateNetworkFaultRequestSource(source string, sourceType string) error {
	ip := net.ParseIP(source)
	if ip != nil && ip.To4() != nil {
		return nil // IPv4 successful
	}

	_, ipnet, err := net.ParseCIDR(source)
	if err == nil && ipnet.IP.To4() != nil {
		return nil // IPv4 CIDR successful
	}
	if err != nil {
		logger.Info("Failed to parse fault source as IPv4 CIDR block", logger.Fields{
			"source":    source,
			field.Error: err,
		})
	}

	return fmt.Errorf(invalidValueError, source, sourceType)
}
