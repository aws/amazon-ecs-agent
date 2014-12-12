package api

import (
	"errors"
	"strconv"

	"github.com/fsouza/go-dockerclient"
)

// PortBindingFromDockerPortBinding constructs a PortBinding slice from a docker
// NetworkSettings.Ports map.
// It prefers to fail-hard rather than report incorrect information, so any
// issues in docker's data will result in an error
func PortBindingFromDockerPortBinding(dockerPortBindings map[docker.Port][]docker.PortBinding) ([]PortBinding, error) {
	portBindings := make([]PortBinding, 0, len(dockerPortBindings))

	for port, bindings := range dockerPortBindings {
		intPort, err := strconv.Atoi(port.Port())
		if err != nil {
			return nil, err
		}
		containerPort := intPort
		if len(bindings) == 0 {
			return nil, errors.New("Zero length port-bindings are not supported.")
		}
		for _, binding := range bindings {
			hostPort, err := strconv.Atoi(binding.HostPort)
			if err != nil {
				return nil, err
			}
			portBindings = append(portBindings, PortBinding{
				ContainerPort: uint16(containerPort),
				HostPort:      uint16(hostPort),
				BindIp:        binding.HostIP,
			})
		}
	}
	return portBindings, nil
}
