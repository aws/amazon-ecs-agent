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

package handlers

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

const TEST_CONTAINER_INSTANCE_ARN = "test_container_instance_arn"
const TEST_CLUSTER_ARN = "test_cluster_arn"

func TestMetadataHandler(t *testing.T) {
	metadataHandler := MetadataV1RequestHandlerMaker(utils.Strptr(TEST_CONTAINER_INSTANCE_ARN), &config.Config{ClusterArn: TEST_CLUSTER_ARN})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "http://localhost:"+strconv.Itoa(config.AGENT_INTROSPECTION_PORT), nil)
	metadataHandler(w, req)

	var resp MetadataResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.ClusterArn != TEST_CLUSTER_ARN {
		t.Error("Metadata returned the wrong cluster arn")
	}
	if *resp.ContainerInstanceArn != TEST_CONTAINER_INSTANCE_ARN {
		t.Error("Metadata returned the wrong cluster arn")
	}
}

type mockTaskEngine struct{}

func (*mockTaskEngine) TaskEvents() <-chan api.ContainerStateChange {
	return nil
}

func (*mockTaskEngine) Init() error                  { return nil }
func (*mockTaskEngine) MustInit()                    {}
func (*mockTaskEngine) AddTask(*api.Task)            {}
func (*mockTaskEngine) MarshalJSON() ([]byte, error) { return []byte{}, errors.New("Mock") }
func (*mockTaskEngine) UnmarshalJSON([]byte) error   { return errors.New("Mock") }
func (*mockTaskEngine) SetSaver(statemanager.Saver)   {}
func (*mockTaskEngine) ListTasks() ([]*api.Task, error) {
	return []*api.Task{}, errors.New("Mock")
}

func TestServeHttp(t *testing.T) {
	go ServeHttp(utils.Strptr(TEST_CONTAINER_INSTANCE_ARN), &mockTaskEngine{}, &config.Config{ClusterArn: TEST_CLUSTER_ARN})

	resp, err := http.Get("http://localhost:" + strconv.Itoa(config.AGENT_INTROSPECTION_PORT) + "/v1/metadata")
	if err != nil {
		t.Fatal(err)
	}
	var metadata MetadataResponse
	body, err := ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &metadata)

	if metadata.ClusterArn != TEST_CLUSTER_ARN {
		t.Error("Metadata returned the wrong cluster arn")
	}
	if *metadata.ContainerInstanceArn != TEST_CONTAINER_INSTANCE_ARN {
		t.Error("Metadata returned the wrong cluster arn")
	}
}
