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

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/fault/v1/types"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
	v4 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4"
	state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/execwrapper"
	"github.com/aws/aws-sdk-go/aws"

	"github.com/gorilla/mux"
)

const (
	startFaultRequestType       = "start %s"
	stopFaultRequestType        = "stop %s"
	checkStatusFaultRequestType = "check status %s"
	invalidNetworkModeError     = "%s mode is not supported. Please use either host or awsvpc mode."
	faultInjectionEnabledError  = "enableFaultInjection is not enabled for task: %s"
	requestTimedOutError        = "%s: request timed out"
	requestTimeoutDuration      = 5 * time.Second
)

var (
	iptablesChainExistCmd         = "iptables -C %s -p %s --dport %s -j DROP"
	nsenterCommandString          = "nsenter --net=%s "
	tcCheckInjectionCommandString = "tc -j q show dev %s parent 1:1"
	tcAddQdiscRootCommandString   = "tc qdisc add dev %s root handle 1: prio priomap 2 2 2 2 2 2 2 2 2 2 2 2 2 2 2 2"
	tcAddQdiscLossCommandString   = "tc qdisc add dev %s parent 1:1 handle 10: netem loss %d%%"
	tcAddFilterForIPCommandString = "tc filter add dev %s protocol ip parent 1:0 prio 1 u32 match ip dst %s flowid 1:1"
)

type FaultHandler struct {
	// mutexMap is used to avoid multiple clients to manipulate same resource at same
	// time. The 'key' is the the network namespace path and 'value' is the RWMutex.
	// Using concurrent map here because the handler is shared by all requests.
	mutexMap       sync.Map
	AgentState     state.AgentState
	MetricsFactory metrics.EntryFactory
	osExecWrapper  execwrapper.Exec
}

func New(agentState state.AgentState, mf metrics.EntryFactory, execWrapper execwrapper.Exec) *FaultHandler {
	return &FaultHandler{
		AgentState:     agentState,
		MetricsFactory: mf,
		mutexMap:       sync.Map{},
		osExecWrapper:  execWrapper,
	}
}

// NetworkFaultPath will take in a fault type and return the TMDS endpoint path
func NetworkFaultPath(fault string) string {
	return fmt.Sprintf("/api/%s/fault/v1/%s",
		utils.ConstructMuxVar(v4.EndpointContainerIDMuxName, utils.AnythingButSlashRegEx), fault)
}

// loadLock returns the lock associated with given key.
func (h *FaultHandler) loadLock(key string) *sync.RWMutex {
	mu := new(sync.RWMutex)
	actualMu, _ := h.mutexMap.LoadOrStore(key, mu)
	return actualMu.(*sync.RWMutex)
}

// StartNetworkBlackholePort will return the request handler function for starting a network blackhole port fault
func (h *FaultHandler) StartNetworkBlackholePort() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var request types.NetworkBlackholePortRequest
		requestType := fmt.Sprintf(startFaultRequestType, types.BlackHolePortFaultType)

		// Parse the fault request
		err := decodeRequest(w, &request, requestType, r)
		if err != nil {
			return
		}
		// Validate the fault request
		err = validateRequest(w, request, requestType)
		if err != nil {
			return
		}

		// Obtain the task metadata via the endpoint container ID
		taskMetadata, err := validateTaskMetadata(w, h.AgentState, requestType, r)
		if err != nil {
			return
		}

		// To avoid multiple requests to manipulate same network resource
		networkNSPath := taskMetadata.TaskNetworkConfig.NetworkNamespaces[0].Path
		rwMu := h.loadLock(networkNSPath)
		rwMu.Lock()
		defer rwMu.Unlock()

		// TODO: Check status of current fault injection
		// TODO: Invoke the start fault injection functionality if not running

		responseBody := types.NewNetworkFaultInjectionSuccessResponse("running")
		logger.Info("Successfully started fault", logger.Fields{
			field.RequestType: requestType,
			field.Request:     request.ToString(),
			field.Response:    responseBody.ToString(),
		})
		utils.WriteJSONResponse(
			w,
			http.StatusOK,
			responseBody,
			requestType,
		)
	}
}

