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

// Package wsclient wraps the generated aws-sdk-go client to provide marshalling
// and unmarshalling of data over a websocket connection in the format expected
// by backend. It allows for bidirectional communication and acts as both a
// client-and-server in terms of requests, but only as a client in terms of
// connecting.
package wsclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"crypto/tls"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/cipher"
	"github.com/aws/amazon-ecs-agent/agent/wsclient/wsconn"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/private/protocol/json/jsonutil"
	"github.com/cihub/seelog"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

const (
	// ServiceName defines the service name for the agent. This is used to sign messages
	// that are sent to the backend.
	ServiceName = "ecs"

	// wsConnectTimeout specifies the default connection timeout to the backend.
	wsConnectTimeout = 30 * time.Second

	// wsHandshakeTimeout specifies the default handshake timeout for the websocket client
	wsHandshakeTimeout = wsConnectTimeout

	// readBufSize is the size of the read buffer for the ws connection.
	readBufSize = 4096

	// writeBufSize is the size of the write buffer for the ws connection.
	writeBufSize = 32768

	// Default NO_PROXY env var IP addresses
	defaultNoProxyIP = "169.254.169.254,169.254.170.2"

	errClosed = "use of closed network connection"
)

// ReceivedMessage is the intermediate message used to unmarshal a
// message from backend
type ReceivedMessage struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

// RequestMessage is the intermediate message marshalled to send to backend.
type RequestMessage struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

// RequestHandler would be func(*ecsacs.T for T in ecsacs.*) to be more proper, but it needs
// to be interface{} to properly capture that
type RequestHandler interface{}

// ClientServer is a combined client and server for the backend websocket connection
type ClientServer interface {
	AddRequestHandler(RequestHandler)
	// SetAnyRequestHandler takes a function with the signature 'func(i
	// interface{})' and calls it with every message the server passes down.
	// Only a single 'AnyRequestHandler' will be active at a given time for a
	// ClientServer
	SetAnyRequestHandler(RequestHandler)
	MakeRequest(input interface{}) error
	WriteMessage(input []byte) error
	Connect() error
	IsConnected() bool
	SetConnection(conn wsconn.WebsocketConn)
	Disconnect(...interface{}) error
	Serve() error
	SetReadDeadline(t time.Time) error
	io.Closer
}

// ClientServerImpl wraps commonly used methods defined in ClientServer interface.
type ClientServerImpl struct {
	// AgentConfig is the user-specified runtime configuration
	AgentConfig *config.Config
	// conn holds the underlying low-level websocket connection
	conn wsconn.WebsocketConn
	// CredentialProvider is used to retrieve AWS credentials
	CredentialProvider *credentials.Credentials
	// RequestHandlers is a map from message types to handler functions of the
	// form:
	//     "FooMessage": func(message *ecsacs.FooMessage)
	RequestHandlers map[string]RequestHandler
	// AnyRequestHandler is a request handler that, if set, is called on every
	// message with said message. It will be called before a RequestHandler is
	// called. It must take a single interface{} argument.
	AnyRequestHandler RequestHandler
	// MakeRequestHook is an optional callback that, if set, is called on every
	// generated request with the raw request body.
	MakeRequestHook MakeRequestHookFunc
	// URL is the full url to the backend, including path, querystring, and so on.
	URL string
	// RWTimeout is the duration used for setting read and write deadlines
	// for the websocket connection
	RWTimeout time.Duration
	// writeLock needed to ensure that only one routine is writing to the socket
	writeLock sync.RWMutex
	ClientServer
	ServiceError
	TypeDecoder
}

// MakeRequestHookFunc is a function that is invoked on every generated request
// with the raw request body.  MakeRequestHookFunc must return either the body
// to send or an error.
type MakeRequestHookFunc func([]byte) ([]byte, error)

