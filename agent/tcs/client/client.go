// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/cihub/seelog"
)

const (
	// tasksInMetricMessage is the maximum number of tasks that can be sent in a message to the backend
	// This is a very conservative estimate assuming max allowed string lengths for all fields.
	tasksInMetricMessage = 10
	// tasksInHealthMessage is the maximum number of tasks that can be sent in a message to the backend
	tasksInHealthMessage = 10
)

// clientServer implements wsclient.ClientServer interface for metrics backend.
type clientServer struct {
	statsEngine            stats.Engine
	publishTicker          *time.Ticker
	publishHealthTicker    *time.Ticker
	ctx                    context.Context
	cancel                 context.CancelFunc
	disableResourceMetrics bool
	publishMetricsInterval time.Duration
	wsclient.ClientServerImpl
}

// New returns a client/server to bidirectionally communicate with the backend.
// The returned struct should have both 'Connect' and 'Serve' called upon it
// before being used.
func New(url string,
	cfg *config.Config,
	credentialProvider *credentials.Credentials,
	statsEngine stats.Engine,
	publishMetricsInterval time.Duration,
	rwTimeout time.Duration,
	disableResourceMetrics bool) wsclient.ClientServer {
	cs := &clientServer{
		statsEngine:            statsEngine,
		publishTicker:          nil,
		publishHealthTicker:    nil,
		publishMetricsInterval: publishMetricsInterval,
	}
	cs.URL = url
	cs.AgentConfig = cfg
	cs.CredentialProvider = credentialProvider
	cs.ServiceError = &tcsError{}
	cs.RequestHandlers = make(map[string]wsclient.RequestHandler)
	cs.TypeDecoder = NewTCSDecoder()
	cs.RWTimeout = rwTimeout
	cs.disableResourceMetrics = disableResourceMetrics
	// TODO make this context inherited from the handler
	cs.ctx, cs.cancel = context.WithCancel(context.TODO())
	return cs
}

// Serve begins serving requests using previously registered handlers (see
// AddRequestHandler). All request handlers should be added prior to making this
// call as unhandled requests will be discarded.
func (cs *clientServer) Serve() error {
	seelog.Debug("TCS client starting websocket poll loop")
	if !cs.IsReady() {
		return fmt.Errorf("tcs client: websocket not ready for connections")
	}

	if cs.statsEngine == nil {
		return fmt.Errorf("tcs client: uninitialized stats engine")
	}

	// Start the timer function to publish metrics to the backend.
	cs.publishTicker = time.NewTicker(cs.publishMetricsInterval)
	cs.publishHealthTicker = time.NewTicker(cs.publishMetricsInterval)

	if !cs.disableResourceMetrics {
		go cs.publishMetrics()
	}
	go cs.publishHealthMetrics()

	return cs.ConsumeMessages()
}

// MakeRequest makes a request using the given input. Note, the input *MUST* be
// a pointer to a valid backend type that this client recognises
func (cs *clientServer) MakeRequest(input interface{}) error {
	payload, err := cs.CreateRequestMessage(input)
	if err != nil {
		return err
	}

	seelog.Debugf("TCS client sending payload: %s", string(payload))
	data, err := cs.signRequest(payload)
	if err != nil {
		return err
	}

	// Over the wire we send something like
	// {"type":"AckRequest","message":{"messageId":"xyz"}}
	return cs.WriteMessage(data)
}

func (cs *clientServer) signRequest(payload []byte) ([]byte, error) {
	reqBody := bytes.NewReader(payload)
	// NewRequest never returns an error if the url parses and we just verified
	// it did above
	request, _ := http.NewRequest("GET", cs.URL, reqBody)
	err := utils.SignHTTPRequest(request, cs.AgentConfig.AWSRegion, "ecs", cs.CredentialProvider, aws.ReadSeekCloser(reqBody))
	if err != nil {
		return nil, err
	}

	request.Header.Add("Host", request.Host)
	var dataBuffer bytes.Buffer
	request.Header.Write(&dataBuffer)
	io.WriteString(&dataBuffer, "\r\n")

	data := dataBuffer.Bytes()
	data = append(data, payload...)

	return data, nil
}

