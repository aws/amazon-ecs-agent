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

package acs

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/aws/amazon-ecs-agent/agent/auth"
	"github.com/aws/amazon-ecs-agent/agent/config"
)

const (
	TEST_ACS_ENDPOINT  = "127.0.0.1"
	TEST_ACS_PORT      = 53080
	TEST_CLUSTER_ARN   = "arn:aws:ec2:123:container/cluster:123456"
	TEST_INSTANCE_ARN  = "arn:aws:ec2:123:container/containerInstance/12345678"
	TEST_CERT_PRIV_KEY = "/tmp/ecs_agent_test_cert.key"
	TEST_CERT_CRT      = "/tmp/ecs_agent_test_cert.crt"
)

var client *AgentCommunicationClient

// Tests can set this handler to manipulate how the mock server behaves
var serveHandler http.Handler

func testClient() *AgentCommunicationClient {
	testc := NewAgentCommunicationClient("https://"+TEST_ACS_ENDPOINT,
		&config.Config{Cluster: TEST_CLUSTER_ARN, AWSRegion: "test"},
		auth.TestCredentialProvider{},
		TEST_INSTANCE_ARN)

	// Override port since it's always 443 normally
	testc.port = TEST_ACS_PORT
	return testc
}

func init() {
	client = testClient()
	startMockTlsServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("Default handler should not be used")
	}))
}

func TestCreateAcsUrl(t *testing.T) {
	testUrl := client.createAcsUrl()
	parsed, err := url.Parse(testUrl)
	if err != nil {
		t.Error("Invalid url")
	}

	vals := parsed.Query()
	if vals["clusterArn"][0] != TEST_CLUSTER_ARN {
		t.Error("Wrong cluster arn")
	}
	if vals["containerInstanceArn"][0] != TEST_INSTANCE_ARN {
		t.Error("Wrong instance arn")
	}
}

func writeSelfSignedCert(keypath, certpath string) error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	pub := &priv.PublicKey

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Country:      []string{"US"},
			Organization: []string{"test"},
			CommonName:   TEST_ACS_ENDPOINT,
		},
		NotBefore:          time.Now(),
		NotAfter:           time.Now().Add(5 * time.Minute),
		SignatureAlgorithm: x509.SHA512WithRSA,
		PublicKeyAlgorithm: x509.ECDSA,
	}

	cert, err := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	if err != nil {
		return err
	}
	certFile, err := os.Create(certpath)
	if err != nil {
		return err
	}
	pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: cert})
	certFile.Close()
	keyFile, err := os.Create(keypath)
	if err != nil {
		return err
	}
	pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyFile.Close()
	return nil
}

func startMockTlsServer(handler http.Handler) {
	// ListenAndServeTLS only takes filenames, so we need actual files...
	// Generate them
	err := writeSelfSignedCert(TEST_CERT_PRIV_KEY, TEST_CERT_CRT)
	if err != nil {
		panic(err)
	}

	go func() {
		err = http.ListenAndServeTLS(TEST_ACS_ENDPOINT+":"+strconv.Itoa(TEST_ACS_PORT), TEST_CERT_CRT, TEST_CERT_PRIV_KEY, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serveHandler.ServeHTTP(w, r)
		}))

		if err != nil {
			panic(err)
		}
	}()
}

func startMockAcsServer(t *testing.T, close_ws <-chan bool) (chan<- string, <-chan error, error) {
	serverChan := make(chan string)
	errChan := make(chan error)

	upgrader := websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		go func() {
			<-close_ws
			ws.Close()
		}()
		if err != nil {
			errChan <- err
		}
		for str := range serverChan {
			err := ws.WriteMessage(websocket.TextMessage, []byte(str))
			if err != nil {
				errChan <- err
			}
		}
		errChan <- errors.New("Got closed")
	})
	serveHandler = handler

	return serverChan, errChan, nil
}

func TestPoll(t *testing.T) {
	close_ws := make(chan bool)
	serverChan, serverErr, err := startMockAcsServer(t, close_ws)
	defer func() {
		os.Remove(TEST_CERT_PRIV_KEY)
		os.Remove(TEST_CERT_CRT)
	}()

	if err != nil {
		t.Fatal(err)
	}

	go func() {
		t.Fatal(<-serverErr)
	}()

	taskChannel, errChannel, err := client.Poll(true)
	for i := 0; i < 10 && err != nil; i++ {
		time.Sleep(200 * time.Millisecond) // Give the mock server time to listen
		taskChannel, errChannel, err = client.Poll(true)
	}
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			pollErr := <-errChannel
			t.Fatal(pollErr)
		}
	}()

	go func() {
		serverChan <- `{"type":"PayloadMessage","message":{"tasks":[{"arn":"arn1","desiredStatus":"RUNNING","overrides":{},"family":"test","version":"v1","containers":[{"name":"c1","image":"redis","command":["arg1","arg2"],"cpu":10,"memory":20,"links":["db"],"portMappings":[{"containerPort":22,"hostPort":22}],"essential":true,"entryPoint":["bash"],"environment":{"key":"val"},"overrides":{},"desiredStatus":"RUNNING"}]}],"messageId":"messageId"}}` + "\n"
	}()

	response, open := <-taskChannel
	if !open {
		t.Fatal("task channel closed on us")
	}
	task := response.Tasks[0]
	if task.Arn != "arn1" {
		t.Error("Wrong task recieved. Its arn was: " + task.Arn)
	}
	if task.Containers[0].Overrides.Command != nil {
		t.Error("There were no overrides, Command should have been nil")
	}

	// Test an override
	go func() {
		serverChan <- `{"type":"PayloadMessage","message":{"tasks":[{"arn":"arn2","desiredStatus":"RUNNING","overrides":{},"family":"test","version":"v1","containers":[{"name":"c1","image":"redis","command":["arg1","arg2"],"cpu":10,"memory":20,"links":["db"],"portMappings":[{"containerPort":22,"hostPort":22}],"essential":true,"entryPoint":["bash"],"environment":{"key":"val"},"overrides":{"command":["overridecmd"]},"desiredStatus":"RUNNING"}]}],"messageId":"messageId"}}` + "\n"
	}()
	response, open = <-taskChannel
	if !open {
		t.Fatal("task channel closed on us")
	}
	task = response.Tasks[0]
	if task.Arn != "arn2" {
		t.Error("Wrong task recieved. Its arn was: " + task.Arn)
	}

	command := task.Containers[0].Overrides.Command
	if command == nil || !reflect.DeepEqual(command, &[]string{"overridecmd"}) {
		t.Error("Command overrides were not correct: got", command)
	}

	// Test that closing it works well
	close_ws <- true
	_, open = <-taskChannel
	if open {
		t.Error("Expected ws to be closed and thus taskChannel to no longer work")
	}
}

func TestMalformedUrl(t *testing.T) {
	badclient := testClient()
	badclient.endpoint = "fake-endpoint-that-won't-parse%%"

	_, _, err := badclient.Poll(true)
	if err == nil {
		t.Error("Did not get an error when trying to use a malformed endpoint url")
	}
}

func TestBadUrl(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping url timeout test")
	}

	badclient := testClient()
	badclient.endpoint = "https://example.com"
	_, _, err := badclient.Poll(true)
	if err == nil {
		t.Error("Could poll against example.com")
	}
}

func TestBadServerNonWebsocket(t *testing.T) {
	serveHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "Sorry, I'm not a websocket server")
	})

	badclient := testClient()
	_, _, err := badclient.Poll(true)
	if err == nil {
		t.Error("Polling a non-websocket server didn't produce an error")
	}

}
