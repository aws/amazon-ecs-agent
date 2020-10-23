package execcmd

import (
	"fmt"
)

// AgentMetadata holds metadata about the exec agent running inside the container (i.e. SSM Agent).
type AgentMetadata struct {
	PID          string `json:"PID"`
	DockerExecID string `json:"DockerExecID"`
	CMD          string `json:"CMD"`
}

func (md *AgentMetadata) String() string {
	return fmt.Sprintf("[PID: %s, DockerExecId: %s, CMD: %s]", md.PID, md.DockerExecID, md.CMD)
}

func (md *AgentMetadata) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"PID":          md.PID,
		"DockerExecID": md.DockerExecID,
		"CMD":          md.CMD,
	}
}

func MapToAgentMetadata(md map[string]interface{}) AgentMetadata {
	var execMD AgentMetadata
	if md == nil {
		return execMD
	}
	execMD.PID, _ = md["PID"].(string)
	execMD.DockerExecID, _ = md["DockerExecID"].(string)
	execMD.CMD, _ = md["CMD"].(string)
	return execMD
}
