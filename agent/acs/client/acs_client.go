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

// Package acsclient wraps the generated aws-sdk-go client to provide marshalling
// and unmarshalling of data over a websocket connection in the format expected
// by ACS. It allows for bidirectional communication and acts as both a
// client-and-server in terms of requests, but only as a client in terms of
// connecting.
package acsclient

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/awslabs/aws-sdk-go/internal/protocol/json/jsonutil"
	"github.com/gorilla/websocket"
)

var log = logger.ForModule("acs client")

const acsService = "ecs"
const acsConnectTimeout = 3 * time.Second

// websocketConn specifies the subset of gorilla/websocket's
// connection's methods that this client uses.
type websocketConn interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, data []byte, err error)
	Close() error
}

// More properly this would be func(*ecsacs.T for T in ecsacs.*), but it needs
// to be interface{} to properly capture that
type RequestHandler interface{}

// ClientServer is a combined client and server for the ACS websocket connection
type ClientServer interface {
	AddRequestHandler(RequestHandler)
	MakeRequest(input interface{}) error
	Connect() error
	Serve() error
	io.Closer
}

//go:generate mockgen.sh github.com/aws/amazon-ecs-agent/agent/acs/client ClientServer mock/$GOFILE

// default implementation of ClientServer
type clientServer struct {
	// url is the full url to ACS, including path, querystring, and so on.
	url                string
	region             string
	credentialProvider credentials.AWSCredentialProvider
	acceptInvalidCert  bool

	conn websocketConn

	// requestHandlers is a map from message types to handler functions of the
	// form:
	//     "FooMessage": func(message *ecsacs.FooMessage)
	requestHandlers map[string]RequestHandler
}

// receivedMessage is the intermediate message used to unmarshal a message from ACS
type receivedMessage struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

// requestMessage is the intermediate message marshalled to send to ACS.
type requestMessage struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

// New returns a client/server to bidirectionally communicate with ACS
// The returned struct should have both 'Connect' and 'Serve' called upon it
// before being used.
func New(url string, region string, credentialProvider credentials.AWSCredentialProvider, acceptInvalidCert bool) ClientServer {
	cs := &clientServer{url: url, region: region, credentialProvider: credentialProvider, acceptInvalidCert: acceptInvalidCert}
	cs.requestHandlers = make(map[string]RequestHandler)
	return cs
}

// Connect opens a connection to ACS and upgrades it to a websocket. Calls to
// 'MakeRequest' can be made after calling this, but responss will not be
// receivable until 'Serve' is also called.
func (cs *clientServer) Connect() error {
	parsedAcsURL, err := url.Parse(cs.url)
	if err != nil {
		return err
	}

	signer := authv4.NewHttpSigner(cs.region, acsService, cs.credentialProvider, nil)

	// NewRequest never returns an error if the url parses and we just verified
	// it did above
	request, _ := http.NewRequest("GET", cs.url, nil)
	signer.SignHttpRequest(request)

	// url.Host might not have the port, but tls.Dial needs it
	dialHost := parsedAcsURL.Host
	if !strings.Contains(dialHost, ":") {
		dialHost += ":443"
	}

	timeoutDialer := &net.Dialer{Timeout: acsConnectTimeout}
	log.Info("Creating poll dialer", "host", parsedAcsURL.Host)
	acsConn, err := tls.DialWithDialer(timeoutDialer, "tcp", dialHost, &tls.Config{InsecureSkipVerify: cs.acceptInvalidCert})
	if err != nil {
		return err
	}

	websocketConn, httpResponse, err := websocket.NewClient(acsConn, parsedAcsURL, request.Header, 1024, 1024)
	if httpResponse != nil {
		defer httpResponse.Body.Close()
	}
	if err != nil {
		var resp []byte
		if httpResponse != nil {
			var readErr error
			resp, readErr = ioutil.ReadAll(httpResponse.Body)
			if readErr != nil {
				return errors.New("Unable to read websocket connection: " + readErr.Error() + ", " + err.Error())
			}
			// If there's a response, we can try to unmarshal it into one of the
			// modeled acs error types
			possibleError, _, decodeErr := decodeData(resp)
			if decodeErr == nil {
				return NewACSError(possibleError)
			}
		}
		log.Warn("Error creating a websocket client", "err", err)
		return errors.New(string(resp) + ", " + err.Error())
	}
	cs.conn = websocketConn
	return nil
}

// Serve begins serving requests using previously registered handlers (see
// AddRequestHandler). All request handlers should be added prior to making this
// call as unhandled requests will be discarded.
func (cs *clientServer) Serve() error {
	log.Debug("Starting websocket poll loop")
	var err error
	if cs.conn == nil {
		return errors.New("nil connection")
	}
	for {
		messageType, message, cerr := cs.conn.ReadMessage()
		err = cerr
		if err != nil {
			if message != nil {
				log.Error("Error getting message from acs", "err", err, "message", message)
			} else {
				log.Error("Error getting message from acs", "err", err)
			}
			break
		}
		if messageType != websocket.TextMessage {
			log.Error("Unexpected messageType", "type", messageType)
			// maybe not fatal though, we'll try to process it anyways
		}
		log.Debug("Got a message from acs websocket", "message", string(message[:]))
		cs.handleMessage(message)
	}
	return err
}

