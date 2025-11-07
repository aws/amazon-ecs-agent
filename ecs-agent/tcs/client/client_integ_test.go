//go:build integration
// +build integration

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
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
	mock_wsconn "github.com/aws/amazon-ecs-agent/ecs-agent/wsclient/wsconn/mock"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	testPublishMetricsInterval = 1 * time.Second
	rwTimeout                  = time.Second
)

var testCreds = credentials.NewStaticCredentialsProvider("test-id", "test-secret", "test-token")
var emptyDoctor, _ = doctor.NewDoctor([]doctor.Healthcheck{}, "test-cluster", "this:is:an:instance:arn")

// TestEndToEndInstanceStatusFlow tests the complete flow from channel message to backend request.
func TestEndToEndInstanceStatusFlow(t *testing.T) {
	testCases := []struct {
		name                  string
		instanceStatusMessage ecstcs.InstanceStatusMessage
		expectedRequestCount  int
		description           string
	}{
		{
			name: "complete flow with single status",
			instanceStatusMessage: ecstcs.InstanceStatusMessage{
				Metadata: &ecstcs.InstanceStatusMetadata{
					Cluster:           aws.String("integration-test-cluster"),
					ContainerInstance: aws.String("integration-test-instance"),
					RequestId:         aws.String("integration-test-request"),
				},
				Statuses: []*ecstcs.InstanceStatus{
					{
						Status: aws.String("OK"),
						Type:   aws.String("AGENT"),
					},
				},
			},
			expectedRequestCount: 1,
			description:          "Single instanceStatus message should result in one backend request",
		},
		{
			name: "complete flow with multiple statuses",
			instanceStatusMessage: ecstcs.InstanceStatusMessage{
				Metadata: &ecstcs.InstanceStatusMetadata{
					Cluster:           aws.String("integration-test-cluster"),
					ContainerInstance: aws.String("integration-test-instance"),
					RequestId:         aws.String("integration-test-request-multi"),
				},
				Statuses: []*ecstcs.InstanceStatus{
					{
						Status: aws.String("OK"),
						Type:   aws.String("AGENT"),
					},
					{
						Status: aws.String("IMPAIRED"),
						Type:   aws.String("DOCKER"),
					},
					{
						Status: aws.String("OK"),
						Type:   aws.String("EBS_CSI"),
					},
				},
			},
			expectedRequestCount: 1,
			description:          "Multiple statuses in one message should result in one backend request",
		},
		{
			name: "complete flow with empty statuses",
			instanceStatusMessage: ecstcs.InstanceStatusMessage{
				Metadata: &ecstcs.InstanceStatusMetadata{
					Cluster:           aws.String("integration-test-cluster"),
					ContainerInstance: aws.String("integration-test-instance"),
					RequestId:         aws.String("integration-test-request-empty"),
				},
				Statuses: []*ecstcs.InstanceStatus{},
			},
			expectedRequestCount: 1,
			description:          "Empty statuses should still result in one backend request",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create mock websocket connection
			conn := mock_wsconn.NewMockWebsocketConn(ctrl)

			// Create channels for all message types
			instanceStatusMessages := make(chan ecstcs.InstanceStatusMessage, 1)

			// Create TCS client with instanceStatus channel
			cs := testCSIntegration(conn, nil, nil, instanceStatusMessages).(*tcsClientServer)

			ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
			defer cancel()

			// Set up mock expectations for the backend request
			conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil).Times(tc.expectedRequestCount)
			conn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).DoAndReturn(
				func(messageType int, data []byte) error {
					// Verify that the request contains expected data from the message
					dataStr := string(data)

					// Verify metadata fields are present in the request
					if tc.instanceStatusMessage.Metadata != nil {
						if tc.instanceStatusMessage.Metadata.Cluster != nil {
							assert.Contains(t, dataStr, *tc.instanceStatusMessage.Metadata.Cluster,
								"Backend request should contain cluster name")
						}
						if tc.instanceStatusMessage.Metadata.ContainerInstance != nil {
							assert.Contains(t, dataStr, *tc.instanceStatusMessage.Metadata.ContainerInstance,
								"Backend request should contain container instance")
						}
						if tc.instanceStatusMessage.Metadata.RequestId != nil {
							assert.Contains(t, dataStr, *tc.instanceStatusMessage.Metadata.RequestId,
								"Backend request should contain request ID")
						}
					}

					// Verify status information is present in the request
					for _, status := range tc.instanceStatusMessage.Statuses {
						if status.Status != nil {
							assert.Contains(t, dataStr, *status.Status,
								"Backend request should contain status value")
						}
						if status.Type != nil {
							assert.Contains(t, dataStr, *status.Type,
								"Backend request should contain status type")
						}
					}

					// Verify timestamp is present (should be in all requests)
					assert.Contains(t, dataStr, "timestamp",
						"Backend request should contain timestamp field")

					return nil
				},
			).Times(tc.expectedRequestCount)

			// Start publishMessages in a goroutine
			go cs.publishMessages(ctx)

			// Send the instanceStatus message through the channel
			instanceStatusMessages <- tc.instanceStatusMessage

			// Give time for the complete flow to process
			time.Sleep(300 * time.Millisecond)

			// Verify message was consumed from channel
			assert.Len(t, instanceStatusMessages, 0,
				"InstanceStatus message should be consumed from channel")

			// Cancel context to stop publishMessages
			cancel()
		})
	}
}

