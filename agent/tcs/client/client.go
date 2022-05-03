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

package tcsclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/doctor"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/private/protocol/json/jsonutil"
	"github.com/cihub/seelog"
	"github.com/pborman/uuid"
)

const (
	// tasksInMetricMessage is the maximum number of tasks that can be sent in a message to the backend
	// This is a very conservative estimate assuming max allowed string lengths for all fields.
	tasksInMetricMessage = 10
	// tasksInHealthMessage is the maximum number of tasks that can be sent in a message to the backend
	tasksInHealthMessage = 10
	// defaultPublishServiceConnectTicker is every 3rd time service connect metrics will be sent to the backend
	// Task metrics are published at 20s interval, thus task's service metrics will be published 60s.
	defaultPublishServiceConnectTicker = 3
)

var (
	// publishMetricRequestSizeLimit is the maximum number of bytes that can be sent in a message to the backend
	publishMetricRequestSizeLimit = 1024 * 1024
)

// clientServer implements wsclient.ClientServer interface for metrics backend.
type clientServer struct {
	statsEngine                         stats.Engine
	doctor                              *doctor.Doctor
	publishTicker                       *time.Ticker
	publishHealthTicker                 *time.Ticker
	pullInstanceStatusTicker            *time.Ticker
	publishServiceConnectTickerInterval int32
	ctx                                 context.Context
	cancel                              context.CancelFunc
	disableResourceMetrics              bool
	publishMetricsInterval              time.Duration
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
	disableResourceMetrics bool,
	doctor *doctor.Doctor,
) wsclient.ClientServer {
	cs := &clientServer{
		statsEngine:              statsEngine,
		doctor:                   doctor,
		publishTicker:            nil,
		publishHealthTicker:      nil,
		pullInstanceStatusTicker: nil,
		publishMetricsInterval:   publishMetricsInterval,
	}
	cs.URL = url
	cs.AgentConfig = cfg
	cs.CredentialProvider = credentialProvider
	cs.ServiceError = &tcsError{}
	cs.RequestHandlers = make(map[string]wsclient.RequestHandler)
	cs.MakeRequestHook = signRequestFunc(url, cs.AgentConfig.AWSRegion, credentialProvider)
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
	cs.pullInstanceStatusTicker = time.NewTicker(cs.publishMetricsInterval)

	if !cs.disableResourceMetrics {
		go cs.publishMetrics()
	}
	go cs.publishHealthMetrics()

	go cs.publishInstanceStatus()

	return cs.ConsumeMessages()
}

// Close closes the underlying connection.
func (cs *clientServer) Close() error {
	if cs.publishTicker != nil {
		cs.publishTicker.Stop()
	}
	if cs.publishHealthTicker != nil {
		cs.publishHealthTicker.Stop()
	}
	if cs.pullInstanceStatusTicker != nil {
		cs.pullInstanceStatusTicker.Stop()
	}

	cs.cancel()
	return cs.Disconnect()
}

