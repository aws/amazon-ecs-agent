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

package session

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
)

// ResponseToACSSender returns a wsclient.RespondFunc that a responder can invoke in response to receiving and
// processing specific websocket request messages from ACS. The returned wsclient.RespondFunc:
//  1. logs the response to be sent, as well as the name of the invoking responder
//  2. sends the response request to ACS
func ResponseToACSSender(responderName string, responseSender wsclient.RespondFunc) wsclient.RespondFunc {
	return func(response interface{}) error {
		logger.Debug("Sending response to ACS", logger.Fields{
			"Name":     responderName,
			"Response": response,
		})
		return responseSender(response)
	}
}