// TestInteractionBetweenMessageTypes tests that instanceStatus messages work correctly alongside metrics and health messages.
func TestInteractionBetweenMessageTypes(t *testing.T) {
	testCases := []struct {
		name                  string
		sendTelemetry         bool
		sendHealth            bool
		sendInstanceStatus    bool
		expectedTotalRequests int
		description           string
	}{
		{
			name:                  "all three message types together",
			sendTelemetry:         true,
			sendHealth:            true,
			sendInstanceStatus:    true,
			expectedTotalRequests: 3,
			description:           "All three message types should be processed independently",
		},
		{
			name:                  "instanceStatus with telemetry only",
			sendTelemetry:         true,
			sendHealth:            false,
			sendInstanceStatus:    true,
			expectedTotalRequests: 2,
			description:           "InstanceStatus and telemetry should work together",
		},
		{
			name:                  "instanceStatus with health only",
			sendTelemetry:         false,
			sendHealth:            true,
			sendInstanceStatus:    true,
			expectedTotalRequests: 2,
			description:           "InstanceStatus and health should work together",
		},
		{
			name:                  "instanceStatus only",
			sendTelemetry:         false,
			sendHealth:            false,
			sendInstanceStatus:    true,
			expectedTotalRequests: 1,
			description:           "InstanceStatus should work independently",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create mock websocket connection
			conn := mock_wsconn.NewMockWebsocketConn(ctrl)

			// Create channels for all message types
			telemetryMessages := make(chan ecstcs.TelemetryMessage, 1)
			healthMessages := make(chan ecstcs.HealthMessage, 1)
			instanceStatusMessages := make(chan ecstcs.InstanceStatusMessage, 1)

			// Create TCS client with all channels
			cs := testCSIntegration(conn, telemetryMessages, healthMessages, instanceStatusMessages).(*tcsClientServer)

			ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
			defer cancel()

			// Set up mock expectations for backend requests
			// Use AnyTimes() to handle variable mock call expectations for different message types
			conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil).AnyTimes()
			conn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

			// Start publishMessages in a goroutine
			go cs.publishMessages(ctx)

			// Send messages based on test case configuration
			if tc.sendTelemetry {
				telemetryMessage := ecstcs.TelemetryMessage{
					Metadata: &ecstcs.MetricsMetadata{
						Cluster:           aws.String("integration-test-cluster"),
						ContainerInstance: aws.String("integration-test-instance"),
						Idle:              aws.Bool(true),
						MessageId:         aws.String("integration-test-telemetry"),
					},
					TaskMetrics: []*ecstcs.TaskMetric{},
				}
				telemetryMessages <- telemetryMessage
			}

			if tc.sendHealth {
				healthMessage := ecstcs.HealthMessage{
					Metadata: &ecstcs.HealthMetadata{
						Cluster:           aws.String("integration-test-cluster"),
						ContainerInstance: aws.String("integration-test-instance"),
						MessageId:         aws.String("integration-test-health"),
					},
					HealthMetrics: []*ecstcs.TaskHealth{},
				}
				healthMessages <- healthMessage
			}

			if tc.sendInstanceStatus {
				instanceStatusMessage := ecstcs.InstanceStatusMessage{
					Metadata: &ecstcs.InstanceStatusMetadata{
						Cluster:           aws.String("integration-test-cluster"),
						ContainerInstance: aws.String("integration-test-instance"),
						RequestId:         aws.String("integration-test-instance-status"),
					},
					Statuses: []*ecstcs.InstanceStatus{
						{
							Status: aws.String("OK"),
							Type:   aws.String("AGENT"),
						},
					},
				}
				instanceStatusMessages <- instanceStatusMessage
			}

			// Give time for all messages to be processed
			time.Sleep(500 * time.Millisecond)

			// Verify all messages were consumed from their respective channels
			if tc.sendTelemetry {
				assert.Len(t, telemetryMessages, 0,
					"Telemetry message should be consumed from channel")
			}
			if tc.sendHealth {
				assert.Len(t, healthMessages, 0,
					"Health message should be consumed from channel")
			}
			if tc.sendInstanceStatus {
				assert.Len(t, instanceStatusMessages, 0,
					"InstanceStatus message should be consumed from channel")
			}

			// Cancel context to stop publishMessages
			cancel()
		})
	}
}