// Connect opens a connection to the backend and upgrades it to a websocket. Calls to
// 'MakeRequest' can be made after calling this, but responses will not be
// receivable until 'Serve' is also called.
func (cs *ClientServerImpl) Connect() error {
	seelog.Infof("Establishing a Websocket connection to %s", cs.URL)
	parsedURL, err := url.Parse(cs.URL)
	if err != nil {
		return err
	}

	wsScheme, err := websocketScheme(parsedURL.Scheme)
	if err != nil {
		return err
	}
	parsedURL.Scheme = wsScheme

	// NewRequest never returns an error if the url parses and we just verified
	// it did above
	request, _ := http.NewRequest("GET", parsedURL.String(), nil)

	// Sign the request; we'll send its headers via the websocket client which includes the signature
	err = utils.SignHTTPRequest(request, cs.AgentConfig.AWSRegion, ServiceName, cs.CredentialProvider, nil)
	if err != nil {
		return err
	}

	timeoutDialer := &net.Dialer{Timeout: wsConnectTimeout}
	tlsConfig := &tls.Config{ServerName: parsedURL.Host, InsecureSkipVerify: cs.AgentConfig.AcceptInsecureCert}
	cipher.WithSupportedCipherSuites(tlsConfig)

	// Ensure that NO_PROXY gets set
	noProxy := os.Getenv("NO_PROXY")
	if noProxy == "" {
		dockerHost, err := url.Parse(cs.AgentConfig.DockerEndpoint)
		if err == nil {
			dockerHost.Scheme = ""
			os.Setenv("NO_PROXY", fmt.Sprintf("%s,%s", defaultNoProxyIP, dockerHost.String()))
			seelog.Info("NO_PROXY set:", os.Getenv("NO_PROXY"))
		} else {
			seelog.Errorf("NO_PROXY unable to be set: the configured Docker endpoint is invalid.")
		}
	}

	dialer := websocket.Dialer{
		ReadBufferSize:   readBufSize,
		WriteBufferSize:  writeBufSize,
		TLSClientConfig:  tlsConfig,
		Proxy:            http.ProxyFromEnvironment,
		NetDial:          timeoutDialer.Dial,
		HandshakeTimeout: wsHandshakeTimeout,
	}

	websocketConn, httpResponse, err := dialer.Dial(parsedURL.String(), request.Header)
	if httpResponse != nil {
		defer httpResponse.Body.Close()
	}

	if err != nil {
		var resp []byte
		if httpResponse != nil {
			var readErr error
			resp, readErr = ioutil.ReadAll(httpResponse.Body)
			if readErr != nil {
				return fmt.Errorf("Unable to read websocket connection: " + readErr.Error() + ", " + err.Error())
			}
			// If there's a response, we can try to unmarshal it into one of the
			// modeled error types
			possibleError, _, decodeErr := DecodeData(resp, cs.TypeDecoder)
			if decodeErr == nil {
				return cs.NewError(possibleError)
			}
		}
		seelog.Warnf("Error creating a websocket client: %v", err)
		return errors.Wrapf(err, "websocket client: unable to dial %s response: %s",
			parsedURL.Host, string(resp))
	}

	cs.writeLock.Lock()
	defer cs.writeLock.Unlock()

	cs.conn = websocketConn
	seelog.Debugf("Established a Websocket connection to %s", cs.URL)
	return nil
}

// IsReady gives a boolean response that informs the caller if the websocket
// connection is fully established.
func (cs *ClientServerImpl) IsReady() bool {
	cs.writeLock.RLock()
	defer cs.writeLock.RUnlock()

	return cs.conn != nil
}

// SetConnection passes a websocket connection object into the client. This is used only in
// testing and should be avoided in non-test code.
func (cs *ClientServerImpl) SetConnection(conn wsconn.WebsocketConn) {
	cs.conn = conn
}

// SetReadDeadline sets the read deadline for the websocket connection
// A read timeout results in an io error if there are any outstanding reads
// that exceed the deadline
func (cs *ClientServerImpl) SetReadDeadline(t time.Time) error {
	err := cs.conn.SetReadDeadline(t)
	if err == nil {
		return nil
	}
	seelog.Warnf("Unable to set read deadline for websocket connection: %v for %s", err, cs.URL)
	// If we get connection closed error from SetReadDeadline, break out of the for loop and
	// return an error
	if opErr, ok := err.(*net.OpError); ok && strings.Contains(opErr.Err.Error(), errClosed) {
		seelog.Errorf("Stopping redundant reads on closed network connection: %s", cs.URL)
		return opErr
	}
	// An unhandled error has occurred while trying to extend read deadline.
	// Try asynchronously closing the connection. We don't want to be blocked on stale connections
	// taking too long to close. The flip side is that we might start accumulating stale connections.
	// But, that still seems more desirable than waiting for ever for the connection to close
	cs.forceCloseConnection()
	return err
}

