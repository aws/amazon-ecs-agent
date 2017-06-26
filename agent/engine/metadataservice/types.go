package metadataservice

import(
	"encoding/json"
)

type Metadata struct {
	status string `json:"Status, omitempty"`
	containerID string `json:"ContainerID, omitempty"`
	containerName string `json:"ContainerName, omitempty"`
	imageID string `json:"ImageID, omitempty"`
	imageName string `json:ImageName, omitempty"`
	clusterArn string `json:"ClusterArn, omitempty"`
	taskArn string `json:"TaskArn, omitempty"`
}

func (m Metadata) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Status string `json:"Status, omitempty"`
		ContainerID string `json:"ContainerID, omitempty"`
		ContainerName string `json:"ContainerName, omitempty"`
		ImageID string `json:"ImageID, omitempty"`
		ImageName string `json:"ImageName, omitempty"`
		ClusterArn string `json:"ClusterArn, omitempty"`
		TaskArn string `json:"TaskArn, omitempty"`
	} {
		Status:        m.status,
		ContainerID:   m.containerID,
		ContainerName: m.containerName,
		ImageID:       m.imageID,
		ImageName:     m.imageName,
		ClusterArn:    m.clusterArn,
		TaskArn:       m.taskArn,
	})
}

//Has redundancies with engine.DockerContainerMetadata but packages all
//docker metadata we want in the service so we can change features easily 
type DockerMetadata struct {
	status string `json:"Status, omitempty"`
	containerID string `json:"ContainerID, omitempty"`
	containerName string `json:"ContainerName, omitempty"`
	imageID string `json:"ImageID, omitempty"`
	imageName string `json:ImageName, omitempty"`
	//networkInfo *ContainerNetworkMetadata
}

/* UNUSED
func (m *Metadata) NetworkInfo() *ContainerNetworkMetadata {
	return m.networkInfo
} */

type AWSMetadata struct {
	clusterArn string `json:"ClusterArn, omitempty"`
	taskArn string `json:"TaskArn, omitempty"`
}

/* Currently unused 
type ContainerNetworkMetadata struct {
	ports map[string][]PortMapping
	gateway string
	iPAddress string
	iPv6Gateway string
}

type PortMapping struct {
	ContainerPort string
	HostPort string
	BindIP string
	Protocol string
} */
