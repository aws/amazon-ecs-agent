package metadataservice

//
type Metadata struct {
	Status string `json:"Status, omitempty"`
	ContainerID string `json:"ContainerID, omitempty"`
	ContainerName string `json:"ContainerName, omitempty"`
	ImageID string `json:"ImageID, omitempty"`
	ImageName string `json:ImageName, omitempty"`
	ClusterArn string `json:"ClusterArn, omitempty"`
	TaskArn string `json:"TaskArn, omitempty"`
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

//Probably unneeded since all our metadata operations occur in docker_task_engine.go where 
//Task information is directly available
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
