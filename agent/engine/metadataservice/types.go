package metadataservice

//Has redundancies with engine.DockerContainerMetadata but packages all
//docker metadata we want in the service so we can change features easily 
type DockerMetadata struct {
	Status string
	ContainerID string
	ContainerName string
	ImageName string
	ImageID string
	NetworkInfo *ContainerNetworkMetadata
}

//TODO	ContainerInstanceArn string Need to figure out how to get this
type AWSMetadata struct {
	ClusterArn string
	TaskArn string
}

//NetworkMode string TODO Not sure what NetworkMode means in documentation
type ContainerNetworkMetadata struct {
	Ports map[string][]PortMapping
	Gateway string
	IPAddress string
	IPv6Gateway string
}

type PortMapping struct {
	ContainerPort string
	HostPort string
	BindIP string
	Protocol string
}