// signRequestFunc is a MakeRequestHookFunc that signs each generated request
func signRequestFunc(url, region string, credentialProvider *credentials.Credentials) wsclient.MakeRequestHookFunc {
	return func(payload []byte) ([]byte, error) {
		reqBody := bytes.NewReader(payload)

		request, err := http.NewRequest("GET", url, reqBody)
		if err != nil {
			return nil, err
		}

		err = utils.SignHTTPRequest(request, region, "ecs", credentialProvider, reqBody)
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
}

// publishMetrics invokes the PublishMetricsRequest on the clientserver object.
func (cs *clientServer) publishMetrics() {
	if cs.publishTicker == nil {
		seelog.Debug("Skipping publishing metrics. Publish ticker is uninitialized")
		return
	}

	// don't simply range over the ticker since its channel doesn't ever get closed
	for {
		select {
		case <-cs.publishTicker.C:
			var includeServiceConnectStats bool
			cs.publishServiceConnectTickerInterval++
			if cs.publishServiceConnectTickerInterval == defaultPublishServiceConnectTicker {
				includeServiceConnectStats = true
				cs.publishServiceConnectTickerInterval = 0
			}
			err := cs.publishMetricsOnce(includeServiceConnectStats)
			if err != nil {
				seelog.Warnf("Error publishing metrics: %v", err)
			}
		case <-cs.ctx.Done():
			return
		}
	}
}

// publishMetricsOnce is invoked by the ticker to periodically publish metrics to backend.
func (cs *clientServer) publishMetricsOnce(includeServiceConnectStats bool) error {
	// Get the list of objects to send to backend.
	requests, err := cs.metricsToPublishMetricRequests(includeServiceConnectStats)
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
func (cs *clientServer) metricsToPublishMetricRequests(includeServiceConnectStats bool) ([]*ecstcs.PublishMetricsRequest, error) {
	metadata, taskMetrics, err := cs.statsEngine.GetInstanceMetrics(includeServiceConnectStats)
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
	var requestMetadata *ecstcs.MetricsMetadata
	numTasks := len(taskMetrics)

	for i, taskMetric := range taskMetrics {
		requestMetadata = copyMetricsMetadata(metadata, false)

		// TODO [SC]: Check if SC is enabled for the task
		if includeServiceConnectStats {
			taskMetric, messageTaskMetrics, requests = cs.serviceConnectMetricsToPublishMetricRequests(requestMetadata, taskMetric, messageTaskMetrics, requests)
		}
		messageTaskMetrics = append(messageTaskMetrics, taskMetric)
		if (i + 1) == numTasks {
			// If this is the last task to send, set fin to true
			requestMetadata = copyMetricsMetadata(metadata, true)
		}
		if len(messageTaskMetrics)%tasksInMetricMessage == 0 {
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

// serviceConnectMetricsToPublishMetricRequests loops over all the SC metrics in a
// task metric to add SC metrics until the message size is within 1 MB.
// If adding a SC metric to the message exceeds the 1 MB limit, it will be sent in the new message
func (cs *clientServer) serviceConnectMetricsToPublishMetricRequests(requestMetadata *ecstcs.MetricsMetadata, taskMetric *ecstcs.TaskMetric,
	messageTaskMetrics []*ecstcs.TaskMetric, requests []*ecstcs.PublishMetricsRequest) (*ecstcs.TaskMetric, []*ecstcs.TaskMetric, []*ecstcs.PublishMetricsRequest) {
	tempTaskMetric := *taskMetric
	tempTaskMetric.ServiceConnectMetricsWrapper = tempTaskMetric.ServiceConnectMetricsWrapper[:0]

	for _, serviceConnectMetric := range taskMetric.ServiceConnectMetricsWrapper {
		tempTaskMetric.ServiceConnectMetricsWrapper = append(tempTaskMetric.ServiceConnectMetricsWrapper, serviceConnectMetric)
		messageTaskMetrics = append(messageTaskMetrics, &tempTaskMetric)
		// TODO [SC]: Load test and profile this since BuildJSON results in lot of CPU and memory consumption.
		tempMessage, _ := jsonutil.BuildJSON(ecstcs.NewPublishMetricsRequest(requestMetadata, copyTaskMetrics(messageTaskMetrics)))
		// remove the tempTaskMetric added to messageTaskMetrics after creating tempMessage
		messageTaskMetrics = messageTaskMetrics[:len(messageTaskMetrics)-1]
		if len(tempMessage) > publishMetricRequestSizeLimit {
			// since adding this SC metric to the message exceeds the 1 MB limit, remove it from taskMetric and create a request to send it to the backend
			tempTaskMetric.ServiceConnectMetricsWrapper = tempTaskMetric.ServiceConnectMetricsWrapper[:len(tempTaskMetric.ServiceConnectMetricsWrapper)-1]
			taskMetricTruncated := tempTaskMetric
			taskMetricTruncated.ServiceConnectMetricsWrapper = copyServiceConnectMetrics(tempTaskMetric.ServiceConnectMetricsWrapper)

			messageTaskMetrics = append(messageTaskMetrics, &taskMetricTruncated)
			requests = append(requests, ecstcs.NewPublishMetricsRequest(requestMetadata, copyTaskMetrics(messageTaskMetrics)))

			// reset the messageTaskMetrics and tempTaskMetric for the new request,
			messageTaskMetrics = messageTaskMetrics[:0]
			tempTaskMetric.ServiceConnectMetricsWrapper = tempTaskMetric.ServiceConnectMetricsWrapper[:0]
			// container metrics will be sent only once for each task metric
			tempTaskMetric.ContainerMetrics = tempTaskMetric.ContainerMetrics[:0]
			// add the serviceConnectMetric to tempTaskMetric to be sent in the next message
			tempTaskMetric.ServiceConnectMetricsWrapper = append(tempTaskMetric.ServiceConnectMetricsWrapper, serviceConnectMetric)
		}
	}
	return &tempTaskMetric, messageTaskMetrics, requests
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

// copyServiceConnectMetrics loops over list of GeneralMetricsWrapper obejcts and creates a new GeneralMetricsWrapper list
// and creates a new GeneralMetricsWrapper object from each given GeneralMetricsWrapper object.
func copyServiceConnectMetrics(scMetrics []*ecstcs.GeneralMetricsWrapper) []*ecstcs.GeneralMetricsWrapper {
	scMetricsTo := make([]*ecstcs.GeneralMetricsWrapper, len(scMetrics))
	for i, scMetricFrom := range scMetrics {
		scMetricTo := *scMetricFrom
		scMetricsTo[i] = &scMetricTo
	}
	return scMetricsTo
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

// publishInstanceStatus queries the doctor.Doctor instance contained within cs,
// converts the healthcheck results to an InstanceStatusRequest and then sends it
// to the backend
func (cs *clientServer) publishInstanceStatus() {
	// Note to disambiguate between health metrics and instance statuses
	//
	// Instance status checks are performed by the Doctor class in the doctor module
	// but the code calls them Healthchecks. They are named such because they denote
	// that the container runtime (Docker) is healthy and communicating with the
	// ECS Agent. Container instance statuses, which this function handles,
	// pertain to the status of this container instance.
	//
	// Health metrics are specific to the tasks that are running on this particular
	// container instance. Health metrics, which the publishHealthMetrics function
	// handles, pertain to the health of the tasks that are running on this
	// container instance.
	if cs.pullInstanceStatusTicker == nil {
		seelog.Debug("Skipping publishing container instance statuses. Publish ticker is uninitialized")
		return
	}

	for {
		select {
		case <-cs.pullInstanceStatusTicker.C:
			if !cs.doctor.HasStatusBeenReported() {
				err := cs.publishInstanceStatusOnce()
				if err != nil {
					seelog.Warnf("Unable to publish instance status: %v", err)
				} else {
					cs.doctor.SetStatusReported(true)
				}
			} else {
				seelog.Debug("Skipping publishing container instance status message that was already sent")
			}
		case <-cs.ctx.Done():
			return
		}
	}
}

// publishInstanceStatusOnce gets called on a ticker to pull instance status
// from the doctor instance contained within cs and sned that information to
// the backend
func (cs *clientServer) publishInstanceStatusOnce() error {
	// Get the list of health request to send to backend.
	request, err := cs.getPublishInstanceStatusRequest()
	if err != nil {
		return err
	}

	// Make the publish instance status request to the backend.
	err = cs.MakeRequest(request)
	if err != nil {
		return err
	}

	cs.doctor.SetStatusReported(true)

	return nil
}

// GetPublishInstanceStatusRequest will get all healthcheck statuses and generate
// a sendable PublishInstanceStatusRequest
func (cs *clientServer) getPublishInstanceStatusRequest() (*ecstcs.PublishInstanceStatusRequest, error) {
	metadata := &ecstcs.InstanceStatusMetadata{
		Cluster:           aws.String(cs.doctor.GetCluster()),
		ContainerInstance: aws.String(cs.doctor.GetContainerInstanceArn()),
		RequestId:         aws.String(uuid.NewRandom().String()),
	}
	instanceStatuses := cs.getInstanceStatuses()
	if instanceStatuses == nil {
		return nil, doctor.EmptyHealthcheckError
	}

	return &ecstcs.PublishInstanceStatusRequest{
		Metadata:  metadata,
		Statuses:  instanceStatuses,
		Timestamp: aws.Time(time.Now()),
	}, nil
}

// getInstanceStatuses returns a list of instance statuses converted from what
// the doctor knows about the registered healthchecks
func (cs *clientServer) getInstanceStatuses() []*ecstcs.InstanceStatus {
	var instanceStatuses []*ecstcs.InstanceStatus

	for _, healthcheck := range *cs.doctor.GetHealthchecks() {
		instanceStatus := &ecstcs.InstanceStatus{
			LastStatusChange: aws.Time(healthcheck.GetStatusChangeTime()),
			LastUpdated:      aws.Time(healthcheck.GetLastHealthcheckTime()),
			Status:           aws.String(healthcheck.GetHealthcheckStatus().String()),
			Type:             aws.String(healthcheck.GetHealthcheckType()),
		}
		instanceStatuses = append(instanceStatuses, instanceStatus)
	}
	return instanceStatuses
}
