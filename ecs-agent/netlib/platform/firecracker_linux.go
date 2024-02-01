// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package platform

import (
	"context"
	"fmt"

	netlibdata "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/data"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/serviceconnect"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
)

type firecraker struct {
	common
}

func (f *firecraker) BuildTaskNetworkConfiguration(
	taskID string,
	taskPayload *ecsacs.Task) (*tasknetworkconfig.TaskNetworkConfig, error) {

	// On Firecracker, there is always only one task network namespace on the bare metal host.
	// Inside the microVM, a dedicated netns will also be created to separate primary interface
	// and secondary interface(s) of the task. The following method invocation inspects the
	// container-to-interface mapping to decide which interface resides in which namespace inside
	// the microVM.
	i2n, err := assignInterfacesToNamespaces(taskPayload)
	if err != nil {
		return nil, err
	}

	return f.common.buildTaskNetworkConfiguration(taskID, taskPayload, true, i2n)
}

func (f *firecraker) CreateDNSConfig(taskID string, netNS *tasknetworkconfig.NetworkNamespace) error {
	err := f.common.createDNSConfig(taskID, false, netNS)
	if err != nil {
		return err
	}

	return f.configureSecondaryDNSConfig(taskID, netNS)
}

func (f *firecraker) ConfigureInterface(
	ctx context.Context,
	netNSPath string,
	iface *networkinterface.NetworkInterface,
	netDAO netlibdata.NetworkDataClient,
) error {
	return f.common.configureInterface(ctx, netNSPath, iface, netDAO)
}

func (f *firecraker) ConfigureAppMesh(ctx context.Context, netNSPath string, cfg *appmesh.AppMesh) error {
	return errors.New("not implemented")
}

func (f *firecraker) ConfigureServiceConnect(
	ctx context.Context,
	netNSPath string,
	primaryIf *networkinterface.NetworkInterface,
	scConfig *serviceconnect.ServiceConnectConfig,
) error {
	return errors.New("not implemented")
}

// configureSecondaryDNSConfig creates DNS config files for secondary interfaces. This is required because
// on FoF, secondary interfaces reside in their own network namespace inside the microVM. The DNS config
// inside the namespace will need to be the secondary interface DNS config.
func (f *firecraker) configureSecondaryDNSConfig(taskID string, netNS *tasknetworkconfig.NetworkNamespace) error {
	for _, iface := range netNS.NetworkInterfaces {
		// Omit primary interface and veth interfaces.
		if iface.IsPrimary() || iface.VETHProperties != nil {
			continue
		}

		// Create DNS files.
		dnsDirName := networkinterface.NetNSName(taskID, iface.Name)
		err := f.common.createNetworkConfigFiles(dnsDirName, iface)
		if err != nil {
			return errors.Wrapf(err, "failed to create DNS config for interface %s", iface.Name)
		}

		// Copy to task volume.
		err = f.common.copyNetworkConfigFilesToTask(taskID, dnsDirName)
		if err != nil {
			return errors.Wrapf(err, "failed to create DNS config for interface %s", iface.Name)
		}
	}

	return nil
}

// assignInterfacesToNamespaces computes how many network namespaces the task needs and assigns
// each network interface to a network namespace.
func assignInterfacesToNamespaces(taskPayload *ecsacs.Task) (map[string]string, error) {
	// The task payload has a list of containers, a list of network interface names, and a list of
	// which interface(s) each container should have access to. For this schema to work, the set of
	// interface(s) used by one or more containers need to be grouped into network namespaces. Then
	// the container runtime needs to be told to launch each container in its designated network
	// namespace. This function computes how many network namespaces are needed, and then returns a
	// map of network interface names to network namespace names.
	i2n := make(map[string]string)

	// Optimization for the common case: If the task has a single interface, there is nothing to do.
	if len(taskPayload.ElasticNetworkInterfaces) == 1 {
		return i2n, nil
	}

	for _, c := range taskPayload.Containers {
		// containerNetNS keeps track of the netns assigned to this container.
		containerNetNS := ""

		for _, i := range c.NetworkInterfaceNames {
			ifName := aws.StringValue(i)

			netnsName, ok := i2n[ifName]
			if !ok {
				// This interface was not assigned to a netns yet.
				// Create a new netns for this container if it doesn't have one.
				if containerNetNS == "" {
					// Use the container's first interface's name as the netns name.
					// This naming isn't strictly necessary, just convenient when debugging.
					containerNetNS = ifName
				}
				// Assign the interface to this container's netns.
				i2n[ifName] = containerNetNS
			} else {
				// This interface was already assigned to a netns in a previous iteration.
				// Assign the interface's netns to this container.
				if containerNetNS == "" {
					containerNetNS = netnsName
				}
				// All interfaces for a given container must be in the same netns.
				if netnsName != containerNetNS {
					return nil, fmt.Errorf("invalid task netns config")
				}
			}
		}
	}

	// The logic above names each netns after the first network interface placed in it. However the
	// first (primary) netns should always be named "" so that it maps to the default netns.
	for _, e := range taskPayload.ElasticNetworkInterfaces {
		if *e.Index == int64(0) {
			for ifName, netNSName := range i2n {
				if netNSName == aws.StringValue(e.Name) {
					i2n[ifName] = ""
				}
			}
			break
		}
	}

	return i2n, nil
}