func (cs *ClientServerImpl) forceCloseConnection() {
	closeChan := make(chan error, 1)
	go func() {
		closeChan <- cs.Close()
	}()
	ctx, cancel := context.WithTimeout(context.TODO(), wsConnectTimeout)
	defer cancel()
	select {
	case closeErr := <-closeChan:
		if closeErr != nil {
			seelog.Warnf("Unable to close websocket connection: %v for %s",
				closeErr, cs.URL)
		}
	case <-ctx.Done():
		if ctx.Err() != nil {
			seelog.Warnf("Context canceled waiting for termination of websocket connection: %v for %s",
				ctx.Err(), cs.URL)
		}
	}
}

// Disconnect disconnects the connection
func (cs *ClientServerImpl) Disconnect(...interface{}) error {
	cs.writeLock.Lock()
	defer cs.writeLock.Unlock()

	if cs.conn == nil {
		return fmt.Errorf("websocker client: no connection to close")
	}

	// Close() in turn results in a an internal flushFrame() call in gorilla
	// as the close frame needs to be sent to the server. Set the deadline
	// for that as well.
	if err := cs.conn.SetWriteDeadline(time.Now().Add(cs.RWTimeout)); err != nil {
		seelog.Warnf("Unable to set write deadline for websocket connection: %v for %s", err, cs.URL)
	}
	return cs.conn.Close()
}

// AddRequestHandler adds a request handler to this client.
// A request handler *must* be a function taking a single argument, and that
// argument *must* be a pointer to a recognized 'ecsacs' struct.
// E.g. if you desired to handle messages from acs of type 'FooMessage', you
// would pass the following handler in:
//     func(message *ecsacs.FooMessage)
// This function will panic if the passed in function does not have one pointer
// argument or the argument is not a recognized type.
// Additionally, the request handler will block processing of further messages
// on this connection so it's important that it return quickly.
func (cs *ClientServerImpl) AddRequestHandler(f RequestHandler) {
	firstArg := reflect.TypeOf(f).In(0)
	firstArgTypeStr := firstArg.Elem().Name()
	recognizedTypes := cs.GetRecognizedTypes()
	_, ok := recognizedTypes[firstArgTypeStr]
	if !ok {
		panic("AddRequestHandler called with invalid function; argument type not recognized: " + firstArgTypeStr)
	}
	cs.RequestHandlers[firstArgTypeStr] = f
}

// SetAnyRequestHandler passes a RequestHandler object into the client.
func (cs *ClientServerImpl) SetAnyRequestHandler(f RequestHandler) {
	cs.AnyRequestHandler = f
}

// MakeRequest makes a request using the given input. Note, the input *MUST* be
// a pointer to a valid backend type that this client recognises
func (cs *ClientServerImpl) MakeRequest(input interface{}) error {
	send, err := cs.CreateRequestMessage(input)
	if err != nil {
		return err
	}

	if cs.MakeRequestHook != nil {
		send, err = cs.MakeRequestHook(send)
		if err != nil {
			return err
		}
	}

	// Over the wire we send something like
	// {"type":"AckRequest","message":{"messageId":"xyz"}}
	return cs.WriteMessage(send)
}

// WriteMessage wraps the low level websocket write method with a lock
func (cs *ClientServerImpl) WriteMessage(send []byte) error {
	cs.writeLock.Lock()
	defer cs.writeLock.Unlock()

	// This is just future proofing. Ignore the error as the gorilla websocket
	// library returns 'nil' anyway for SetWriteDeadline
	// https://github.com/gorilla/websocket/blob/4201258b820c74ac8e6922fc9e6b52f71fe46f8d/conn.go#L761
	if err := cs.conn.SetWriteDeadline(time.Now().Add(cs.RWTimeout)); err != nil {
		seelog.Warnf("Unable to set write deadline for websocket connection: %v for %s", err, cs.URL)
	}

	return cs.conn.WriteMessage(websocket.TextMessage, send)
}

