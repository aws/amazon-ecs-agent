//go:build linux && unit

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

package stats

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

func TestRetrieveServiceConnectMetrics(t *testing.T) {

	t1 := &apitask.Task{
		Arn:               "t1",
		Family:            "f1",
		ENIs:              []*apieni.ENI{{ID: "ec2Id"}},
		KnownStatusUnsafe: apitaskstatus.TaskRunning,
		Containers: []*apicontainer.Container{
			{Name: "test"},
		},
		LocalIPAddressUnsafe: "127.0.0.1",
	}

	// Set up a mock http sever on the statsUrlpath
	statsMockurl := "127.0.0.1:9901"
	r := mux.NewRouter()
	r.HandleFunc("/stats/prometheus", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Service Connect Stats")
	}))

	ts := httptest.NewUnstartedServer(r)

	l, err := net.Listen("tcp", statsMockurl)
	if err != nil {
		fmt.Printf("%v", err)
	}
	ts.Listener.Close()
	ts.Listener = l
	ts.Start()
	defer ts.Close()

	serviceConnectStats := &ServiceConnectStats{}
	serviceConnectStats.retrieveServiceConnectStats(t1)

	assert.Equal(t, "Service Connect Stats\n", serviceConnectStats.GetStats())
}
