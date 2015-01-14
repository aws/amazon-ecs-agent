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

package client

import (
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/client/dialer"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/codec/codec"
	"io"
)

const DEFAULT_POOL_SIZE = 5

type Client interface {
	Call(operation string, input interface{}, output interface{}) error
}

type OperationRef struct {
	Name string
}

type ServiceRef struct {
	ServiceName string
}

type client struct {
	serviceRef codec.ShapeRef
	connPool   chan io.ReadWriter
	dialer     dialer.Dialer
	codec      codec.Codec
}

func NewClient(serviceName string, dialer dialer.Dialer, c codec.Codec) Client {
	return &client{
		codec.ShapeRef{serviceName},
		make(chan io.ReadWriter, DEFAULT_POOL_SIZE),
		dialer,
		c,
	}
}

//consume a connection from the pool
//if no connection is available, a new one is created from the dialer
func (c *client) consumeConn() (io.ReadWriter, error) {
	var conn io.ReadWriter
	var err error = nil
	select {
	case conn = <-c.connPool:
	default:
		conn, err = c.dialer.Dial()
	}
	return conn, err
}

//return a conn to the pool
//if the conn pool is full and the ReadWriter is able to be closed, it closes it
func (c *client) returnConn(rw io.ReadWriter) {
	select {
	case c.connPool <- rw:
	default:
		//Pool of connections is full, close it if we can and move on
		if closer, ok := rw.(io.Closer); ok {
			closer.Close()
		}
	}
}

func (c *client) Call(operation string, in interface{}, out interface{}) error {
	conn, err := c.consumeConn()
	if err != nil {
		return err
	}
	defer c.returnConn(conn)
	req := &codec.Request{
		Service:   c.serviceRef,
		Operation: codec.ShapeRef{operation},
		Input:     in,
		Output:    out,
	}
	err = c.codec.RoundTrip(req, conn)
	return err
}