// StopNetworkBlackHolePort will return the request handler function for stopping a network blackhole port fault
func (h *FaultHandler) StopNetworkBlackHolePort() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var request types.NetworkBlackholePortRequest
		requestType := fmt.Sprintf(stopFaultRequestType, types.BlackHolePortFaultType)

		// Parse the fault request
		err := decodeRequest(w, &request, requestType, r)
		if err != nil {
			return
		}
		// Validate the fault request
		err = validateRequest(w, request, requestType)
		if err != nil {
			return
		}

		logger.Debug("Successfully parsed fault request payload", logger.Fields{
			field.Request: request.ToString(),
		})

		// Obtain the task metadata via the endpoint container ID
		taskMetadata, err := validateTaskMetadata(w, h.AgentState, requestType, r)
		if err != nil {
			return
		}

		// To avoid multiple requests to manipulate same network resource
		networkNSPath := taskMetadata.TaskNetworkConfig.NetworkNamespaces[0].Path
		rwMu := h.loadLock(networkNSPath)
		rwMu.Lock()
		defer rwMu.Unlock()

		// TODO: Check status of current fault injection
		// TODO: Invoke the stop fault injection functionality if running

		responseBody := types.NewNetworkFaultInjectionSuccessResponse("stopped")
		logger.Info("Successfully stopped fault", logger.Fields{
			field.RequestType: requestType,
			field.Request:     request.ToString(),
			field.Response:    responseBody.ToString(),
		})
		utils.WriteJSONResponse(
			w,
			http.StatusOK,
			responseBody,
			requestType,
		)
	}
}

// CheckNetworkBlackHolePort will return the request handler function for checking the status of a network blackhole port fault
func (h *FaultHandler) CheckNetworkBlackHolePort() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var request types.NetworkBlackholePortRequest
		requestType := fmt.Sprintf(checkStatusFaultRequestType, types.BlackHolePortFaultType)

		// Parse the fault request
		err := decodeRequest(w, &request, requestType, r)
		if err != nil {
			return
		}
		// Validate the fault request
		err = validateRequest(w, request, requestType)
		if err != nil {
			return
		}

		logger.Debug("Successfully parsed fault request payload", logger.Fields{
			field.Request: request.ToString(),
		})

		// Obtain the task metadata via the endpoint container ID
		taskMetadata, err := validateTaskMetadata(w, h.AgentState, requestType, r)
		if err != nil {
			return
		}

		// To avoid multiple requests to manipulate same network resource
		networkNSPath := taskMetadata.TaskNetworkConfig.NetworkNamespaces[0].Path
		rwMu := h.loadLock(networkNSPath)
		rwMu.RLock()
		defer rwMu.RUnlock()

		ctx := context.Background()
		ctxWithTimeout, cancel := context.WithTimeout(ctx, requestTimeoutDuration)
		defer cancel()

		var responseBody types.NetworkFaultInjectionResponse
		var statusCode int
		port := strconv.FormatUint(uint64(aws.Uint16Value(request.Port)), 10)
		chainName := fmt.Sprintf("%s-%s-%s", aws.StringValue(request.TrafficType), aws.StringValue(request.Protocol), port)
		running, cmdOutput, cmdErr := h.checkNetworkBlackHolePort(ctxWithTimeout, aws.StringValue(request.Protocol), port, chainName,
			taskMetadata.TaskNetworkConfig.NetworkMode, taskMetadata.TaskNetworkConfig.NetworkNamespaces[0].Path)

		// We've timed out trying to check if the black hole port fault injection is running
		if err := ctx.Err(); err == context.DeadlineExceeded {
			logger.Error("Request timed out", logger.Fields{
				field.RequestType: requestType,
				field.Request:     request.ToString(),
				field.Error:       err,
			})
			statusCode = http.StatusInternalServerError
			responseBody = types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf(requestTimedOutError, requestType))
		} else if cmdErr != nil {
			logger.Error("Unknown error encountered for request", logger.Fields{
				field.RequestType: requestType,
				field.Request:     request.ToString(),
				field.Error:       cmdErr,
			})
			statusCode = http.StatusInternalServerError
			responseBody = types.NewNetworkFaultInjectionErrorResponse(cmdOutput)
		} else {
			statusCode = http.StatusOK
			if running {
				responseBody = types.NewNetworkFaultInjectionSuccessResponse("running")
			} else {
				responseBody = types.NewNetworkFaultInjectionSuccessResponse("not-running")
			}
			logger.Info("[INFO] Successfully checked status for fault", logger.Fields{
				field.RequestType: requestType,
				field.Request:     request.ToString(),
				field.Response:    responseBody.ToString(),
			})
		}
		utils.WriteJSONResponse(
			w,
			statusCode,
			responseBody,
			requestType,
		)
	}
}