// AddRequestHandler adds a request handler to this client.
// A request handler *must* be a function taking a single argument, and that
// argument *must* be a pointer to a recognized 'ecsacs' struct.
// E.g. if you desired to handle messages from acs of type 'FooMessage', you
// would pass the following handler in:
//     func(message *ecsacs.FooMessage)
// This function will panic if the passed in function does not have one pointer
// argument or the argument is not a recognized type.
func (cs *clientServer) AddRequestHandler(f RequestHandler) {
	firstArg := reflect.TypeOf(f).In(0)
	firstArgTypeStr := firstArg.Elem().Name()
	_, ok := typeMappings[firstArgTypeStr]
	if !ok {
		panic("AddRequestHandler called with invalid function; argument type not recognized: " + firstArgTypeStr)
	}
	cs.requestHandlers[firstArgTypeStr] = f
}

// Close closes the underlying connection
func (cs *clientServer) Close() error {
	return cs.conn.Close()
}

// MakeRequest makes a request using the given input. Note, the input *MUST* be
// a pointer to a valid ecsacs type that this client recognises
func (cs *clientServer) MakeRequest(input interface{}) error {
	msg := &requestMessage{}

	for typeStr, typeVal := range typeMappings {
		if reflect.TypeOf(input) == reflect.PtrTo(typeVal) {
			msg.Type = typeStr
			break
		}
	}
	if msg.Type == "" {
		return &UnrecognizedACSRequestType{reflect.TypeOf(input).String()}
	}
	messageData, err := jsonutil.BuildJSON(input)
	if err != nil {
		return &NotMarshallableACSRequest{msg.Type, err}
	}
	msg.Message = json.RawMessage(messageData)

	send, err := json.Marshal(msg)
	if err != nil {
		return &NotMarshallableACSRequest{msg.Type, err}
	}
	// Over the wire we send something like
	// {"type":"AckRequest","message":{"messageId":"xyz"}}
	return cs.conn.WriteMessage(websocket.TextMessage, send)
}

// decodeData decodes a raw ACS message into its type. E.g. a message of the
// form {"type":"FooMessage","message":{"foo":1}} will be decoded into the
// corresponding *ecsacs.FooMessage type. The type string, "FooMessage", will
// also be returned as a convenience.
func decodeData(data []byte) (interface{}, string, error) {
	raw := &receivedMessage{}
	// Delay unmarshal until we know the type
	err := json.Unmarshal(data, raw)
	if err != nil || raw.Type == "" {
		// Unframed messages can be of the {"Type":"Message"} form as well, try
		// that.
		connErr, connErrType, decodeErr := decodeConnectionError(data)
		if decodeErr == nil && connErrType != "" {
			return connErr, connErrType, nil
		}
		return nil, "", decodeErr
	}

	reqMessage, ok := NewOfType(raw.Type)
	if !ok {
		return nil, raw.Type, &UnrecognizedACSRequestType{raw.Type}
	}
	err = jsonutil.UnmarshalJSON(reqMessage, bytes.NewReader(raw.Message))
	return reqMessage, raw.Type, err
}

// decodeConnectionError decodes some of the connection errors returned by ACS.
// Some differ from the usual ones in that they do not have a 'type' and
// 'message' field, but rather are of the form {"ErrorType":"ErrorMessage"}
func decodeConnectionError(data []byte) (interface{}, string, error) {
	var acsErr map[string]string
	err := json.Unmarshal(data, &acsErr)
	if err != nil {
		return nil, "", &UndecodableMessage{string(data)}
	}
	if len(acsErr) != 1 {
		return nil, "", &UndecodableMessage{string(data)}
	}
	var typeStr string
	for key := range acsErr {
		typeStr = key
	}
	errType, ok := NewOfType(typeStr)
	if !ok {
		return nil, typeStr, &UnrecognizedACSRequestType{}
	}

	val := reflect.ValueOf(errType)
	if val.Kind() != reflect.Ptr {
		return nil, typeStr, &UnrecognizedACSRequestType{"Non-pointer kind: " + val.Kind().String()}
	}
	ret := reflect.New(val.Elem().Type())
	retObj := ret.Elem()

	if retObj.Kind() != reflect.Struct {
		return nil, typeStr, &UnrecognizedACSRequestType{"Pointer to non-struct kind: " + retObj.Kind().String()}
	}

	msgField := retObj.FieldByName("Message")
	if !msgField.IsValid() {
		return nil, typeStr, &UnrecognizedACSRequestType{"Expected error type to have 'Message' field"}
	}
	if msgField.IsValid() && msgField.CanSet() {
		msgStr := acsErr[typeStr]
		msgStrVal := reflect.ValueOf(&msgStr)
		if !msgStrVal.Type().AssignableTo(msgField.Type()) {
			return nil, typeStr, &UnrecognizedACSRequestType{"Type mismatch; 'Message' field must be a *string"}
		}
		msgField.Set(msgStrVal)
		return ret.Interface(), typeStr, nil
	}
	return nil, typeStr, &UnrecognizedACSRequestType{"Invalid message field; must not be nil"}
}

// handleMessage dispatches a message to the correct 'requestHandler' for its
// type. If no request handler is found, the message is discarded.
func (cs *clientServer) handleMessage(data []byte) {
	typedMessage, typeStr, err := decodeData(data)
	if err != nil {
		log.Warn("Unable to handle message from acs", "err", err)
		return
	}

	if handler, ok := cs.requestHandlers[typeStr]; ok {
		go reflect.ValueOf(handler).Call([]reflect.Value{reflect.ValueOf(typedMessage)})
	} else {
		log.Info("No handler for message type", "type", typeStr)
	}
}
