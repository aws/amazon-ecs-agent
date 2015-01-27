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

package acs

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"
	"github.com/aws/amazon-ecs-agent/agent/utils"

	"github.com/gorilla/websocket"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/logger"
)

var log = logger.ForModule("acs")

const CONNECT_TIMEOUT = 3 * time.Second

// The maximum time to wait between heartbeats without disconnecting
const HEARTBEAT_TIMEOUT = 5 * time.Minute
const HEARTBEAT_JITTER = 3 * time.Minute

// AgentCommunicationClient is a client that keeps track of connections to the
// AgentCommunication backend service. It stores configuration needed to connect
// and its current connection
type AgentCommunicationClient struct {
	cfg                  *config.Config
	containerInstanceArn string
	credentialProvider   credentials.AWSCredentialProvider
	endpoint             string
	port                 int

	connection     *websocket.Conn
	heartbeatTimer *time.Timer
}

func NewAgentCommunicationClient(endpoint string, cfg *config.Config, credentialProvider credentials.AWSCredentialProvider, containerInstanceArn string) *AgentCommunicationClient {
	return &AgentCommunicationClient{
		cfg:                  cfg,
		credentialProvider:   credentialProvider,
		containerInstanceArn: containerInstanceArn,
		endpoint:             endpoint,
		port:                 443,
	}
}

func (acc *AgentCommunicationClient) createAcsUrl() string {
	acsUrl := acc.endpoint + "/ws"
	query := url.Values{}
	query.Set("clusterArn", acc.cfg.Cluster)
	query.Set("containerInstanceArn", acc.containerInstanceArn)

	return acsUrl + "?" + query.Encode()
}

// Poll contacts the agent communication service and opens a websocket to
// wait for updates. It emits each state update to the Payload channel it
// returns
func (acc *AgentCommunicationClient) Poll(acceptInvalidCert bool) (<-chan *Payload, <-chan error, error) {
	cfg := acc.cfg
	credentialProvider := acc.credentialProvider

	tasksc := make(chan *Payload)
	errc := make(chan error)

	acsUrl := acc.createAcsUrl()
	parsedAcsUrl, err := url.Parse(acsUrl)
	if err != nil {
		return nil, nil, err
	}

	signer := authv4.NewHttpSigner(cfg.AWSRegion, api.ECS_SERVICE, credentialProvider, nil)

	// NewRequest never returns an error if the url parses and we just verified
	// it did above
	request, _ := http.NewRequest("GET", acsUrl, nil)
	signer.SignHttpRequest(request)

	timeoutDialer := &net.Dialer{Timeout: CONNECT_TIMEOUT}
	log.Info("Creating poll dialer", "host", parsedAcsUrl.Host)
	acsConn, err := tls.DialWithDialer(timeoutDialer, "tcp", net.JoinHostPort(parsedAcsUrl.Host, strconv.Itoa(acc.port)), &tls.Config{InsecureSkipVerify: acceptInvalidCert})
	if err != nil {
		return nil, nil, err
	}

	websocketConn, httpResponse, err := websocket.NewClient(acsConn, parsedAcsUrl, request.Header, 1024, 1024)
	if httpResponse != nil {
		defer httpResponse.Body.Close()
	}
	if err != nil {
		var resp []byte
		if httpResponse != nil {
			resp, _ = ioutil.ReadAll(httpResponse.Body)
		}
		log.Warn("Error creating a websocket client", "err", err)
		return nil, nil, errors.New(string(resp) + ", " + err.Error())
	}

	acc.connection = websocketConn
	acc.resetHeartbeat()

	log.Info("Starting websocket poll loop")
	go func() {
		defer close(tasksc)
		for {
			messageType, message, err := websocketConn.ReadMessage()
			if err != nil {
				if message != nil {
					log.Error("Error getting message from acs", "err", err, "message", message)
				} else {
					log.Error("Error getting message from acs", "err", err)
				}
				break
			}
			if messageType != websocket.TextMessage {
				log.Error("Unexpected messageType", "type", messageType)
			}
			log.Debug("Got a message from acs websocket", "message", string(message[:]))
			tasks, skip, err := acc.parseResponseLine(message)
			log.Debug("Unmarshalled message as", "tasks", tasks, "skip", skip, "err", err)
			if err != nil {
				errc <- err
			} else if !skip {
				tasksc <- tasks
			} // skip
		}
	}()

	return tasksc, errc, nil
}

// Ack acknowledges that a given message was recieved. It is expected to be
// called after Poll such that Poll has already opened a websocket connection.
func (acc *AgentCommunicationClient) Ack(message *Payload) bool {
	if acc.connection == nil {
		log.Error("Could not ack message, connection nil")
		return false
	}

	log.Info("Acking a message", "messageId", message.MessageId)
	ackRequest := AckRequest{ClusterArn: acc.cfg.Cluster, ContainerInstanceArn: acc.containerInstanceArn, MessageId: message.MessageId}
	result, err := json.Marshal(ackRequest)
	if err != nil {
		log.Error("Unable to marshal ack; this is odd", "err", err)
		return false
	}

	acc.connection.WriteMessage(websocket.TextMessage, result)
	return true
}

func (acc *AgentCommunicationClient) resetHeartbeat() {
	if acc.heartbeatTimer != nil {
		acc.heartbeatTimer.Stop()
	}
	acc.heartbeatTimer = time.AfterFunc(utils.AddJitter(HEARTBEAT_TIMEOUT, HEARTBEAT_JITTER), func() {
		log.Error("Heartbeat timer expired! Closing connection")
		acc.connection.Close()
	})
}

func (acc *AgentCommunicationClient) parseResponseLine(line []byte) (*Payload, bool, error) {
	var response PollResponse

	err := json.Unmarshal(line, &response)
	if err != nil {
		return nil, true, err
	}

	log.Info("Message = ", "message", response)

	if response.MessageType == "PayloadMessage" {
		// Payload PollResponse, we unmarshaled it right
		return &response.Message, false, err
	}

	if response.MessageType == "HeartbeatMessage" {
		acc.resetHeartbeat()
		return nil, true, err
	}

	err = errors.New("Could not unmarshal ACS response... Skipping")
	return nil, true, err
}
