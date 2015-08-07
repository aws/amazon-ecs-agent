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

package api

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/httpclient"
	"github.com/aws/amazon-ecs-agent/agent/httpclient/mock"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/golang/mock/gomock"
)

func versionedOperation(op string) string {
	return "AmazonEC2ContainerServiceV20141113." + op
}

func operationErrorResp(status int, body string) *http.Response {
	return &http.Response{
		Status:     strconv.Itoa(status) + " foobar",
		StatusCode: status,
		Proto:      "HTTP/1.1",
		ProtoMinor: 1,
		ProtoMajor: 1,
		Body:       aws.ReadSeekCloser(bytes.NewReader([]byte(body))),
	}
}

func setup(t *testing.T) (*gomock.Controller, ECSClient, *mock_http.MockRoundTripper) {
	ctrl := gomock.NewController(t)
	mockRoundTripper := mock_http.NewMockRoundTripper(ctrl)
	mockHttpClient := httpclient.New(1*time.Second, true)
	mockHttpClient.Transport.(httpclient.OverridableTransport).SetTransport(mockRoundTripper)
	client := NewECSClient(credentials.AnonymousCredentials, &config.Config{AWSRegion: "us-east-1"}, mockHttpClient)
	testTime := ttime.NewTestTime()
	testTime.LudicrousSpeed(true)
	ttime.SetTime(testTime)

	return ctrl, client, mockRoundTripper
}

func TestNoClientExceptionRetries(t *testing.T) {
	ctrl, client, mockRoundTripper := setup(t)
	defer ctrl.Finish()

	mockRoundTripper.EXPECT().RoundTrip(mock_http.NewHTTPOperationMatcher(versionedOperation("DiscoverPollEndpoint"))).Return(operationErrorResp(400, `{"__type":"ClientException","message":"something went wrong"}`), nil)

	_, err := client.DiscoverPollEndpoint("foo")
	if err == nil {
		t.Fatal("Expected an error to have been propogated up")
	}
}

func TestServerExceptionRetries(t *testing.T) {
	ctrl, client, mockRoundTripper := setup(t)
	defer ctrl.Finish()

	timesCalled := 0
	// This resp.Body song and dance is because it *must* be reset between
	// retries for the sdk to behave sanely; it rewinds request bodies, not
	// response bodies. The actual server would, indeed put a new body each time
	// so this is not a bad thing to do
	resp := operationErrorResp(500, `{"__type":"BadStuffHappenedException","message":"something went wrong"}`)
	mockRoundTripper.EXPECT().RoundTrip(mock_http.NewHTTPOperationMatcher(versionedOperation("DiscoverPollEndpoint"))).AnyTimes().Do(func(_ interface{}) {
		timesCalled++
		resp.Body = operationErrorResp(500, `{"__type":"BadStuffHappenedException","message":"something went wrong"}`).Body
	}).Return(resp, nil).AnyTimes()

	start := ttime.Now()
	_, err := client.DiscoverPollEndpoint("foo")
	if err == nil {
		t.Error("Expected it to error after retrying")
	}
	duration := ttime.Since(start)
	if duration < 100*time.Millisecond {
		t.Error("Retries should have taken some time; took " + duration.String())
	}
	if timesCalled < 2 || timesCalled > 10 {
		// Actaully 4 at the time of writing, but a reasonable range is fine
		t.Error("Retries should happen a reasonable number of times")
	}
}

func TestSubmitRetries(t *testing.T) {
	ctrl, client, mockRoundTripper := setup(t)
	defer ctrl.Finish()

	timesCalled := 0
	resp := operationErrorResp(500, `{"__type":"SubmitContainerStateChangeException","message":"something broke horribly"}`)
	mockRoundTripper.EXPECT().RoundTrip(mock_http.NewHTTPOperationMatcher(versionedOperation("SubmitContainerStateChange"))).AnyTimes().Do(func(_ interface{}) {
		timesCalled++
		resp.Body = operationErrorResp(500, `{"__type":"SubmitContainerStateChangeException","message":"something broke horribly"}`).Body
	}).Return(resp, nil)

	start := ttime.Now()
	err := client.SubmitContainerStateChange(ContainerStateChange{ContainerName: "foo", TaskArn: "bar", Status: ContainerRunning})
	if err == nil {
		t.Fatal("Expected it to error after retrying")
	}
	duration := ttime.Since(start)
	if duration < 23*time.Hour || duration > 25*time.Hour {
		t.Fatal("Retries should have taken roughly 24 hours; took " + duration.String())
	}
	if timesCalled < 10 {
		t.Fatal("Expected to be called many times")
	}
}

func TestSubmitRetriesStopOnSuccess(t *testing.T) {
	ctrl, client, mockRoundTripper := setup(t)
	defer ctrl.Finish()

	resp := operationErrorResp(500, `{"__type":"SubmitContainerStateChangeException","message":"something broke horribly"}`)
	gomock.InOrder(
		mockRoundTripper.EXPECT().RoundTrip(mock_http.NewHTTPOperationMatcher(versionedOperation("SubmitContainerStateChange"))).Times(20).Do(func(_ interface{}) {
			resp.Body = operationErrorResp(500, `{"__type":"SubmitContainerStateChangeException","message":"something broke horribly"}`).Body
		}).Return(resp, nil),
		mockRoundTripper.EXPECT().RoundTrip(mock_http.NewHTTPOperationMatcher(versionedOperation("SubmitContainerStateChange"))).Return(mock_http.SuccessResponse("{}"), nil),
	)

	err := client.SubmitContainerStateChange(ContainerStateChange{ContainerName: "foo", TaskArn: "bar", Status: ContainerRunning})
	if err != nil {
		t.Fatal("Expected it to succeed after retrying")
	}
}

func TestSubmitHandlesNilHttpResponse(t *testing.T) {
	ctrl, client, mockRoundTripper := setup(t)
	defer ctrl.Finish()

	gomock.InOrder(
		mockRoundTripper.EXPECT().RoundTrip(mock_http.NewHTTPOperationMatcher(versionedOperation("SubmitContainerStateChange"))).Return(nil, errors.New("Something went terribly wrong in net/http")),
		mockRoundTripper.EXPECT().RoundTrip(mock_http.NewHTTPOperationMatcher(versionedOperation("SubmitContainerStateChange"))).Return(mock_http.SuccessResponse("{}"), nil),
	)

	err := client.SubmitContainerStateChange(ContainerStateChange{ContainerName: "foo", TaskArn: "bar", Status: ContainerRunning})
	if err != nil {
		t.Fatal("Expected it to succeed after retrying")
	}
}

func TestOtherHandlesNilHttpResponse(t *testing.T) {
	ctrl, client, mockRoundTripper := setup(t)
	defer ctrl.Finish()

	gomock.InOrder(
		mockRoundTripper.EXPECT().RoundTrip(mock_http.NewHTTPOperationMatcher(versionedOperation("DiscoverPollEndpoint"))).Return(nil, errors.New("Something went terribly wrong in net/http")),
		mockRoundTripper.EXPECT().RoundTrip(mock_http.NewHTTPOperationMatcher(versionedOperation("DiscoverPollEndpoint"))).Return(mock_http.SuccessResponse(`{"endpoint":"foobar"}`), nil),
	)

	_, err := client.DiscoverPollEndpoint("foo")
	if err != nil {
		t.Fatal("Expected it to succeed after retrying")
	}
}