// checkNetworkBlackHolePort will check if there's a running black hole port within the task network namespace based on the chain name and the passed in required request fields.
// It does so by calling iptables linux utility tool.
func (h *FaultHandler) checkNetworkBlackHolePort(ctx context.Context, protocol, port, chain, networkMode, netNs string) (bool, string, error) {
	cmdString := fmt.Sprintf(iptablesChainExistCmd, chain, protocol, port)
	cmdList := strings.Split(cmdString, " ")

	// For host mode, the task network namespace is the host network namespace (i.e. we don't need to run nsenter)
	if networkMode != ecs.NetworkModeHost {
		cmdList = append(strings.Split(fmt.Sprintf(nsenterCommandString, netNs), " "), cmdList...)
	}

	cmdOutput, err := h.runExecCommand(ctx, cmdList)
	if err != nil {
		if exitErr, eok := h.osExecWrapper.ConvertToExitError(err); eok {
			logger.Info("[INFO] Black hole port fault is not running", logger.Fields{
				"netns":    netNs,
				"command":  strings.Join(cmdList, " "),
				"output":   string(cmdOutput),
				"exitCode": h.osExecWrapper.GetExitCode(exitErr),
			})
			return false, string(cmdOutput), nil
		}
		logger.Error("Error: Unable to check status of black hole port fault", logger.Fields{
			"netns":   netNs,
			"command": strings.Join(cmdList, " "),
			"output":  string(cmdOutput),
			"err":     err,
		})
		return false, string(cmdOutput), err
	}
	logger.Info("[INFO] Black hole port fault has been found running", logger.Fields{
		"netns":   netNs,
		"command": strings.Join(cmdList, " "),
		"output":  string(cmdOutput),
	})
	return true, string(cmdOutput), nil
}

// StartNetworkLatency starts a network latency fault in the associated ENI if no existing same fault.
func (h *FaultHandler) StartNetworkLatency() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var request types.NetworkLatencyRequest
		requestType := fmt.Sprintf(startFaultRequestType, types.LatencyFaultType)
		// Parse the fault request
		err := decodeRequest(w, &request, requestType, r)
		if err != nil {
			return
		}

		// Validate the fault request
		err = validateRequest(w, request, requestType)
		if err != nil {
			return
		}

		// Obtain the task metadata via the endpoint container ID
		taskMetadata, err := validateTaskMetadata(w, h.AgentState, requestType, r)
		if err != nil {
			return
		}

		// To avoid multiple requests to manipulate same network resource
		networkNSPath := taskMetadata.TaskNetworkConfig.NetworkNamespaces[0].Path
		rwMu := h.loadLock(networkNSPath)
		rwMu.Lock()
		defer rwMu.Unlock()

		// TODO: Check status of current fault injection
		// TODO: Invoke the start fault injection functionality if not running

		responseBody := types.NewNetworkFaultInjectionSuccessResponse("running")
		logger.Info("Successfully started fault", logger.Fields{
			field.RequestType: requestType,
			field.Request:     request.ToString(),
			field.Response:    responseBody.ToString(),
		})
		utils.WriteJSONResponse(
			w,
			http.StatusOK,
			responseBody,
			requestType,
		)
	}
}

// StopNetworkLatency stops a network latency fault in the associated ENI if there is one existing same fault.
func (h *FaultHandler) StopNetworkLatency() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var request types.NetworkLatencyRequest
		requestType := fmt.Sprintf(stopFaultRequestType, types.LatencyFaultType)

		// Parse the fault request
		err := decodeRequest(w, &request, requestType, r)
		if err != nil {
			return
		}
		// Validate the fault request
		err = validateRequest(w, request, requestType)
		if err != nil {
			return
		}

		// Obtain the task metadata via the endpoint container ID
		taskMetadata, err := validateTaskMetadata(w, h.AgentState, requestType, r)
		if err != nil {
			return
		}

		// To avoid multiple requests to manipulate same network resource
		networkNSPath := taskMetadata.TaskNetworkConfig.NetworkNamespaces[0].Path
		rwMu := h.loadLock(networkNSPath)
		rwMu.Lock()
		defer rwMu.Unlock()

		// TODO: Check status of current fault injection
		// TODO: Invoke the stop fault injection functionality if running

		responseBody := types.NewNetworkFaultInjectionSuccessResponse("stopped")
		logger.Info("Successfully stopped fault", logger.Fields{
			field.RequestType: requestType,
			field.Request:     request.ToString(),
			field.Response:    responseBody.ToString(),
		})
		utils.WriteJSONResponse(
			w,
			http.StatusOK,
			responseBody,
			requestType,
		)
	}
}

