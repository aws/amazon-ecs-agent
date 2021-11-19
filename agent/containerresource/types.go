package containerresource

import (
	"time"

	"github.com/aws/amazon-ecs-agent/agent/containerresource/containerstatus"
)

// Secret contains all essential attributes needed for ECS secrets vending as environment variables/tmpfs files
type Secret struct {
	Name          string `json:"name"`
	ValueFrom     string `json:"valueFrom"`
	Region        string `json:"region"`
	ContainerPath string `json:"containerPath"`
	Type          string `json:"type"`
	Provider      string `json:"provider"`
	Target        string `json:"target"`
}

// GetSecretResourceCacheKey returns the key required to access the secret
// from the ssmsecret resource
func (s *Secret) GetSecretResourceCacheKey() string {
	return s.ValueFrom + "_" + s.Region
}

type EnvironmentFile struct {
	Value string `json:"value"`
	Type  string `json:"type"`
}

// FirelensConfig describes the type and options of a Firelens container.
type FirelensConfig struct {
	Type                       string            `json:"type"`
	Version                    string            `json:"version"`
	CollectStdoutLogs          bool              `json:"collectStdoutLogs,omitempty"`
	StatusMessageReportingPath string            `json:"statusMessageReportingPath,omitempty"`
	FirelensConfigurationMode  string            `json:"firelensConfigurationMode,omitempty"`
	Options                    map[string]string `json:"options"`
}

// HealthStatus contains the health check result returned by docker
type HealthStatus struct {
	// Status is the container health status
	Status containerstatus.ContainerHealthStatus `json:"status,omitempty"`
	// Since is the timestamp when container health status changed
	Since *time.Time `json:"statusSince,omitempty"`
	// ExitCode is the exitcode of health check if failed
	ExitCode int `json:"exitCode,omitempty"`
	// Output is the output of health check
	Output string `json:"output,omitempty"`
}
