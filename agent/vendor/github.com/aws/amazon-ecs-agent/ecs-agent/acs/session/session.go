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