// CheckNetworkLatency checks the status of given network latency fault.
func (h *FaultHandler) CheckNetworkLatency() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var request types.NetworkLatencyRequest
		requestType := fmt.Sprintf(checkStatusFaultRequestType, types.LatencyFaultType)

		// Parse the fault request
		err := decodeRequest(w, &request, requestType, r)
		if err != nil {
			return
		}
		// Validate the fault request
		err = validateRequest(w, request, requestType)
		if err != nil {
			return
		}

		// Obtain the task metadata via the endpoint container ID
		taskMetadata, err := validateTaskMetadata(w, h.AgentState, requestType, r)
		if err != nil {
			return
		}

		// To avoid multiple requests to manipulate same network resource
		networkNSPath := taskMetadata.TaskNetworkConfig.NetworkNamespaces[0].Path
		rwMu := h.loadLock(networkNSPath)
		rwMu.RLock()
		defer rwMu.RUnlock()

		// TODO: Check status of current fault injection
		// TODO: Return the correct status state
		responseBody := types.NewNetworkFaultInjectionSuccessResponse("running")
		logger.Info("Successfully checked status for fault", logger.Fields{
			field.RequestType: requestType,
			field.Request:     request.ToString(),
			field.Response:    responseBody.ToString(),
		})
		utils.WriteJSONResponse(
			w,
			http.StatusOK,
			responseBody,
			requestType,
		)
	}
}

// StartNetworkPacketLoss starts a network packet loss fault in the associated ENI if no existing same fault.
func (h *FaultHandler) StartNetworkPacketLoss() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var request types.NetworkPacketLossRequest
		requestType := fmt.Sprintf(startFaultRequestType, types.PacketLossFaultType)
		// Parse the fault request
		err := decodeRequest(w, &request, requestType, r)
		if err != nil {
			return
		}

		// Validate the fault request
		err = validateRequest(w, request, requestType)
		if err != nil {
			return
		}

		// Obtain the task metadata via the endpoint container ID
		taskMetadata, err := validateTaskMetadata(w, h.AgentState, requestType, r)
		if err != nil {
			return
		}

		// To avoid multiple requests to manipulate same network resource
		networkNSPath := taskMetadata.TaskNetworkConfig.NetworkNamespaces[0].Path
		rwMu := h.loadLock(networkNSPath)
		rwMu.Lock()
		defer rwMu.Unlock()

		var responseBody types.NetworkFaultInjectionResponse
		var httpStatusCode int
		stringToBeLogged := "Failed to start fault"
		// All command executions for the start network packet loss workflow all together should finish within 5 seconds.
		// Thus, create the context here so that it can be shared by all os/exec calls.
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeoutDuration)
		defer cancel()
		// Check the status of current fault injection.
		latencyFaultExists, packetLossFaultExists, err := h.checkTCFault(ctx, taskMetadata)
		if err != nil {
			responseBody = types.NewNetworkFaultInjectionErrorResponse(err.Error())
			httpStatusCode = http.StatusInternalServerError
		} else {
			// If there already exists a fault in the task network namespace.
			if latencyFaultExists {
				responseBody = types.NewNetworkFaultInjectionErrorResponse("There is already one network latency fault running")
				httpStatusCode = http.StatusConflict
			} else if packetLossFaultExists {
				responseBody = types.NewNetworkFaultInjectionErrorResponse("There is already one network packet loss fault running")
				httpStatusCode = http.StatusConflict
			} else {
				// Invoke the start fault injection functionality if not running.
				err := h.startNetworkPacketLossFault(ctx, taskMetadata, request)
				if err != nil {
					responseBody = types.NewNetworkFaultInjectionErrorResponse("Failed to inject network-packet-loss fault")
					httpStatusCode = http.StatusInternalServerError
				} else {
					stringToBeLogged = "Successfully started fault"
					responseBody = types.NewNetworkFaultInjectionSuccessResponse("running")
					httpStatusCode = http.StatusOK
				}
			}
		}
		logger.Info(stringToBeLogged, logger.Fields{
			field.RequestType: requestType,
			field.Request:     request.ToString(),
			field.Response:    responseBody.ToString(),
		})
		utils.WriteJSONResponse(
			w,
			httpStatusCode,
			responseBody,
			requestType,
		)
	}
}

