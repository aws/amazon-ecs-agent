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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/cipher"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/httpproxy"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient/wsconn"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/private/protocol/json/jsonutil"
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

	// ExitTerminal indicates the agent run into error that's not recoverable
	// no need to restart
	ExitTerminal = 5

	// disconnectTimeout is the maximum time taken by the server side (TACS/ACS) to send a
	// disconnect payload for the Agent.
	DisconnectTimeout = 30 * time.Minute

	// disconnectJitterMax is the maximum jitter time chosen as reasonable initial value
	// to prevent mass retries at the same time from multiple clients/tasks synchronizing.
	DisconnectJitterMax = 5 * time.Minute

	// dateTimeFormat is a string format to format time for better readability: YYYY-MM-DD hh:mm:ss
	dateTimeFormat = "2006-01-02 15:04:05"
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

// RequestResponder wraps the RequestHandler interface with a Respond()
// method that can be used to Respond to requests read and processed via
// the RequestHandler interface for a particular message type.
//
// Example:
//
//	type payloadMessageDispatcher struct {
//	    respond  func(interface{}) error
//	    dispatcher actor.Dispatcher
//	}
//	func(d *payloadmessagedispatcher) HandlerFunc() RequestHandler {
//	    return func(payload *ecsacs.PayloadMessage) {
//	        message := &actor.DispatcherMessage{
//	            Payload: payload,
//	            AckFunc: func() error {
//	                return d.respond()
//	            },
//	            ...
//	        }
//	        d.dispatcher.Send(message)
//	    }
//	}
type RequestResponder interface {
	// Name returns the name of the responder. This is used mostly for logging.
	Name() string
	// HandlerFunc returns the RequestHandler callback for a particular
	// websocket request message type.
	HandlerFunc() RequestHandler
}

// RespondFunc specifies a function callback that can be used by the
// RequestResponder to respond to requests.
type RespondFunc func(interface{}) error

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
	WriteCloseMessage() error
	Connect(disconnectMetricName string, disconnectTimeout time.Duration, disconnectJitterMax time.Duration) (*time.Timer, error)
	IsConnected() bool
	SetConnection(conn wsconn.WebsocketConn)
	Disconnect(...interface{}) error
	Serve(ctx context.Context) error
	SetReadDeadline(t time.Time) error
	CloseClient(t time.Time, dur time.Duration) error
	io.Closer
}

// WSClientMinAgentConfig is a subset of agent's config.
type WSClientMinAgentConfig struct {
	AWSRegion          string
	AcceptInsecureCert bool
	DockerEndpoint     string
	IsDocker           bool
}