// ConsumeMessages reads messages from the websocket connection and handles read
// messages from an active connection.
func (cs *ClientServerImpl) ConsumeMessages() error {
	for {
		if err := cs.SetReadDeadline(time.Now().Add(cs.RWTimeout)); err != nil {
			return err
		}
		messageType, message, err := cs.conn.ReadMessage()

		switch {
		case err == nil:
			if messageType != websocket.TextMessage {
				// maybe not fatal though, we'll try to process it anyways
				seelog.Errorf("Unexpected messageType: %v", messageType)
			}
			cs.handleMessage(message)

		case permissibleCloseCode(err):
			seelog.Debugf("Connection closed for a valid reason: %s", err)
			return io.EOF

		default:
			// Unexpected error occurred
			seelog.Debugf("Error getting message from ws backend: error: [%v], messageType: [%v] ",
				err, messageType)
			return err
		}

	}
}

// CreateRequestMessage creates the request json message using the given input.
// Note, the input *MUST* be a pointer to a valid backend type that this
// client recognises.
func (cs *ClientServerImpl) CreateRequestMessage(input interface{}) ([]byte, error) {
	msg := &RequestMessage{}

	recognizedTypes := cs.GetRecognizedTypes()
	for typeStr, typeVal := range recognizedTypes {
		if reflect.TypeOf(input) == reflect.PtrTo(typeVal) {
			msg.Type = typeStr
			break
		}
	}
	if msg.Type == "" {
		return nil, &UnrecognizedWSRequestType{reflect.TypeOf(input).String()}
	}
	messageData, err := jsonutil.BuildJSON(input)
	if err != nil {
		return nil, &NotMarshallableWSRequest{msg.Type, err}
	}
	msg.Message = json.RawMessage(messageData)

	send, err := json.Marshal(msg)
	if err != nil {
		return nil, &NotMarshallableWSRequest{msg.Type, err}
	}
	return send, nil
}

// handleMessage dispatches a message to the correct 'requestHandler' for its
// type. If no request handler is found, the message is discarded.
func (cs *ClientServerImpl) handleMessage(data []byte) {
	typedMessage, typeStr, err := DecodeData(data, cs.TypeDecoder)
	if err != nil {
		seelog.Warnf("Unable to handle message from backend: %v", err)
		return
	}

	seelog.Debugf("Received message of type: %s", typeStr)

	if cs.AnyRequestHandler != nil {
		reflect.ValueOf(cs.AnyRequestHandler).Call([]reflect.Value{reflect.ValueOf(typedMessage)})
	}

	if handler, ok := cs.RequestHandlers[typeStr]; ok {
		reflect.ValueOf(handler).Call([]reflect.Value{reflect.ValueOf(typedMessage)})
	} else {
		seelog.Infof("No handler for message type: %s %s", typeStr, typedMessage)
	}
}

func websocketScheme(httpScheme string) (string, error) {
	// gorilla/websocket expects the websocket scheme (ws[s]://)
	var wsScheme string
	switch httpScheme {
	case "http":
		wsScheme = "ws"
	case "https":
		wsScheme = "wss"
	default:
		return "", fmt.Errorf("wsclient: unknown scheme %s", httpScheme)
	}
	return wsScheme, nil
}

// See https://github.com/gorilla/websocket/blob/87f6f6a22ebfbc3f89b9ccdc7fddd1b914c095f9/conn.go#L650
func permissibleCloseCode(err error) bool {
	return websocket.IsCloseError(err,
		websocket.CloseNormalClosure,     // websocket error code 1000
		websocket.CloseAbnormalClosure,   // websocket error code 1006
		websocket.CloseGoingAway,         // websocket error code 1001
		websocket.CloseInternalServerErr) // websocket error code 1011
}
