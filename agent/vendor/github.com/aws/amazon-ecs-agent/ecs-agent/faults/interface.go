package faults

import(
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/fault/v1/types"
	state "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"
)

type FaultInjection interface{
	CheckFaultStatus(request types.NetworkFaultRequest, taskMetadata *state.TaskResponse) (string, error)
}