package metadataservice

import(
	"encoding/json"
)

type PortMapping struct {
	ContainerPort string
	HostPort string
	BindIP string
	Protocol string
}

type NetworkMetadata struct {
	ports map[string]PortMapping
	gateway string
	iPAddress string
	iPv6Gateway string
}

func (nm NetworkMetadata) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct{
		Ports map[string]PortMapping
		Gateway string
		IPAddress string
		IPv6Gateway string
	} {
		Ports:       nm.ports,
		Gateway:     nm.gateway,
		IPAddress:    nm.iPAddress,
		IPv6Gateway: nm.iPv6Gateway,
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
	networkInfo *NetworkMetadata
}

type AWSMetadata struct {
	clusterArn string `json:"ClusterArn, omitempty"`
	taskArn string `json:"TaskArn, omitempty"`
}

type Metadata struct {
	status string
	containerID string
	containerName string
	imageID string
	imageName string
	clusterArn string
	taskArn string
	network *NetworkMetadata
}

func (m Metadata) MarshalJSON() ([]byte, error) {
	var network_tmp NetworkMetadata
	if m.network == nil {
		network_tmp = NetworkMetadata{}
	} else {
		network_tmp = *m.network
	}
	return json.Marshal(struct{
		Status string `json:"Status, omitempty"`
		ContainerID string `json:"ContainerID, omitempty"`
		ContainerName string `json:"ContainerName, omitempty"`
		ImageID string `json:"ImageID, omitempty"`
		ImageName string `json:"ImageName, omitempty"`
		ClusterArn string `json:"ClusterArn, omitempty"`
		TaskArn string `json:"TaskArn, omitempty"`
		Network NetworkMetadata `json:"Network, omitempty"`
	} {
		Status:        m.status,
		ContainerID:   m.containerID,
		ContainerName: m.containerName,
		ImageID:       m.imageID,
		ImageName:     m.imageName,
		ClusterArn:    m.clusterArn,
		TaskArn:       m.taskArn,
		Network:       network_tmp,
	})
}