// ClientServerImpl wraps commonly used methods defined in ClientServer interface.
type ClientServerImpl struct {
	// Cfg is the subset of user-specified runtime configuration
	Cfg *WSClientMinAgentConfig
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
	// MetricsFactory needed to emit metrics for monitoring.
	MetricsFactory metrics.EntryFactory
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
func (cs *ClientServerImpl) Connect(disconnectMetricName string,
	disconnectTimeout time.Duration,
	disconnectJitterMax time.Duration) (*time.Timer, error) {

	logger.Info("Establishing a Websocket connection", logger.Fields{
		"url": cs.URL,
	})
	parsedURL, err := url.Parse(cs.URL)
	if err != nil {
		return nil, err
	}

	wsScheme, err := websocketScheme(parsedURL.Scheme)
	if err != nil {
		return nil, err
	}
	parsedURL.Scheme = wsScheme

	// NewRequest never returns an error if the url parses and we just verified
	// it did above
	request, _ := http.NewRequest("GET", parsedURL.String(), nil)

	// Sign the request; we'll send its headers via the websocket client which includes the signature
	err = utils.SignHTTPRequest(request, cs.Cfg.AWSRegion, ServiceName, cs.CredentialProvider, nil)
	if err != nil {
		return nil, err
	}

	timeoutDialer := &net.Dialer{Timeout: wsConnectTimeout}
	tlsConfig := &tls.Config{ServerName: parsedURL.Host, InsecureSkipVerify: cs.Cfg.AcceptInsecureCert, MinVersion: tls.VersionTLS12}

	//TODO: In order to get rid of the check -
	// 1. Remove the hardcoded cipher suites, and rely on default by tls package
	// 2. NO_PROXY should be set as part of config check or in init somewhere. Wsclient is not the right place.
	if cs.Cfg.IsDocker {

		cipher.WithSupportedCipherSuites(tlsConfig)

		// Ensure that NO_PROXY gets set
		noProxy := os.Getenv("NO_PROXY")
		if noProxy == "" {
			dockerHost, err := url.Parse(cs.Cfg.DockerEndpoint)
			if err == nil {
				dockerHost.Scheme = ""
				os.Setenv("NO_PROXY", fmt.Sprintf("%s,%s", defaultNoProxyIP, dockerHost.String()))
				logger.Info(fmt.Sprintf("NO_PROXY is set: %s", os.Getenv("NO_PROXY")))
			} else {
				logger.Error("NO_PROXY unable to be set: the configured Docker endpoint is invalid.")
			}
		}
	}

	dialer := websocket.Dialer{
		ReadBufferSize:   readBufSize,
		WriteBufferSize:  writeBufSize,
		TLSClientConfig:  tlsConfig,
		Proxy:            httpproxy.Proxy,
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
			resp, readErr = io.ReadAll(httpResponse.Body)
			if readErr != nil {
				return nil, fmt.Errorf("Unable to read websocket connection: " + readErr.Error() + ", " + err.Error())
			}
			// If there's a response, we can try to unmarshal it into one of the
			// modeled error types
			possibleError, _, decodeErr := DecodeData(resp, cs.TypeDecoder)
			if decodeErr == nil {
				return nil, cs.NewError(possibleError)
			}
		}
		logger.Warn(fmt.Sprintf(
			"Error creating a websocket client: %v", err))
		return nil, errors.Wrapf(err, "websocket client: unable to dial %s response: %s",
			parsedURL.Host, string(resp))
	}

	cs.writeLock.Lock()
	defer cs.writeLock.Unlock()

	cs.conn = websocketConn

	startTime := time.Now()
	// newDisconnectTimeoutTimerHandler returns a timer.Afterfunc(timeout, f) which will
	// call f as goroutine after timeout. The timeout is currently set to 30m+jitter(5m max) to match max duration
	// of connection with server(ACS/TACS).
	disconnectTimer := cs.newDisconnectTimeoutHandler(startTime, disconnectMetricName, disconnectTimeout, disconnectJitterMax)
	return disconnectTimer, nil
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
	logger.Warn(fmt.Sprintf("Unable to set read deadline for websocket connection: %v for %s", err, cs.URL))
	// If we get connection closed error from SetReadDeadline, break out of the for loop and
	// return an error
	if opErr, ok := err.(*net.OpError); ok && strings.Contains(opErr.Err.Error(), errClosed) {
		logger.Error(fmt.Sprintf("Stopping redundant reads on closed network connection: %s", cs.URL))
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
			logger.Warn(fmt.Sprintf("Unable to close websocket connection: %v for %s",
				closeErr, cs.URL))
		}
	case <-ctx.Done():
		if ctx.Err() != nil {
			logger.Warn(fmt.Sprintf("Context canceled waiting for termination of websocket connection: %v for %s",
				ctx.Err(), cs.URL))
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
		logger.Warn(fmt.Sprintf("Unable to set write deadline for websocket connection: %v for %s", err, cs.URL))
	}
	return cs.conn.Close()
}

