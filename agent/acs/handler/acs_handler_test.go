package handler_test

import (
	"net/url"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/acs/handler"
	"github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	"github.com/aws/amazon-ecs-agent/agent/version"

	"code.google.com/p/gomock/gomock"
)

func TestAcsWsUrl(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := mock_engine.NewMockTaskEngine(ctrl)

	taskEngine.EXPECT().Version().Return("Docker version result", nil)

	wsurl := handler.AcsWsUrl("http://endpoint.tld", "myCluster", "myContainerInstance", taskEngine)

	parsed, err := url.Parse(wsurl)
	if err != nil {
		t.Fatal("Should be able to parse url")
	}

	if parsed.Path != "/ws" {
		t.Fatal("Wrong path")
	}

	if parsed.Query().Get("clusterArn") != "myCluster" {
		t.Fatal("Wrong cluster")
	}
	if parsed.Query().Get("containerInstanceArn") != "myContainerInstance" {
		t.Fatal("Wrong cluster")
	}
	if parsed.Query().Get("agentVersion") != version.Version {
		t.Fatal("Wrong cluster")
	}
	if parsed.Query().Get("agentHash") != version.GitHashString() {
		t.Fatal("Wrong cluster")
	}

	if parsed.Query().Get("dockerVersion") != "Docker version result" {
		t.Fatal("Wrong docker version")
	}
}
