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

package codec

import (
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/awsjson/encoding"
	cod "github.com/aws/amazon-ecs-agent/agent/ecs_client/codec/codec"
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	authv4 "github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4"
)

type BasicAuth struct {
	Username, Password string
}

type AwsJson struct {
	Host          string            // required, host or host:port
	Path          string            // optional
	SignerV4      authv4.HttpSigner // optional
	AuthInfo      *BasicAuth        // optional
	SecurityToken string            // optional
}

type Option func(*AwsJson)

/*
   Example: AwsJson := codec.NewAwsJson("foo.com:8080", codec.SetSignerV4(signer))
*/
func NewAwsJson(host string, options ...Option) AwsJson {

	AwsJson := AwsJson{Host: host}

	for _, option := range options {
		option(&AwsJson)
	}

	return AwsJson
}

func SetPath(path string) Option {
	return (Option)(func(AwsJson *AwsJson) {
		AwsJson.Path = path
	})
}

func SetSignerV4(signer authv4.HttpSigner) Option {
	return (Option)(func(AwsJson *AwsJson) {
		AwsJson.SignerV4 = signer
	})
}

func SetBasicAuth(ba *BasicAuth) Option {
	return (Option)(func(AwsJson *AwsJson) {
		AwsJson.AuthInfo = ba
	})
}

func SetSecurityToken(securityToken string) Option {
	return (Option)(func(AwsJson *AwsJson) {
		AwsJson.SecurityToken = securityToken
	})
}

func (c AwsJson) RoundTrip(r *cod.Request, rw io.ReadWriter) error {
	path := c.Path
	if path == "" {
		path = "/"
	}

	b, err := encoding.Marshal(r.Input)
	if err != nil {
		return err
	}
	body := bytes.NewBuffer(b)
	request, err := http.NewRequest("POST", path, body)
	if err != nil {
		//TODO: Panic here? Suggests malformed input to NewRequest
		return err
	}

	if c.Host != "" {
		request.Host = c.Host
		request.URL.Host = c.Host
	}

	request.Header.Set("Accept", "application/json, text/javascript")
	request.Header.Set("Content-Type", "application/x-amz-json-1.1")
	request.Header.Set("X-Amz-Target", r.Service.ShapeName+"."+r.Operation.ShapeName)

	if c.SecurityToken != "" {
		request.Header.Set("X-Amz-Security-Token", c.SecurityToken)
	}

	if c.AuthInfo != nil {
		request.SetBasicAuth(c.AuthInfo.Username, c.AuthInfo.Password)
	}

	// v4 signing?
	if c.SignerV4 != nil {
		c.SignerV4.SignHttpRequest(request)
	} else { // v4 signing will add this header
		request.Header.Set("X-Amz-Date", time.Now().UTC().Format(time.RFC822))
	}

	err = request.Write(rw)
	if err != nil {
		return err
	}
	resp, err := http.ReadResponse(bufio.NewReader(rw), request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return errors.New(string(respBody))
	}

	return encoding.Unmarshal(respBody, r.Output)
}
