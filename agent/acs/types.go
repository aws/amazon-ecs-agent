package acs

import "github.com/aws/amazon-ecs-agent/agent/api"

type PollCallback func(api.Task)

type PollResponse struct {
	MessageType string `json:"type"`
	//TODO handle other messageTypes, this should be union type
	Message Payload `json:"message"`
}

type Payload struct {
	Tasks     []*api.Task
	MessageId string
}

type HealthResponse struct {
	Healthy bool `json:"healthy"`
}

type AckRequest struct {
	ClusterArn           string `json:"cluster"`
	ContainerInstanceArn string `json:"containerInstance"`
	MessageId            string `json:"messageId"`
}
