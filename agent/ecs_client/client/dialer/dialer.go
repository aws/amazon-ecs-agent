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

package dialer

import (
	"fmt"
	"io"
	"net"

	"crypto/tls"
)

type Dialer interface {
	//Dial creates a new connection to the service
	Dial() (io.ReadWriter, error)
}

type DialerFunc func() (io.ReadWriter, error)

func (f DialerFunc) Dial() (io.ReadWriter, error) {
	return f()
}

//TCP creates a Dialer to the address and port using a TCP connection
func TCP(addr string, port uint16) (Dialer, error) {
	address := fmt.Sprintf("%s:%d", addr, port)
	resolved, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return nil, err
	}
	return DialerFunc(func() (io.ReadWriter, error) {
		return net.DialTCP("tcp", nil, resolved)
	}), nil
}

//TLS creates a Dialer to the address and port using a TLS (SSL) connection
func TLS(addr string, port uint16, config *tls.Config) (Dialer, error) {
	address := fmt.Sprintf("%s:%d", addr, port)
	return DialerFunc(func() (io.ReadWriter, error) {
		return tls.Dial("tcp", address, config)
	}), nil
}
