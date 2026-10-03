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
	"strconv"

	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/utils"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/pkg/errors"
)

const (
	BlackHolePortFaultType   = "network-blackhole-port"
	LatencyFaultType         = "network-latency"
	PacketLossFaultType      = "network-packet-loss"
	StartNetworkFaultPostfix = "start"
	StopNetworkFaultPostfix  = "stop"
	CheckNetworkFaultPostfix = "status"
	AddNetworkFaultPostfix   = "add-sources"
	// AddSourcesMaxIPs caps the total number of entries (Sources + SourcesToFilter)
	// accepted by a single add-sources request. The cap keeps a well-formed call
	// inside the shared requestTimeoutSeconds budget and forces callers to keep
	// one refresh call per domain. Route53 returns at most 8 IPs per answer, so
	// 16 leaves headroom for two-domain bundles without requiring a split.
	AddSourcesMaxIPs   = 16
	TrafficTypeIngress = "ingress"
	TrafficTypeEgress  = "egress"
	// Request Payload Errors
	MissingRequiredFieldError = "required parameter %s is missing"
	MissingRequestBodyError   = "required request body is missing"
	ZeroDelayAndJitterError   = "required either DelayMilliseconds or JitterMilliseconds to be non-zero"
	InvalidValueError         = "invalid value %s for parameter %s"
	// AddSources-specific errors. "Sources" and "SourcesToFilter" are JSON
	// field names on NetworkFaultAddSourcesRequest, not English nouns; the
	// capitalization lets callers grep from error back to field.
	AddSourcesTooManyIPsError = "Sources and SourcesToFilter together have %d entries, maximum allowed is %d"
	AddSourcesEmptyError      = "at least one of Sources or SourcesToFilter must be non-empty"
	AddSourcesOverlapError    = "%s is present in both Sources and SourcesToFilter"
)

type NetworkFaultRequest interface {
	ValidateRequest() error
	ToString() string
}

type NetworkBlackholePortRequest struct {
	Port        *uint16 `json:"Port"`
	Protocol    *string `json:"Protocol"`
	TrafficType *string `json:"TrafficType"`
	// SourcesToFilter is a list including IPv4/IPv6 addresses or IPv4/IPv6 CIDR blocks that will be excluded
	// from the fault.
	SourcesToFilter []*string `json:"SourcesToFilter,omitempty"`
}

type NetworkFaultInjectionResponse struct {
	Status string `json:"Status,omitempty"`
	Error  string `json:"Error,omitempty"`
}

