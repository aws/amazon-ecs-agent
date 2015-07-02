package handler_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/aws/amazon-ecs-agent/agent/acs/handler"
	"github.com/aws/amazon-ecs-agent/agent/api/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/gorilla/websocket"

	"github.com/golang/mock/gomock"
)

const samplePayloadMessage = `{"type":"PayloadMessage","message":{"messageId":"123","tasks":[{"taskDefinitionAccountId":"123","containers":[{"environment":{},"name":"name","cpu":1,"essential":true,"memory":1,"portMappings":[],"overrides":"{}","image":"i","mountPoints":[],"volumesFrom":[]}],"version":"3","volumes":[],"family":"f","arn":"arn","desiredStatus":"RUNNING"}],"generatedAt":1,"clusterArn":"1","containerInstanceArn":"1","seqNum":1}}`

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

func TestHandlerReconnects(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := mock_engine.NewMockTaskEngine(ctrl)
	ecsclient := mock_api.NewMockECSClient(ctrl)
	statemanager := statemanager.NewNoopStateManager()

	closeWS := make(chan bool)
	server, serverIn, _, _, err := startMockAcsServer(t, closeWS)
	if err != nil {
		t.Fatal(err)
	}

	ecsclient.EXPECT().DiscoverPollEndpoint("myArn").Return(server.URL, nil).Times(10)
	taskEngine.EXPECT().Version().Return("Docker: 1.5.0", nil).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	ended := make(chan bool, 1)
	go func() {
		handler.StartSession(ctx, handler.StartSessionArguments{"myArn", credentials.AnonymousCredentials, &config.Config{Cluster: "someCluster"}, taskEngine, ecsclient, statemanager, true})
		// This should never return
		ended <- true
	}()
	start := time.Now()
	for i := 0; i < 10; i++ {
		serverIn <- `{"type":"HeartbeatMessage","message":{"healthy":true}}`
		closeWS <- true
	}
	if time.Since(start) > 2*time.Second {
		t.Error("Test took longer than expected; backoff should not have occured for EOF")
	}

	select {
	case <-ended:
		t.Fatal("Should not have stopped session")
	default:
	}
	cancel()
	<-ended
}

func TestHeartbeatOnlyWhenIdle(t *testing.T) {
	testTime := ttime.NewTestTime()
	ttime.SetTime(testTime)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := mock_engine.NewMockTaskEngine(ctrl)
	ecsclient := mock_api.NewMockECSClient(ctrl)
	statemanager := statemanager.NewNoopStateManager()

	closeWS := make(chan bool)
	server, serverIn, requestsChan, errChan, err := startMockAcsServer(t, closeWS)
	defer server.Close()
	defer close(serverIn)

	go func() { <-requestsChan }()
	if err != nil {
		t.Fatal(err)
	}

	// We're testing that it does not reconnect here; must be the case
	ecsclient.EXPECT().DiscoverPollEndpoint("myArn").Return(server.URL, nil).Times(1)
	taskEngine.EXPECT().Version().Return("Docker: 1.5.0", nil).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	ended := make(chan bool, 1)
	go func() {
		handler.StartSession(ctx, handler.StartSessionArguments{"myArn", credentials.AnonymousCredentials, &config.Config{Cluster: "someCluster"}, taskEngine, ecsclient, statemanager, true})
		ended <- true
	}()

	taskAdded := make(chan bool)
	taskEngine.EXPECT().AddTask(gomock.Any()).Do(func(interface{}) {
		taskAdded <- true
	}).Times(10)
	for i := 0; i < 10; i++ {
		serverIn <- samplePayloadMessage
		testTime.Warp(1 * time.Minute)
		<-taskAdded
	}

	select {
	case <-ended:
		t.Fatal("Should not have stop session")
	case err := <-errChan:
		t.Fatal("Error should not have been returned from server", err)
	default:
	}
	cancel()
	<-ended
}

func startMockAcsServer(t *testing.T, closeWS <-chan bool) (*httptest.Server, chan<- string, <-chan string, <-chan error, error) {
	serverChan := make(chan string)
	requestsChan := make(chan string)
	errChan := make(chan error)

	upgrader := websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		go func() {
			<-closeWS
			ws.WriteMessage(websocket.CloseMessage, nil)
			ws.Close()
			errChan <- io.EOF
		}()
		if err != nil {
			errChan <- err
		}
		go func() {
			_, msg, err := ws.ReadMessage()
			if err != nil {
				errChan <- err
			} else {
				requestsChan <- string(msg)
			}
		}()
		for str := range serverChan {
			err := ws.WriteMessage(websocket.TextMessage, []byte(str))
			if err != nil {
				errChan <- err
			}
		}
	})

	server := httptest.NewTLSServer(handler)
	return server, serverChan, requestsChan, errChan, nil
}
