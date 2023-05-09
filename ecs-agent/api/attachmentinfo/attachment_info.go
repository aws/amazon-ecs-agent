package attachmentinfo

import (
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/api/status"
)

type AttachmentInfo struct {
	// TaskARN is the task identifier from ecs
	TaskARN string `json:"taskArn"`
	// AttachmentARN is the identifier for the attachment
	AttachmentARN string `json:"attachmentArn"`
	// Status is the status of the attachment: none/attached/detached
	Status status.AttachmentStatus `json:"status"`
	// ExpiresAt is the timestamp past which the attachment is considered
	// unsuccessful. The SubmitTaskStateChange API, with the attachment information
	// should be invoked before this timestamp.
	ExpiresAt time.Time `json:"expiresAt"`
	// AttachStatusSent indicates whether the attached status has been sent to backend
	AttachStatusSent bool `json:"attachSent,omitempty"`
	// TaskClusterARN is the identifier for the cluster which the task resides in
	TaskClusterARN string `json:"taskClusterArn,omitempty"`
	// ClusterARN is the identifier for the cluster which the container instance is registered to
	ClusterARN string `json:"clusterArn,omitempty"`
	// ContainerInstanceARN is the identifier for the container instance
	ContainerInstanceARN string `json:"containerInstanceArn,omitempty"`
}