func (request NetworkBlackholePortRequest) ValidateRequest() error {
	if request.Port == nil {
		return fmt.Errorf(MissingRequiredFieldError, "Port")
	}
	if request.Protocol == nil || *request.Protocol == "" {
		return fmt.Errorf(MissingRequiredFieldError, "Protocol")
	}
	if request.TrafficType == nil || *request.TrafficType == "" {
		return fmt.Errorf(MissingRequiredFieldError, "TrafficType")
	}

	if *request.Protocol != "tcp" && *request.Protocol != "udp" {
		return fmt.Errorf(InvalidValueError, *request.Protocol, "Protocol")
	}

	if *request.TrafficType != TrafficTypeIngress && *request.TrafficType != TrafficTypeEgress {
		return fmt.Errorf(InvalidValueError, *request.TrafficType, "TrafficType")
	}
	if err := requireIPInRequestSources(request.SourcesToFilter, "SourcesToFilter"); err != nil {
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
		if aws.ToString(src) == source {
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
	FlowsPercent    *int      `json:"FlowsPercent"`
}

// ValidateRequest validates required fields are present and its value.
func (request NetworkLatencyRequest) ValidateRequest() error {
	if request.DelayMilliseconds == nil {
		return fmt.Errorf(MissingRequiredFieldError, "DelayMilliseconds")
	}
	if request.JitterMilliseconds == nil {
		return fmt.Errorf(MissingRequiredFieldError, "JitterMilliseconds")
	}

	if aws.ToUint64(request.DelayMilliseconds) == 0 && aws.ToUint64(request.JitterMilliseconds) == 0 {
		return errors.New(ZeroDelayAndJitterError)
	}

	if len(request.Sources) == 0 {
		return fmt.Errorf(MissingRequiredFieldError, "Sources")
	}
	if err := requireIPInRequestSources(request.Sources, "Sources"); err != nil {
		return err
	}
	if err := requireIPInRequestSources(request.SourcesToFilter, "SourcesToFilter"); err != nil {
		return err
	}

	if request.FlowsPercent != nil {
		flowsPercent := aws.ToInt(request.FlowsPercent)
		if flowsPercent <= 0 || flowsPercent > 100 {
			return fmt.Errorf(InvalidValueError, strconv.Itoa(flowsPercent), "flowsPercent")
		}
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
	FlowsPercent    *int      `json:"FlowsPercent"`
}

// ValidateRequest validates required fields are present and its value.
func (request NetworkPacketLossRequest) ValidateRequest() error {
	if request.LossPercent == nil {
		return fmt.Errorf(MissingRequiredFieldError, "LossPercent")
	}
	// request.LossPercent should be an integer between 1 and 100 (inclusive).
	if *request.LossPercent < 1 || *request.LossPercent > 100 {
		return fmt.Errorf(InvalidValueError, strconv.Itoa(int(*request.LossPercent)), "LossPercent")
	}
	if len(request.Sources) == 0 {
		return fmt.Errorf(MissingRequiredFieldError, "Sources")
	}
	if err := requireIPInRequestSources(request.Sources, "Sources"); err != nil {
		return err
	}
	if err := requireIPInRequestSources(request.SourcesToFilter, "SourcesToFilter"); err != nil {
		return err
	}

	if request.FlowsPercent != nil {
		flowsPercent := aws.ToInt(request.FlowsPercent)
		if flowsPercent <= 0 || flowsPercent > 100 {
			return fmt.Errorf(InvalidValueError, strconv.Itoa(flowsPercent), "flowsPercent")
		}
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

// requireIPInRequestSources requires each source is IPv4/IPv6 or IPv4/IPv6 CIDR block.
func requireIPInRequestSources(sources []*string, sourcesType string) error {
	for _, element := range sources {
		if err := requireIPInRequestSource(aws.ToString(element), sourcesType); err != nil {
			return err
		}
	}
	return nil
}

// requireIPInRequestSource requires the source is IPv4/IPv6 or IPv4/IPv6 CIDR block.
func requireIPInRequestSource(source string, sourceType string) error {
	if utils.IsIPv4(source) ||
		utils.IsIPv6(source) ||
		utils.IsIPv4CIDR(source) ||
		utils.IsIPv6CIDR(source) {
		return nil
	}

	return fmt.Errorf(InvalidValueError, source, sourceType)
}

// NetworkFaultAddSourcesRequest is the request body for the add-sources endpoint
// used to push additional IPs into an already-running tc fault. It carries only
// the IP lists because the fault parameters (delay, jitter, loss percent, flows
// percent) are locked in at start time and cannot be changed mid-fault.
type NetworkFaultAddSourcesRequest struct {
	// Sources is a list of IPv4/IPv6 addresses or IPv4/IPv6 CIDR blocks to add
	// as tc filters for the impaired flow.
	Sources []*string `json:"Sources"`
	// SourcesToFilter is a list of IPv4/IPv6 addresses or IPv4/IPv6 CIDR blocks
	// to add as allowlist filters that bypass the impairment.
	SourcesToFilter []*string `json:"SourcesToFilter,omitempty"`
}

// ValidateRequest enforces the add-sources contract: at least one IP across the
// two lists, every entry is a valid IPv4/IPv6 address or CIDR block, the two
// lists are disjoint within this request, and the combined size fits under
// AddSourcesMaxIPs. Cross-request overlap is the SSM script's responsibility;
// tracking it here would require per-namespace state and additional tc queries.
func (request NetworkFaultAddSourcesRequest) ValidateRequest() error {
	if len(request.Sources) == 0 && len(request.SourcesToFilter) == 0 {
		return errors.New(AddSourcesEmptyError)
	}
	total := len(request.Sources) + len(request.SourcesToFilter)
	if total > AddSourcesMaxIPs {
		return fmt.Errorf(AddSourcesTooManyIPsError, total, AddSourcesMaxIPs)
	}
	if err := requireIPInRequestSources(request.Sources, "Sources"); err != nil {
		return err
	}
	if err := requireIPInRequestSources(request.SourcesToFilter, "SourcesToFilter"); err != nil {
		return err
	}
	// Reject within-request overlap. Any IP that appears in both lists is
	// ambiguous: the caller cannot want the same address on both flowid 1:1
	// (impaired) and flowid 1:3 (allowlist). 400 surfaces the mistake instead
	// of installing conflicting filters.
	filterSet := make(map[string]struct{}, len(request.SourcesToFilter))
	for _, ipPtr := range request.SourcesToFilter {
		filterSet[aws.ToString(ipPtr)] = struct{}{}
	}
	for _, ipPtr := range request.Sources {
		ip := aws.ToString(ipPtr)
		if _, found := filterSet[ip]; found {
			return fmt.Errorf(AddSourcesOverlapError, ip)
		}
	}
	return nil
}

func (request NetworkFaultAddSourcesRequest) ToString() string {
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Sprintf("Error: Unable to parse add-sources request with error %v.", err)
	}
	return string(data)
}
