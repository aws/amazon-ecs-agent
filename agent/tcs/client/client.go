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

package tcsclient

import (
	"bytes"
	"errors"
	"net/http"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	"github.com/gorilla/websocket"
)

var log = logger.ForModule("tcs client")

// clientServer implements wsclient.ClientServer interface for metrics backend.
type clientServer struct {
	statsEngine  stats.Engine
	publishTimer *timer
	wsclient.ClientServerImpl
	signer authv4.HttpSigner
}

// New returns a client/server to bidirectionally communicate with the backend.
// The returned struct should have both 'Connect' and 'Serve' called upon it
// before being used.
func New(url string, region string, credentialProvider credentials.AWSCredentialProvider, acceptInvalidCert bool, statsEngine stats.Engine, publishMetricsInterval time.Duration) wsclient.ClientServer {
	cs := &clientServer{
		statsEngine:  statsEngine,
		publishTimer: newTimer(publishMetricsInterval, publishMetrics),
		signer:       authv4.NewHttpSigner(region, wsclient.ServiceName, credentialProvider, nil),
	}
	cs.URL = url
	cs.Region = region
	cs.CredentialProvider = credentialProvider
	cs.AcceptInvalidCert = acceptInvalidCert
	cs.ServiceError = &tcsError{}
	cs.RequestHandlers = make(map[string]wsclient.RequestHandler)
	cs.TypeDecoder = &TcsDecoder{}
	return cs
}

// Serve begins serving requests using previously registered handlers (see
// AddRequestHandler). All request handlers should be added prior to making this
// call as unhandled requests will be discarded.
func (cs *clientServer) Serve() error {
	log.Debug("Starting websocket poll loop")
	if cs.Conn == nil {
		return errors.New("nil connection")
	}

	// Start the timer function to publish metrics to the backend.
	if cs.statsEngine != nil {
		cs.publishTimer.start(cs)
	}

	return cs.ConsumeMessages()
}

// MakeRequest makes a request using the given input. Note, the input *MUST* be
// a pointer to a valid backend type that this client recognises
func (cs *clientServer) MakeRequest(input interface{}) error {
	payload, err := cs.CreateRequestMessage(input)
	if err != nil {
		return err
	}

	data := cs.signRequest(payload)

	// Over the wire we send something like
	// {"type":"AckRequest","message":{"messageId":"xyz"}}
	// return cs.Conn.WriteMessage(websocket.TextMessage, send)
	return cs.Conn.WriteMessage(websocket.TextMessage, data)
}

func (cs *clientServer) signRequest(payload []byte) []byte {
	signer := authv4.NewHttpSigner(cs.Region, "ecs", cs.CredentialProvider, nil)
	reqBody := bytes.NewBuffer(payload)
	// NewRequest never returns an error if the url parses and we just verified
	// it did above
	request, _ := http.NewRequest("GET", cs.URL, reqBody)
	signer.SignHttpRequest(request)

	var data []byte
	for k, vs := range request.Header {
		for _, v := range vs {
			data = append(data, k...)
			data = append(data, ": "...)
			data = append(data, v...)
			data = append(data, "\r\n"...)
		}
	}
	data = append(data, "\r\n"...)
	data = append(data, payload...)

	return data
}

// Close closes the underlying connection.
func (cs *clientServer) Close() error {
	cs.publishTimer.stop()
	if cs.Conn != nil {
		return cs.Conn.Close()
	}
	return nil
}

// publishMetrics invokes the PublishMetricsRequest on the clientserver object.
// The argument must be of the clientServer type.
func publishMetrics(cs interface{}) error {
	clsrv, ok := cs.(*clientServer)
	if !ok {
		return errors.New("Unexpected object type in publishMetric")
	}
	metadata, taskMetrics, err := clsrv.statsEngine.GetInstanceMetrics()
	if err != nil {
		return err
	}

	return clsrv.MakeRequest(ecstcs.NewPublishMetricsRequest(metadata, taskMetrics))
}
