package containermetadata

import (
	"encoding/json"
)

// PortMapping holds data about the container's port bind to the host
type PortMapping struct {
	ContainerPort string
	HostPort      string
	BindIP        string
	Protocol      string
}

// NetworkMetadata keeps track of the data we parse from the Network Settings
// in docker containers
type NetworkMetadata struct {
	ports       []PortMapping
	gateway     string
	iPAddress   string
	iPv6Gateway string
}

func (nm *NetworkMetadata) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Ports       []PortMapping
		Gateway     string
		IPAddress   string
		IPv6Gateway string
	}{
		Ports:       nm.ports,
		Gateway:     nm.gateway,
		IPAddress:   nm.iPAddress,
		IPv6Gateway: nm.iPv6Gateway,
	})
}

// DockerMetadata keeps track of all metadata acquired from Docker inspection
// Has redundancies with engine.DockerContainerMetadata but packages all
// docker metadata we want in the service so we can change features easily
type DockerMetadata struct {
	status        string
	containerID   string
	containerName string
	imageID       string
	imageName     string
	networkInfo   *NetworkMetadata
}

// AWSMetadata keeps track of all metadata acquired from AWS
type AWSMetadata struct {
	clusterArn string `json:"ClusterArn, omitempty"`
	taskArn    string `json:"TaskArn, omitempty"`
}

// Metadata packages all acquired metadata and is used to format it
// into JSON to write to the metadata file
type Metadata struct {
	status        string
	containerID   string
	containerName string
	imageID       string
	imageName     string
	clusterArn    string
	taskArn       string
	network       *NetworkMetadata
}

func (m *Metadata) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Status        string           `json:"Status, omitempty"`
		ContainerID   string           `json:"ContainerID, omitempty"`
		ContainerName string           `json:"ContainerName, omitempty"`
		ImageID       string           `json:"ImageID, omitempty"`
		ImageName     string           `json:"ImageName, omitempty"`
		ClusterArn    string           `json:"ClusterArn, omitempty"`
		TaskArn       string           `json:"TaskArn, omitempty"`
		Network       *NetworkMetadata `json:"Network, omitempty"`
	}{
		Status:        m.status,
		ContainerID:   m.containerID,
		ContainerName: m.containerName,
		ImageID:       m.imageID,
		ImageName:     m.imageName,
		ClusterArn:    m.clusterArn,
		TaskArn:       m.taskArn,
		Network:       m.network,
	})
}