// AddRequestHandler adds a request handler to this client.
// A request handler *must* be a function taking a single argument, and that
// argument *must* be a pointer to a recognized 'ecsacs' struct.
// E.g. if you desired to handle messages from acs of type 'FooMessage', you
// would pass the following handler in:
//
//	func(message *ecsacs.FooMessage)
//
// This function will cause agent exit if the passed in function does not have one pointer
// argument or the argument is not a recognized type.
// Additionally, the request handler will block processing of further messages
// on this connection so it's important that it return quickly.
func (cs *ClientServerImpl) AddRequestHandler(f RequestHandler) {
	firstArg := reflect.TypeOf(f).In(0)
	firstArgTypeStr := firstArg.Elem().Name()
	recognizedTypes := cs.GetRecognizedTypes()
	_, ok := recognizedTypes[firstArgTypeStr]
	if !ok {
		logger.Error(fmt.Sprintf("Invalid Handler. AddRequestHandler called with invalid function; "+
			"argument type not recognized: %v", firstArgTypeStr))
		os.Exit(ExitTerminal)
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

	// It is possible that the client implementing may invoke "WriteMessage" before calling a "Connect".
	// It would lead to a nil pointer exception as the cs.conn value will not be set.
	// Returning error messages in such cases asking the client to Connect.
	if cs.conn == nil {
		return errors.New("the connection is currently nil. Please connect and try again.")
	}
	// This is just future proofing. Ignore the error as the gorilla websocket
	// library returns 'nil' anyway for SetWriteDeadline
	// https://github.com/gorilla/websocket/blob/4201258b820c74ac8e6922fc9e6b52f71fe46f8d/conn.go#L761
	if err := cs.conn.SetWriteDeadline(time.Now().Add(cs.RWTimeout)); err != nil {
		logger.Warn(fmt.Sprintf("Unable to set write deadline for websocket connection: %v for %s",
			err, cs.URL))
	}

	return cs.conn.WriteMessage(websocket.TextMessage, send)
}

// WriteCloseMessage wraps the low level websocket WriteControl method with a lock, and sends a message of type
// CloseMessage (Ref: https://github.com/gorilla/websocket/blob/9111bb834a68b893cebbbaed5060bdbc1d9ab7d2/conn.go#L74)
func (cs *ClientServerImpl) WriteCloseMessage() error {
	cs.writeLock.Lock()
	defer cs.writeLock.Unlock()

	send := websocket.FormatCloseMessage(websocket.CloseNormalClosure,
		"ConnectionExpired: Reconnect to continue")

	return cs.conn.WriteControl(websocket.CloseMessage, send, time.Now().Add(cs.RWTimeout))
}

// ConsumeMessages reads messages from the websocket connection and handles read
// messages from an active connection.
func (cs *ClientServerImpl) ConsumeMessages(ctx context.Context) error {
	// Since ReadMessage is blocking, we don't want to wait for timeout when context gets cancelled
	errChan := make(chan error, 1)
	go func() {
		for {
			if err := cs.SetReadDeadline(time.Now().Add(cs.RWTimeout)); err != nil {
				errChan <- err
				return
			}
			messageType, message, err := cs.conn.ReadMessage()

			switch {
			case err == nil:
				if messageType != websocket.TextMessage {
					// maybe not fatal though, we'll try to process it anyways
					logger.Error(fmt.Sprintf("Unexpected messageType: %v", messageType))
				}

				cs.handleMessage(message)

			case permissibleCloseCode(err):
				logger.Debug(fmt.Sprintf("Connection closed for a valid reason: %s", err))
				errChan <- io.EOF
				return

			default:
				// Unexpected error occurred
				logger.Debug(fmt.Sprintf("Error getting message from ws backend: error: [%v], messageType: [%v] ",
					err, messageType))
				errChan <- err
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			// Close connection and wait for Read goroutine to finish
			_ = cs.Disconnect()
			<-errChan
			return ctx.Err()
		case err := <-errChan:
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
		logger.Warn(fmt.Sprintf("Unable to handle message from backend: %v", err))
		return
	}

	logger.Debug(fmt.Sprintf("Received message of type: %s", typeStr))

	if cs.AnyRequestHandler != nil {
		reflect.ValueOf(cs.AnyRequestHandler).Call([]reflect.Value{reflect.ValueOf(typedMessage)})
	}

	if handler, ok := cs.RequestHandlers[typeStr]; ok {
		reflect.ValueOf(handler).Call([]reflect.Value{reflect.ValueOf(typedMessage)})
	} else {
		logger.Info(fmt.Sprintf("No handler for message type: %s %s", typeStr, typedMessage))
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

// newDisconnectTimeoutHandler returns new timer object to disconnect from server connection start time, with goroutine
// to disconnect from client.
func (cs *ClientServerImpl) newDisconnectTimeoutHandler(startTime time.Time,
	metricName string,
	disconnectTimeout time.Duration,
	disconnectJitterMax time.Duration) *time.Timer {

	maxConnectionDuration := retry.AddJitter(disconnectTimeout, disconnectJitterMax)
	logger.Info(("Websocket connection established."), logger.Fields{
		"URL":                    cs.URL,
		"ConnectTime":            startTime.Format(dateTimeFormat),
		"ExpectedDisconnectTime": startTime.Add(disconnectTimeout).Format(dateTimeFormat),
	})

	timer := time.AfterFunc(maxConnectionDuration, func() {
		err := cs.CloseClient(startTime, maxConnectionDuration)
		cs.MetricsFactory.New(metricName).Done(err)
	})
	return timer
}

// closeClient will attempt to close the provided client, retries are not recommended
// as failure modes for this are when client is not found or already closed.
func (cs *ClientServerImpl) CloseClient(startTime time.Time, timeoutDuration time.Duration) error {
	logger.Warn(("Closing connection"), logger.Fields{
		"URL":                  cs.URL,
		"ConnectionStartTime":  startTime.Format(dateTimeFormat),
		"MaxDisconnectionTime": startTime.Add(timeoutDuration).Format(dateTimeFormat),
	})
	err := cs.WriteCloseMessage()
	if err != nil {
		logger.Warn(("Error disconnecting client."), logger.Fields{
			"URL":       cs.URL,
			field.Error: err,
		})
	}
	logger.Info(("Disconnected from server."), logger.Fields{
		"URL": cs.URL,
	})
	return err
}
