package containermetadata

import (
	"encoding/json"
)

// PortMapping holds data about the container's port bind to the host
type PortMapping struct {
	ContainerPort string `json:"ContainerPort, omitempty"`
	HostPort      string `json:"HostPort, omitempty"`
	BindIP        string `json:"BindIP, omitempty"`
	Protocol      string `json:"Protocol, omitempty"`
}

// NetworkMetadata keeps track of the data we parse from the Network Settings
// in docker containers
type NetworkMetadata struct {
	ports       []PortMapping
	networkMode string
	gateway     string
	iPAddress   string
	iPv6Gateway string
}

/*
func (nm *NetworkMetadata) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Ports       []PortMapping `json:"PortMappings, omitempty"`
		NetworkMode string        `json:"NetworkMode, omitempty"`
		Gateway     string        `json:"Gateway, omitempty"`
		IPAddress   string        `json:"IPAdress, omitempty"`
		IPv6Gateway string        `json:"IPv6Gateway, omitempty"`
	}{
		Ports:       nm.ports,
		NetworkMode: nm.networkMode,
		Gateway:     nm.gateway,
		IPAddress:   nm.iPAddress,
		IPv6Gateway: nm.iPv6Gateway,
	})
}
*/

// DockerContainerMetadata keeps track of all metadata acquired from Docker inspection
// Has redundancies with engine.DockerContainerMetadata but packages all
// docker metadata we want in the service so we can change features easily
type DockerContainerMetadata struct {
	status        string
	containerID   string
	containerName string
	imageID       string
	imageName     string
	networkInfo   *NetworkMetadata
}

// TaskMetadata keeps track of all metadata associated with a task
// provided by AWS, does not depend on the creation of the container
type TaskMetadata struct {
	version           string
	clusterArn        string
	containerInstance string
	taskArn           string
}

// Metadata packages all acquired metadata and is used to format it
// into JSON to write to the metadata file
type Metadata struct {
	version           string
	status            string
	clusterArn        string
	containerInstance string
	taskArn           string
	imageName         string
	imageID           string
	containerName     string
	containerID       string
	network           *NetworkMetadata
}

func (m *Metadata) MarshalJSON() ([]byte, error) {
	var ports []PortMapping
	var networkMode, gateway, iPAddress, iPv6Gateway string
	if m.network != nil {
		ports = m.network.ports
		networkMode = m.network.networkMode
		gateway = m.network.gateway
		iPAddress = m.network.iPAddress
		iPv6Gateway = m.network.iPv6Gateway
	}
	return json.Marshal(struct {
		Version           string        `json:"DockerVersion, omitempty"`
		Status            string        `json:"Status, omitempty"`
		ClusterArn        string        `json:"ClusterArn, omitempty"`
		ContainerInstance string        `json:"ContainerInstanceArn, omitempty"`
		TaskArn           string        `json:"TaskArn, omitempty"`
		ContainerID       string        `json:"ContainerID, omitempty"`
		ContainerName     string        `json:"ContainerName, omitempty"`
		ImageID           string        `json:"ImageID, omitempty"`
		ImageName         string        `json:"ImageName, omitempty"`
		Ports             []PortMapping `json:"PortMappings, omitempty"`
		NetworkMode       string        `json:"NetworkMode, omitempty"`
		Gateway           string        `json:"Gateway, omitempty"`
		IPAddress         string        `json:"IPAddress, omitempty"`
		IPv6Gateway       string        `json:"IPv6Gateway, omitempty"`
		//		Network           *NetworkMetadata `json:"Network, omitempty"`
	}{
		Version:           m.version,
		Status:            m.status,
		ClusterArn:        m.clusterArn,
		ContainerInstance: m.containerInstance,
		TaskArn:           m.taskArn,
		ImageName:         m.imageName,
		ImageID:           m.imageID,
		ContainerName:     m.containerName,
		ContainerID:       m.containerID,
		Ports:             ports,
		NetworkMode:       networkMode,
		Gateway:           gateway,
		IPAddress:         iPAddress,
		IPv6Gateway:       iPv6Gateway,
		//		Network:       m.network,
	})
}