// StopNetworkPacketLoss stops a network packet loss fault in the associated ENI if there is one existing same fault.
func (h *FaultHandler) StopNetworkPacketLoss() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var request types.NetworkPacketLossRequest
		requestType := fmt.Sprintf(startFaultRequestType, types.PacketLossFaultType)

		// Parse the fault request
		err := decodeRequest(w, &request, requestType, r)
		if err != nil {
			return
		}
		// Validate the fault request
		err = validateRequest(w, request, requestType)
		if err != nil {
			return
		}

		// Obtain the task metadata via the endpoint container ID
		taskMetadata, err := validateTaskMetadata(w, h.AgentState, requestType, r)
		if err != nil {
			return
		}

		// To avoid multiple requests to manipulate same network resource
		networkNSPath := taskMetadata.TaskNetworkConfig.NetworkNamespaces[0].Path
		rwMu := h.loadLock(networkNSPath)
		rwMu.Lock()
		defer rwMu.Unlock()

		// TODO: Check status of current fault injection
		// TODO: Invoke the stop fault injection functionality if running

		responseBody := types.NewNetworkFaultInjectionSuccessResponse("stopped")
		logger.Info("Successfully stopped fault", logger.Fields{
			field.RequestType: requestType,
			field.Request:     request.ToString(),
			field.Response:    responseBody.ToString(),
		})
		utils.WriteJSONResponse(
			w,
			http.StatusOK,
			responseBody,
			requestType,
		)
	}
}

// CheckNetworkPacketLoss checks the status of given network packet loss fault.
func (h *FaultHandler) CheckNetworkPacketLoss() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var request types.NetworkPacketLossRequest
		requestType := fmt.Sprintf(startFaultRequestType, types.PacketLossFaultType)

		// Parse the fault request
		err := decodeRequest(w, &request, requestType, r)
		if err != nil {
			return
		}
		// Validate the fault request
		err = validateRequest(w, request, requestType)
		if err != nil {
			return
		}

		// Obtain the task metadata via the endpoint container ID
		// TODO: Will be using the returned task metadata in a future PR
		taskMetadata, err := validateTaskMetadata(w, h.AgentState, requestType, r)
		if err != nil {
			return
		}

		// To avoid multiple requests to manipulate same network resource
		networkNSPath := taskMetadata.TaskNetworkConfig.NetworkNamespaces[0].Path
		rwMu := h.loadLock(networkNSPath)
		rwMu.RLock()
		defer rwMu.RUnlock()

		// TODO: Check status of current fault injection
		// TODO: Return the correct status state
		responseBody := types.NewNetworkFaultInjectionSuccessResponse("running")
		logger.Info("Successfully checked status for fault", logger.Fields{
			field.RequestType: requestType,
			field.Request:     request.ToString(),
			field.Response:    responseBody.ToString(),
		})
		utils.WriteJSONResponse(
			w,
			http.StatusOK,
			responseBody,
			requestType,
		)
	}
}

// decodeRequest will log the request and then translate/unmarshal an incoming fault injection request into
// one of the network fault structs which requires the reqeust body to non-empty.
func decodeRequest(w http.ResponseWriter, request types.NetworkFaultRequest, requestType string, r *http.Request) error {
	logRequest(requestType, r)

	jsonDecoder := json.NewDecoder(r.Body)
	if err := jsonDecoder.Decode(request); err != nil {
		// The request has empty body and then respond an explicit message.
		if err == io.EOF {
			err = errors.New(types.MissingRequestBodyError)
		}

		responseBody := types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf("%v", err))
		logger.Error("Error: failed to decode request", logger.Fields{
			field.Error:       err,
			field.RequestType: requestType,
			field.Request:     request.ToString(),
			field.Response:    responseBody.ToString(),
		})

		utils.WriteJSONResponse(
			w,
			http.StatusBadRequest,
			responseBody,
			requestType,
		)
		return err
	}
	return nil
}

// validateRequest will validate that the incoming fault injection request will have the required fields
// in the request body.
func validateRequest(w http.ResponseWriter, request types.NetworkFaultRequest, requestType string) error {
	if err := request.ValidateRequest(); err != nil {
		responseBody := types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf("%v", err))
		logger.Error("Error: missing required payload fields", logger.Fields{
			field.Error:       err,
			field.RequestType: requestType,
			field.Request:     request.ToString(),
			field.Response:    responseBody.ToString(),
		})
		utils.WriteJSONResponse(
			w,
			http.StatusBadRequest,
			responseBody,
			requestType,
		)
		return err
	}
	return nil
}

