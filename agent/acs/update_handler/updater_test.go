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

package updater

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"code.google.com/p/gomock/gomock"

	"github.com/aws/amazon-ecs-agent/agent/acs/client/mock"
	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/acs/update_handler/os/mock"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/httpclient"
	"github.com/aws/amazon-ecs-agent/agent/httpclient/mock"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
)

func ptr(i interface{}) interface{} {
	n := reflect.New(reflect.TypeOf(i))
	n.Elem().Set(reflect.ValueOf(i))
	return n.Interface()
}

func mocks(t *testing.T, cfg *config.Config) (*gomock.Controller, *config.Config, *mock_os.MockFileSystem, *mock_client.MockClientServer, *mock_http.MockRoundTripper) {
	if cfg == nil {
		cfg = &config.Config{
			UpdatesEnabled:    true,
			UpdateDownloadDir: "/tmp/test/",
		}
	}
	ctrl := gomock.NewController(t)

	mockfs := mock_os.NewMockFileSystem(ctrl)
	mockacs := mock_client.NewMockClientServer(ctrl)
	mockhttp := mock_http.NewMockRoundTripper(ctrl)
	httpclient.Transport = mockhttp

	return ctrl, cfg, mockfs, mockacs, mockhttp
}

func TestFullUpdateFlow(t *testing.T) {
	ctrl, cfg, mockfs, mockacs, mockhttp := mocks(t, nil)
	defer ctrl.Finish()

	// First, it'll try to download the file
	mockhttp.EXPECT().RoundTrip(mock_http.NewHTTPSimpleMatcher("GET", "https://update.tld")).Return(mock_http.SuccessResponse("update-tar-data"), nil)

	// And then write the new update tarball
	var writtenFile bytes.Buffer
	mockfs.EXPECT().Create(gomock.Any()).Return(mock_os.NopReadWriteCloser(&writtenFile), nil)
	// Write that it's available
	mockfs.EXPECT().WriteFile("/tmp/test/desired-image", gomock.Any(), gomock.Any()).Return(nil)

	// And then ack it
	mockacs.EXPECT().MakeRequest(gomock.Eq(&ecsacs.AckRequest{
		Cluster:           ptr("cluster").(*string),
		ContainerInstance: ptr("containerInstance").(*string),
		MessageId:         ptr("mid").(*string),
	}))

	u := &updater{
		acs:    mockacs,
		config: cfg,
		fs:     mockfs,
	}
	u.stageUpdateHandler()(&ecsacs.StageUpdateMessage{
		ClusterArn:           ptr("cluster").(*string),
		ContainerInstanceArn: ptr("containerInstance").(*string),
		MessageId:            ptr("mid").(*string),
		UpdateInfo: &ecsacs.UpdateInfo{
			Location:  ptr("https://update.tld").(*string),
			Signature: ptr("6caeef375a080e3241781725b357890758d94b15d7ce63f6b2ff1cb5589f2007").(*string),
		},
	})

	if writtenFile.String() != "update-tar-data" {
		t.Error("Incorrect data written")
	}

	// Now that we've staged the update, we can proceed with it
	mockfs.EXPECT().Exit(42)

	u.performUpdateHandler(statemanager.NewNoopStateManager(), engine.NewTaskEngine(cfg))(&ecsacs.PerformUpdateMessage{
		ClusterArn:           ptr("cluster").(*string),
		ContainerInstanceArn: ptr("containerInstance").(*string),
		MessageId:            ptr("mid2").(*string),
		UpdateInfo: &ecsacs.UpdateInfo{
			Location:  ptr("https://update.tld").(*string),
			Signature: ptr("c54518806ff4d14b680c35784113e1e7478491fe").(*string),
		},
	})
}

type nackRequestMatcher struct {
	*ecsacs.NackRequest
}

func (m *nackRequestMatcher) Matches(nack interface{}) bool {
	other := nack.(*ecsacs.NackRequest)
	if m.Cluster != nil && *m.Cluster != *other.Cluster {
		return false
	}
	if m.ContainerInstance != nil && *m.ContainerInstance != *other.ContainerInstance {
		fmt.Println(*other.ContainerInstance)
		return false
	}
	if m.MessageId != nil && *m.MessageId != *other.MessageId {
		fmt.Println(*other.MessageId)
		return false
	}
	if m.Reason != nil && *m.Reason != *other.Reason {
		return false
	}
	return true
}

func TestMissingUpdateInfo(t *testing.T) {
	ctrl, cfg, mockfs, mockacs, _ := mocks(t, nil)
	defer ctrl.Finish()

	u := &updater{
		acs:    mockacs,
		config: cfg,
		fs:     mockfs,
	}

	mockacs.EXPECT().MakeRequest(&nackRequestMatcher{&ecsacs.NackRequest{
		Cluster:           ptr("cluster").(*string),
		ContainerInstance: ptr("containerInstance").(*string),
		MessageId:         ptr("mid").(*string),
	}})

	u.stageUpdateHandler()(&ecsacs.StageUpdateMessage{
		ClusterArn:           ptr("cluster").(*string),
		ContainerInstanceArn: ptr("containerInstance").(*string),
		MessageId:            ptr("mid").(*string),
	})
}

func (m *nackRequestMatcher) String() string {
	return fmt.Sprintf("Nack request matcher %v", m.NackRequest)
}

func TestUndownloadedUpdate(t *testing.T) {
	ctrl, cfg, mockfs, mockacs, _ := mocks(t, nil)
	defer ctrl.Finish()

	u := &updater{
		acs:    mockacs,
		config: cfg,
		fs:     mockfs,
	}

	mockacs.EXPECT().MakeRequest(&nackRequestMatcher{&ecsacs.NackRequest{
		Cluster:           ptr("cluster").(*string),
		ContainerInstance: ptr("containerInstance").(*string),
		MessageId:         ptr("mid").(*string),
	}})

	u.stageUpdateHandler()(&ecsacs.StageUpdateMessage{
		ClusterArn:           ptr("cluster").(*string),
		ContainerInstanceArn: ptr("containerInstance").(*string),
		MessageId:            ptr("mid").(*string),
	})
}