// Close closes the underlying connection.
func (cs *clientServer) Close() error {
	if cs.publishTicker != nil {
		cs.publishTicker.Stop()
	}
	if cs.publishHealthTicker != nil {
		cs.publishHealthTicker.Stop()
	}

	cs.cancel()
	return cs.Disconnect()
}

// publishMetrics invokes the PublishMetricsRequest on the clientserver object.
func (cs *clientServer) publishMetrics() {
	if cs.publishTicker == nil {
		seelog.Debug("Skipping publishing metrics. Publish ticker is uninitialized")
		return
	}

	// Publish metrics immediately after we connect and wait for ticks. This makes
	// sure that there is no data loss when a scheduled metrics publishing fails
	// due to a connection reset.
	err := cs.publishMetricsOnce()
	if err != nil && err != stats.EmptyMetricsError {
		seelog.Warnf("Error publishing metrics: %v", err)
	}
	// don't simply range over the ticker since its channel doesn't ever get closed
	for {
		select {
		case <-cs.publishTicker.C:
			err := cs.publishMetricsOnce()
			if err != nil {
				seelog.Warnf("Error publishing metrics: %v", err)
			}
		case <-cs.ctx.Done():
			return
		}
	}
}

// publishMetricsOnce is invoked by the ticker to periodically publish metrics to backend.
func (cs *clientServer) publishMetricsOnce() error {
	// Get the list of objects to send to backend.
	requests, err := cs.metricsToPublishMetricRequests()
	if err != nil {
		return err
	}

	// Make the publish metrics request to the backend.
	for _, request := range requests {
		err = cs.MakeRequest(request)
		if err != nil {
			return err
		}
	}
	return nil
}

// metricsToPublishMetricRequests gets task metrics and converts them to a list of PublishMetricRequest
// objects.
func (cs *clientServer) metricsToPublishMetricRequests() ([]*ecstcs.PublishMetricsRequest, error) {
	metadata, taskMetrics, err := cs.statsEngine.GetInstanceMetrics()
	if err != nil {
		return nil, err
	}

	var requests []*ecstcs.PublishMetricsRequest
	if *metadata.Idle {
		metadata.Fin = aws.Bool(true)
		// Idle instance, we have only one request to send to backend.
		requests = append(requests, ecstcs.NewPublishMetricsRequest(metadata, taskMetrics))
		return requests, nil
	}
	var messageTaskMetrics []*ecstcs.TaskMetric
	numTasks := len(taskMetrics)

	for i, taskMetric := range taskMetrics {
		messageTaskMetrics = append(messageTaskMetrics, taskMetric)
		var requestMetadata *ecstcs.MetricsMetadata
		if (i + 1) == numTasks {
			// If this is the last task to send, set fin to true
			requestMetadata = copyMetricsMetadata(metadata, true)
		} else {
			requestMetadata = copyMetricsMetadata(metadata, false)
		}
		if (i+1)%tasksInMetricMessage == 0 {
			// Construct payload with tasksInMetricMessage number of task metrics and send to backend.
			requests = append(requests, ecstcs.NewPublishMetricsRequest(requestMetadata, copyTaskMetrics(messageTaskMetrics)))
			messageTaskMetrics = messageTaskMetrics[:0]
		}
	}

	if len(messageTaskMetrics) > 0 {
		// Create the new metadata object and set fin to true as this is the last message in the payload.
		requestMetadata := copyMetricsMetadata(metadata, true)
		// Create a request with remaining task metrics.
		requests = append(requests, ecstcs.NewPublishMetricsRequest(requestMetadata, messageTaskMetrics))
	}
	return requests, nil
}

// publishHealthMetrics send the container health information to backend
func (cs *clientServer) publishHealthMetrics() {
	if cs.publishTicker == nil {
		seelog.Debug("Skipping publishing health metrics. Publish ticker is uninitialized")
		return
	}

	// Publish metrics immediately after we connect and wait for ticks. This makes
	// sure that there is no data loss when a scheduled metrics publishing fails
	// due to a connection reset.
	err := cs.publishHealthMetricsOnce()
	if err != nil {
		seelog.Warnf("Unable to publish health metrics: %v", err)
	}
	for {
		select {
		case <-cs.publishHealthTicker.C:
			err := cs.publishHealthMetricsOnce()
			if err != nil {
				seelog.Warnf("Unable to publish health metrics: %v", err)
			}
		case <-cs.ctx.Done():
			return
		}
	}
}

