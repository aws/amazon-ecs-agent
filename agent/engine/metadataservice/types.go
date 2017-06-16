package metadataservice

//
type Metadata struct {
	dockerMetadata *DockerMetadata
	awsMetadata    *AWSMetadata
}

func (m *Metadata) DockerMetadata() *DockerMetadata {
	return m.dockerMetadata
}

func (m *Metadata) AWSMetadata() *AWSMetadata {
	return m.awsMetadata
}

//Has redundancies with engine.DockerContainerMetadata but packages all
//docker metadata we want in the service so we can change features easily 
type DockerMetadata struct {
	status string
	containerID string
	containerName string
	imageID string
	imageName string
	//networkInfo *ContainerNetworkMetadata
}

func (dm *DockerMetadata) Status() string {
	return dm.status
}

func (dm *DockerMetadata) ContainerID() string {
	return dm.containerID
}

func (dm *DockerMetadata) ContainerName() string {
	return dm.containerName
}

func (dm *DockerMetadata) ImageID() string {
	return dm.imageID
}

func (dm *DockerMetadata) ImageName() string {
	return dm.imageName
}

/* UNUSED
func (dm *DockerMetadata) NetworkInfo() *ContainerNetworkMetadata {
	return dm.networkInfo
} */

//Probably unneeded since all our metadata operations occur in docker_task_engine.go where 
//Task information is directly available
type AWSMetadata struct {
	clusterArn string
	taskArn string
}

func (awsm *AWSMetadata) ClusterArn() string {
	return awsm.clusterArn
}

func (awsm *AWSMetadata) TaskArn() string {
	return awsm.taskArn
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