// validateTaskMetadata will first fetch the associated task metadata and then validate it to make sure
// the task has enabled fault injection and the corresponding network mode is supported.
func validateTaskMetadata(w http.ResponseWriter, agentState state.AgentState, requestType string, r *http.Request) (*state.TaskResponse, error) {
	var taskMetadata state.TaskResponse
	endpointContainerID := mux.Vars(r)[v4.EndpointContainerIDMuxName]
	taskMetadata, err := agentState.GetTaskMetadata(endpointContainerID)
	if err != nil {
		code, errResponse := getTaskMetadataErrorResponse(endpointContainerID, requestType, err)
		responseBody := types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf("%v", errResponse))
		logger.Error("Error: Unable to obtain task metadata", logger.Fields{
			field.Error:       errResponse,
			field.RequestType: requestType,
			field.Response:    responseBody.ToString(),
		})
		utils.WriteJSONResponse(
			w,
			code,
			responseBody,
			requestType,
		)
		return nil, errResponse
	}

	// Check if task is FIS-enabled
	if !taskMetadata.FaultInjectionEnabled {
		errResponse := fmt.Sprintf(faultInjectionEnabledError, taskMetadata.TaskARN)
		responseBody := types.NewNetworkFaultInjectionErrorResponse(errResponse)
		logger.Error("Error: Task is not fault injection enabled.", logger.Fields{
			field.RequestType:             requestType,
			field.TMDSEndpointContainerID: endpointContainerID,
			field.Response:                responseBody.ToString(),
			field.TaskARN:                 taskMetadata.TaskARN,
			field.Error:                   errResponse,
		})
		utils.WriteJSONResponse(
			w,
			http.StatusBadRequest,
			responseBody,
			requestType,
		)
		return nil, errors.New(errResponse)
	}

	if err := validateTaskNetworkConfig(taskMetadata.TaskNetworkConfig); err != nil {
		code, errResponse := getTaskMetadataErrorResponse(endpointContainerID, requestType, err)
		responseBody := types.NewNetworkFaultInjectionErrorResponse(fmt.Sprintf("%v", errResponse))
		logger.Error("Error: Unable to resolve task network config within task metadata", logger.Fields{
			field.Error:                   err,
			field.RequestType:             requestType,
			field.Response:                responseBody.ToString(),
			field.TMDSEndpointContainerID: endpointContainerID,
		})
		utils.WriteJSONResponse(
			w,
			code,
			responseBody,
			requestType,
		)
		return nil, errResponse
	}

	// Check if task is using a valid network mode
	networkMode := taskMetadata.TaskNetworkConfig.NetworkMode
	if networkMode != ecs.NetworkModeHost && networkMode != ecs.NetworkModeAwsvpc {
		errResponse := fmt.Sprintf(invalidNetworkModeError, networkMode)
		responseBody := types.NewNetworkFaultInjectionErrorResponse(errResponse)
		logger.Error("Error: Invalid network mode for fault injection", logger.Fields{
			field.RequestType: requestType,
			field.NetworkMode: networkMode,
			field.Response:    responseBody.ToString(),
		})
		utils.WriteJSONResponse(
			w,
			http.StatusBadRequest,
			responseBody,
			requestType,
		)
		return nil, errors.New(errResponse)
	}

	return &taskMetadata, nil
}

// getTaskMetadataErrorResponse will be used to classify certain errors that was returned from a GetTaskMetadata function call.
func getTaskMetadataErrorResponse(endpointContainerID, requestType string, err error) (int, error) {
	var errContainerLookupFailed *state.ErrorLookupFailure
	if errors.As(err, &errContainerLookupFailed) {
		return http.StatusNotFound, fmt.Errorf("unable to lookup container: %s", endpointContainerID)
	}

	var errFailedToGetContainerMetadata *state.ErrorMetadataFetchFailure
	if errors.As(err, &errFailedToGetContainerMetadata) {
		return http.StatusInternalServerError, fmt.Errorf("unable to obtain container metadata for container: %s", endpointContainerID)
	}

	logger.Error("Unknown error encountered when handling task metadata fetch failure", logger.Fields{
		field.Error:       err,
		field.RequestType: requestType,
	})
	return http.StatusInternalServerError, fmt.Errorf("failed to get task metadata due to internal server error for container: %s", endpointContainerID)
}

// logRequest is used to log incoming fault injection requests.
func logRequest(requestType string, r *http.Request) {
	endpointContainerID := mux.Vars(r)[v4.EndpointContainerIDMuxName]
	var body []byte
	var err error
	if r.Body != nil {
		body, err = io.ReadAll(r.Body)
		if err != nil {
			logger.Error("Error: Unable to read request body", logger.Fields{
				field.RequestType:             requestType,
				field.Error:                   err,
				field.TMDSEndpointContainerID: endpointContainerID,
			})
			return
		}
	}

	logger.Info(fmt.Sprintf("Received new request for request type: %s", requestType), logger.Fields{
		field.Request:                 string(body),
		field.RequestType:             requestType,
		field.TMDSEndpointContainerID: endpointContainerID,
	})
	r.Body = io.NopCloser(bytes.NewBuffer(body))
}