// publishHealthMetricsOnce is invoked by the ticker to periodically publish metrics to backend.
func (cs *clientServer) publishHealthMetricsOnce() error {
	// Get the list of health request to send to backend.
	requests, err := cs.createPublishHealthRequests()
	if err != nil {
		return err
	}
	// Make the publish metrics request to the backend.
	for _, request := range requests {
		err = cs.MakeRequest(request)
		if err != nil {
			return err
		}
	}
	return nil
}

// createPublishHealthRequests creates the requests to publish container health
func (cs *clientServer) createPublishHealthRequests() ([]*ecstcs.PublishHealthRequest, error) {
	metadata, taskHealthMetrics, err := cs.statsEngine.GetTaskHealthMetrics()
	if err != nil {
		return nil, err
	}

	if metadata == nil || taskHealthMetrics == nil {
		seelog.Debug("No container health metrics to report")
		return nil, nil
	}

	var requests []*ecstcs.PublishHealthRequest
	var taskHealths []*ecstcs.TaskHealth
	numOfTasks := len(taskHealthMetrics)
	for i, taskHealth := range taskHealthMetrics {
		taskHealths = append(taskHealths, taskHealth)
		// create a request if the number of task reaches the maximum page size
		if (i+1)%tasksInHealthMessage == 0 {
			requestMetadata := copyHealthMetadata(metadata, (i+1) == numOfTasks)
			requestTaskHealth := copyTaskHealthMetrics(taskHealths)
			request := ecstcs.NewPublishHealthMetricsRequest(requestMetadata, requestTaskHealth)
			requests = append(requests, request)
			taskHealths = taskHealths[:0]
		}
	}

	// Put the rest of the metrics in another request
	if len(taskHealths) != 0 {
		requestMetadata := copyHealthMetadata(metadata, true)
		requests = append(requests, ecstcs.NewPublishHealthMetricsRequest(requestMetadata, taskHealths))
	}

	return requests, nil
}

// copyMetricsMetadata creates a new MetricsMetadata object from a given MetricsMetadata object.
// It copies all the fields from the source object to the new object and sets the 'Fin' field
// as specified by the argument.
func copyMetricsMetadata(metadata *ecstcs.MetricsMetadata, fin bool) *ecstcs.MetricsMetadata {
	return &ecstcs.MetricsMetadata{
		Cluster:           aws.String(*metadata.Cluster),
		ContainerInstance: aws.String(*metadata.ContainerInstance),
		Idle:              aws.Bool(*metadata.Idle),
		MessageId:         aws.String(*metadata.MessageId),
		Fin:               aws.Bool(fin),
	}
}

// copyTaskMetrics copies a slice of TaskMetric objects to another slice. This is needed as we
// reset the source slice after creating a new PublishMetricsRequest object.
func copyTaskMetrics(from []*ecstcs.TaskMetric) []*ecstcs.TaskMetric {
	to := make([]*ecstcs.TaskMetric, len(from))
	copy(to, from)
	return to
}

// copyHealthMetadata performs a deep copy of HealthMetadata object
func copyHealthMetadata(metadata *ecstcs.HealthMetadata, fin bool) *ecstcs.HealthMetadata {
	return &ecstcs.HealthMetadata{
		Cluster:           aws.String(aws.StringValue(metadata.Cluster)),
		ContainerInstance: aws.String(aws.StringValue(metadata.ContainerInstance)),
		Fin:               aws.Bool(fin),
		MessageId:         aws.String(aws.StringValue(metadata.MessageId)),
	}
}

// copyTaskHealthMetrics copies a slice of taskHealthMetrics to another slice
func copyTaskHealthMetrics(from []*ecstcs.TaskHealth) []*ecstcs.TaskHealth {
	to := make([]*ecstcs.TaskHealth, len(from))
	copy(to, from)
	return to
}
