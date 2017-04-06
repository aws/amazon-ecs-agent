// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package api

import "errors"

const (
	// TransportProtocolTCP represents TCP
	TransportProtocolTCP TransportProtocol = iota
	// TransportProtocolUDP represents UDP
	TransportProtocolUDP

	tcp = "tcp"
	udp = "udp"
)

// TransportProtocol is an enumeration of valid transport protocols
type TransportProtocol int32

// NewTransportProtocol returns a TransportProtocol from a string in the task
func NewTransportProtocol(protocol string) (TransportProtocol, error) {
	switch protocol {
	case tcp:
		return TransportProtocolTCP, nil
	case udp:
		return TransportProtocolUDP, nil
	default:
		return TransportProtocolTCP, errors.New(protocol + " is not a recognized transport protocol")
	}
}

func (tp *TransportProtocol) String() string {
	if tp == nil {
		return tcp
	}
	switch *tp {
	case TransportProtocolUDP:
		return udp
	case TransportProtocolTCP:
		return tcp
	default:
		log.Crit("Unknown TransportProtocol type!")
		return tcp
	}
}