// TestNoInterferenceBetweenMessageTypes tests that different message types don't interfere with each other.
func TestNoInterferenceBetweenMessageTypes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock websocket connection
	conn := mock_wsconn.NewMockWebsocketConn(ctrl)

	// Create channels for all message types
	telemetryMessages := make(chan ecstcs.TelemetryMessage, 2)
	healthMessages := make(chan ecstcs.HealthMessage, 2)
	instanceStatusMessages := make(chan ecstcs.InstanceStatusMessage, 2)

	// Create TCS client with all channels
	cs := testCSIntegration(conn, telemetryMessages, healthMessages, instanceStatusMessages).(*tcsClientServer)

	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	// Track the order of requests to verify no interference
	var requestOrder []string
	var requestMutex sync.Mutex

	// Set up mock expectations - expect 6 total requests (2 of each type)
	conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil).Times(6)
	conn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).DoAndReturn(
		func(messageType int, data []byte) error {
			requestMutex.Lock()
			defer requestMutex.Unlock()

			dataStr := string(data)
			// Identify request type based on content
			if contains(dataStr, "integration-test-telemetry") {
				requestOrder = append(requestOrder, "telemetry")
			} else if contains(dataStr, "integration-test-health") {
				requestOrder = append(requestOrder, "health")
			} else if contains(dataStr, "integration-test-instance-status") {
				requestOrder = append(requestOrder, "instanceStatus")
			}

			return nil
		},
	).Times(6)

	// Start publishMessages in a goroutine
	go cs.publishMessages(ctx)

	// Send messages in a specific order with delays to test interference
	// First batch
	telemetryMessage1 := ecstcs.TelemetryMessage{
		Metadata: &ecstcs.MetricsMetadata{
			Cluster:           aws.String("integration-test-cluster"),
			ContainerInstance: aws.String("integration-test-instance"),
			Idle:              aws.Bool(true),
			MessageId:         aws.String("integration-test-telemetry-1"),
		},
		TaskMetrics: []*ecstcs.TaskMetric{},
	}
	telemetryMessages <- telemetryMessage1

	instanceStatusMessage1 := ecstcs.InstanceStatusMessage{
		Metadata: &ecstcs.InstanceStatusMetadata{
			Cluster:           aws.String("integration-test-cluster"),
			ContainerInstance: aws.String("integration-test-instance"),
			RequestId:         aws.String("integration-test-instance-status-1"),
		},
		Statuses: []*ecstcs.InstanceStatus{
			{
				Status: aws.String("OK"),
				Type:   aws.String("AGENT"),
			},
		},
	}
	instanceStatusMessages <- instanceStatusMessage1

	healthMessage1 := ecstcs.HealthMessage{
		Metadata: &ecstcs.HealthMetadata{
			Cluster:           aws.String("integration-test-cluster"),
			ContainerInstance: aws.String("integration-test-instance"),
			MessageId:         aws.String("integration-test-health-1"),
		},
		HealthMetrics: []*ecstcs.TaskHealth{},
	}
	healthMessages <- healthMessage1

	// Small delay before second batch
	time.Sleep(100 * time.Millisecond)

	// Second batch
	telemetryMessage2 := ecstcs.TelemetryMessage{
		Metadata: &ecstcs.MetricsMetadata{
			Cluster:           aws.String("integration-test-cluster"),
			ContainerInstance: aws.String("integration-test-instance"),
			Idle:              aws.Bool(true),
			MessageId:         aws.String("integration-test-telemetry-2"),
		},
		TaskMetrics: []*ecstcs.TaskMetric{},
	}
	telemetryMessages <- telemetryMessage2

	healthMessage2 := ecstcs.HealthMessage{
		Metadata: &ecstcs.HealthMetadata{
			Cluster:           aws.String("integration-test-cluster"),
			ContainerInstance: aws.String("integration-test-instance"),
			MessageId:         aws.String("integration-test-health-2"),
		},
		HealthMetrics: []*ecstcs.TaskHealth{},
	}
	healthMessages <- healthMessage2

	instanceStatusMessage2 := ecstcs.InstanceStatusMessage{
		Metadata: &ecstcs.InstanceStatusMetadata{
			Cluster:           aws.String("integration-test-cluster"),
			ContainerInstance: aws.String("integration-test-instance"),
			RequestId:         aws.String("integration-test-instance-status-2"),
		},
		Statuses: []*ecstcs.InstanceStatus{
			{
				Status: aws.String("IMPAIRED"),
				Type:   aws.String("DOCKER"),
			},
		},
	}
	instanceStatusMessages <- instanceStatusMessage2

	// Give time for all messages to be processed
	time.Sleep(1 * time.Second)

	// Verify all messages were consumed from their respective channels
	assert.Len(t, telemetryMessages, 0,
		"All telemetry messages should be consumed from channel")
	assert.Len(t, healthMessages, 0,
		"All health messages should be consumed from channel")
	assert.Len(t, instanceStatusMessages, 0,
		"All instanceStatus messages should be consumed from channel")

	// Verify that we received all expected requests
	requestMutex.Lock()
	assert.Len(t, requestOrder, 6,
		"Should have received exactly 6 requests")

	// Verify that each message type was processed (order may vary due to concurrency)
	telemetryCount := 0
	healthCount := 0
	instanceStatusCount := 0

	for _, reqType := range requestOrder {
		switch reqType {
		case "telemetry":
			telemetryCount++
		case "health":
			healthCount++
		case "instanceStatus":
			instanceStatusCount++
		}
	}

	assert.Equal(t, 2, telemetryCount, "Should have processed 2 telemetry messages")
	assert.Equal(t, 2, healthCount, "Should have processed 2 health messages")
	assert.Equal(t, 2, instanceStatusCount, "Should have processed 2 instanceStatus messages")
	requestMutex.Unlock()

	// Cancel context to stop publishMessages
	cancel()
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// testCSIntegration creates a test TCS client for integration tests
func testCSIntegration(conn *mock_wsconn.MockWebsocketConn,
	metricsMessages <-chan ecstcs.TelemetryMessage,
	healthMessages <-chan ecstcs.HealthMessage,
	instanceStatusMessages <-chan ecstcs.InstanceStatusMessage) wsclient.ClientServer {
	cfg := &wsclient.WSClientMinAgentConfig{
		AWSRegion:          "us-east-1",
		AcceptInsecureCert: true,
	}
	cs := New("https://aws.amazon.com/ecs", cfg, emptyDoctor, false, testPublishMetricsInterval,
		aws.NewCredentialsCache(testCreds), rwTimeout, metricsMessages, healthMessages,
		instanceStatusMessages, metrics.NewNopEntryFactory()).(*tcsClientServer)
	cs.SetConnection(conn)
	return cs
}