// validateTaskNetworkConfig validates the passed in task network config for any null/empty values.
func validateTaskNetworkConfig(taskNetworkConfig *state.TaskNetworkConfig) error {
	if taskNetworkConfig == nil {
		return errors.New("TaskNetworkConfig is empty within task metadata")
	}

	if len(taskNetworkConfig.NetworkNamespaces) == 0 || taskNetworkConfig.NetworkNamespaces[0] == nil {
		return errors.New("empty network namespaces within task network config")
	}

	// Task network namespace path is required to inject faults in the associated task.
	if taskNetworkConfig.NetworkNamespaces[0].Path == "" {
		return errors.New("no path in the network namespace within task network config")
	}

	if len(taskNetworkConfig.NetworkNamespaces[0].NetworkInterfaces) == 0 || taskNetworkConfig.NetworkNamespaces[0].NetworkInterfaces == nil {
		return errors.New("empty network interfaces within task network config")
	}

	// Device name is required to inject network faults to given ENI in the task.
	if taskNetworkConfig.NetworkNamespaces[0].NetworkInterfaces[0].DeviceName == "" {
		return errors.New("no ENI device name in the network namespace within task network config")
	}

	return nil
}

// startNetworkPacketLossFault invokes the linux TC utility tool to start the network-packet-loss fault.
func (h *FaultHandler) startNetworkPacketLossFault(ctx context.Context, taskMetadata *state.TaskResponse, request types.NetworkPacketLossRequest) error {
	interfaceName := taskMetadata.TaskNetworkConfig.NetworkNamespaces[0].NetworkInterfaces[0].DeviceName
	networkMode := taskMetadata.TaskNetworkConfig.NetworkMode
	// If task's network mode is awsvpc, we need to run nsenter to access the task's network namespace.
	nsenterPrefix := ""
	if networkMode == ecs.NetworkModeAwsvpc {
		nsenterPrefix = fmt.Sprintf(nsenterCommandString, taskMetadata.TaskNetworkConfig.NetworkNamespaces[0].Path)
	}
	lossPercent := aws.Uint64Value(request.LossPercent)

	// Command to be executed:
	// <nsenterPrefix> tc qdisc add dev <interfaceName> root handle 1: prio priomap 2 2 2 2 2 2 2 2 2 2 2 2 2 2 2 2
	// <nsenterPrefix> "tc qdisc add dev <interfaceName> parent 1:1 handle 10: netem loss <lossPercentage>%"
	tcAddQdiscRootCommandComposed := nsenterPrefix + fmt.Sprintf(tcAddQdiscRootCommandString, interfaceName)
	cmdList := strings.Split(tcAddQdiscRootCommandComposed, " ")
	cmdOutput, err := h.runExecCommand(ctx, cmdList)
	if err != nil {
		logger.Error(fmt.Sprintf("'%s' command failed with the following error: '%s'. std output: '%s'",
			tcAddQdiscRootCommandComposed, err, string(cmdOutput[:])))
		return err
	}
	logger.Info(fmt.Sprintf("'%s' command result: '%s'", tcAddQdiscRootCommandComposed, string(cmdOutput[:])))
	tcAddQdiscLossCommandComposed := nsenterPrefix + fmt.Sprintf(tcAddQdiscLossCommandString, interfaceName, lossPercent)
	cmdList = strings.Split(tcAddQdiscLossCommandComposed, " ")
	cmdOutput, err = h.runExecCommand(ctx, cmdList)
	if err != nil {
		logger.Error(fmt.Sprintf("'%s' command failed with the following error: '%s'. std output: '%s'",
			tcAddQdiscLossCommandComposed, err, string(cmdOutput[:])))
		return err
	}
	logger.Info(fmt.Sprintf("'%s' command result: '%s'", tcAddQdiscLossCommandComposed, string(cmdOutput[:])))
	// After creating the queueing discipline, create filters to associate the IPs in the request with the handle.
	for _, ip := range request.Sources {
		tcAddFilterForIPCommandComposed := nsenterPrefix + fmt.Sprintf(tcAddFilterForIPCommandString, interfaceName, *ip)
		cmdList = strings.Split(tcAddFilterForIPCommandComposed, " ")
		_, err = h.runExecCommand(ctx, cmdList)
		if err != nil {
			logger.Error(fmt.Sprintf("'%s' command failed with the following error: '%s'. std output: '%s'",
				tcAddFilterForIPCommandComposed, err, string(cmdOutput[:])))
			return err
		}
	}

	return nil
}

