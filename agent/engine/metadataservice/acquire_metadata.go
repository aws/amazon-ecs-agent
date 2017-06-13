package metadataservice

import (
	"fmt"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	docker "github.com/fsouza/go-dockerclient"
)

//At container creation we can get most of docker metadata 
func AcquireDockerMetadata(container *docker.Container) *DockerMetadata {
	if container == nil {
		return nil
	}
	status := getStatus(container)
	containerID := getContainerID(container)
	containerName := getContainerName(container)
	imageName := getImageName(container)
	imageID := getImageID(container)
	networkInfo := getNetworkInfo(container)

	md := &DockerMetadata { Status: status, ContainerID: containerID, ContainerName: containerName, ImageName: imageName, ImageID: imageID, NetworkInfo: networkInfo }
	return md
}

/* Getter functions to modulate metadata retrieval since data is 
   accessible at different times */
func getStatus(container *docker.Container) string {
	if container == nil {
		return ""
	}
	return container.State.String()
}

func getContainerID(container *docker.Container) string {
	if container == nil {
		return ""
	}
	return container.ID
}

func getContainerName(container *docker.Container) string {
	if container == nil {
		return ""
	}
	return container.Name
}

func getImageName(container *docker.Container) string {
	if container == nil || container.Config == nil {
		return ""
	}
	return container.Config.Image
}

func getImageID(container *docker.Container) string {
	if container == nil {
		return ""
	}
	return container.Image
}

func getNetworkInfo(container *docker.Container) *ContainerNetworkMetadata {
	if container == nil {
		return nil
	}
	return networkToMetadata(container.NetworkSettings)
} 

func networkToMetadata(ns *docker.NetworkSettings) *ContainerNetworkMetadata {
	return nil
	//TODO: return &ContainerNetworkMetadata { Ports: getPortInfo(ns.Ports), Gateway: ns.Gateway, IPAddress: ns.IPAddress, IPv6Gateway: ns.IPv6Gateway }
}

//TODO: Terrible variable names
func getPortInfo(ports map[docker.Port][]docker.PortBinding) map[string][]PortMapping {
	mapping := make(map[string][]PortMapping)
	for port, portbindings := range ports {
		bindings := make([]PortMapping, len(portbindings))
		var portn string
		var protocol string
		fmt.Sscanf(port.Port(), "%s/%s", &portn, &protocol)
		for i, pb := range portbindings {
			bindings[i] = PortMapping { ContainerPort: portn, HostPort: pb.HostPort, BindIP: pb.HostIP, Protocol: protocol }
		}
		mapping[portn] = bindings
	}
	return mapping
}

//AWS metadata is all available at container creation so we can get it all here 
func AcquireAWSMetadata(cfg *config.Config, task *api.Task, container *api.Container) *AWSMetadata {
	clusterArn := cfg.Cluster
	//TODO: Figure out how to get get containerInstanceArn
	return &AWSMetadata { ClusterArn: clusterArn, TaskArn: task.Arn }
}