// checkTCFault check if there's existing network-latency fault or network-packet-loss fault.
func (h *FaultHandler) checkTCFault(ctx context.Context, taskMetadata *state.TaskResponse) (bool, bool, error) {
	interfaceName := taskMetadata.TaskNetworkConfig.NetworkNamespaces[0].NetworkInterfaces[0].DeviceName
	networkMode := taskMetadata.TaskNetworkConfig.NetworkMode
	// If task's network mode is awsvpc, we need to run nsenter to access the task's network namespace.
	nsenterPrefix := ""
	if networkMode == ecs.NetworkModeAwsvpc {
		nsenterPrefix = fmt.Sprintf(nsenterCommandString, taskMetadata.TaskNetworkConfig.NetworkNamespaces[0].Path)
	}

	// We will run the following Linux command to assess if there existing fault.
	// "tc -j q show dev {INTERFACE} parent 1:1"
	// The command above gives the output of "tc q show dev {INTERFACE} parent 1:1" in json format.
	// We will then unmarshall the json string and evaluate the fields of it.
	tcCheckInjectionCommandComposed := nsenterPrefix + fmt.Sprintf(tcCheckInjectionCommandString, interfaceName)
	cmdList := strings.Split(tcCheckInjectionCommandComposed, " ")
	cmdOutput, err := h.runExecCommand(ctx, cmdList)
	if err != nil {
		logger.Error(fmt.Sprintf("'%s' command failed with the following error: '%s'. std output: '%s'",
			tcCheckInjectionCommandComposed, err, string(cmdOutput[:])))
		return false, false, fmt.Errorf("failed to check existing network fault: '%s' command failed with the following error: '%s'. std output: '%s'",
			tcCheckInjectionCommandComposed, err, string(cmdOutput[:]))
	}
	// Log the command output to better help us debug.
	logger.Info(fmt.Sprintf("'%s' command result: '%s'", tcCheckInjectionCommandComposed, string(cmdOutput[:])))

	// Check whether latency fault exists and whether packet loss fault exists separately.
	var outputUnmarshalled []map[string]interface{}
	err = json.Unmarshal(cmdOutput, &outputUnmarshalled)
	if err != nil {
		return false, false, errors.New("failed to check existing network fault: failed to unmarshal tc command output: " + err.Error())
	}
	latencyFaultExists, err := checkLatencyFault(outputUnmarshalled)
	if err != nil {
		return false, false, errors.New("failed to check existing network fault: " + err.Error())
	}
	packetLossFaultExists, err := checkPacketLossFault(outputUnmarshalled)
	if err != nil {
		return false, false, errors.New("failed to check existing network fault: " + err.Error())
	}
	return latencyFaultExists, packetLossFaultExists, nil
}

// checkLatencyFault parses the tc command output and checks if there's existing network-latency fault running.
func checkLatencyFault(outputUnmarshalled []map[string]interface{}) (bool, error) {
	for _, line := range outputUnmarshalled {
		// Check if field "kind":"netem" exists.
		if line["kind"] == "netem" {
			// Now check if network packet loss fault exists.
			if options := line["options"]; options != nil {
				if delay := options.(map[string]interface{})["delay"]; delay != nil {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// checkPacketLossFault parses the tc command output and checks if there's existing network-packet-loss fault running.
func checkPacketLossFault(outputUnmarshalled []map[string]interface{}) (bool, error) {
	for _, line := range outputUnmarshalled {
		// First check field "kind":"netem" exists.
		if line["kind"] == "netem" {
			// Now check if field "loss":"<loss percent>" exists, and if the percentage matches with the value in the request.
			if options := line["options"]; options != nil {
				if lossRandom := options.(map[string]interface{})["loss-random"]; lossRandom != nil {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// runExecCommand wraps around the execwrapper, providing a convenient way of running any Linux command
// and getting the result in both stdout and stderr.
func (h *FaultHandler) runExecCommand(ctx context.Context, cmdList []string) ([]byte, error) {
	cmdExec := h.osExecWrapper.CommandContext(ctx, cmdList[0], cmdList[1:]...)
	return cmdExec.CombinedOutput()
}
