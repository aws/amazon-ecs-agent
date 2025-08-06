//go:build !windows
// +build !windows

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
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	netlibdata "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/data"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/serviceconnect"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/ioutilwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/volume"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	cnitypes "github.com/containernetworking/cni/pkg/types/100"
	cnins "github.com/containernetworking/plugins/pkg/ns"
	"github.com/pkg/errors"
)

const (
	networkConfigFileDirectory    = "/etc/netns"
	networkConfigHostnameFilePath = "/etc/hostname"
	networkConfigFileMode         = 0644
	taskDNSConfigFileMode         = 0666
	HostsLocalhostEntryIPv4       = "127.0.0.1 localhost"
	HostsLocalhostEntryIPv6       = "::1 localhost"

	// DNS related configuration.
	HostnameFileName    = "hostname"
	ResolveConfFileName = "resolv.conf"
	HostsFileName       = "hosts"

	// indexHighValue is a placeholder value used while finding
	// interface with lowest index in from the ACS payload.
	// It is assigned 100 because it is an unrealistically high
	// value for interface index.
	indexHighValue = 100
)

// common will be embedded within every implementation of the platform API.
// It contains all fields and methods that can be commonly used by all
// platforms.
type common struct {
	nsUtil            ecscni.NetNSUtil
	dnsVolumeAccessor volume.TaskVolumeAccessor
	os                oswrapper.OS
	ioutil            ioutilwrapper.IOUtil
	netlink           netlinkwrapper.NetLink
	stateDBDir        string
	cniClient         ecscni.CNI
	net               netwrapper.Net
	resolvConfPath    string
}

// NewPlatform creates an implementation of the platform API depending on the
// platform type where the agent is executing.
func NewPlatform(
	platformConfig Config,
	volumeAccessor volume.TaskVolumeAccessor,
	stateDBDirectory string,
	netWrapper netwrapper.Net,
) (API, error) {
	commonPlatform := common{
		nsUtil:            ecscni.NewNetNSUtil(),
		dnsVolumeAccessor: volumeAccessor,
		os:                oswrapper.NewOS(),
		ioutil:            ioutilwrapper.NewIOUtil(),
		netlink:           netlinkwrapper.New(),
		stateDBDir:        stateDBDirectory,
		cniClient:         ecscni.NewCNIClient([]string{CNIPluginPathDefault}),
		net:               netWrapper,
		resolvConfPath:    platformConfig.ResolvConfPath,
	}

	switch platformConfig.Name {
	case WarmpoolPlatform:
		return &containerd{
			common: commonPlatform,
		}, nil
	case FirecrackerPlatform:
		return &firecraker{
			common: commonPlatform,
		}, nil
	case WarmpoolDebugPlatform:
		return &containerdDebug{
			containerd: containerd{
				common: commonPlatform,
			},
		}, nil
	case FirecrackerDebugPlatform:
		return &firecrackerDebug{
			firecraker: firecraker{
				common: commonPlatform,
			},
		}, nil
	case ManagedPlatform, ManagedDebugPlatform:
		ec2Client, err := ec2.NewEC2MetadataClient(nil)
		if err != nil {
			return nil, err
		}
		return &managedLinux{
			common: commonPlatform,
			client: ec2Client,
		}, nil
	}
	return nil, errors.New("invalid platform: " + platformConfig.Name)
}

// BuildTaskNetworkConfiguration translates network data in task payload sent by ACS
// into the task network configuration data structure internal to the agent.
func (c *common) buildTaskNetworkConfiguration(
	taskID string,
	taskPayload *ecsacs.Task,
	singleNetNS bool,
	ifaceToGuestNetNS map[string]string,
) (*tasknetworkconfig.TaskNetworkConfig, error) {
	mode := types.NetworkMode(aws.ToString(taskPayload.NetworkMode))
	var netNSs []*tasknetworkconfig.NetworkNamespace
	var err error
	switch mode {
	case types.NetworkModeAwsvpc:
		netNSs, err = c.buildAWSVPCNetworkNamespaces(taskID, taskPayload, singleNetNS, ifaceToGuestNetNS)
		if err != nil {
			return nil, errors.Wrap(err, "failed to translate network configuration")
		}
	case types.NetworkModeBridge:
		return nil, errors.New("not implemented")
	case types.NetworkModeHost:
		return nil, errors.New("not implemented")
	case types.NetworkModeNone:
		return nil, errors.New("not implemented")
	default:
		return nil, errors.New("invalid network mode: " + string(mode))
	}

	return &tasknetworkconfig.TaskNetworkConfig{
		NetworkNamespaces: netNSs,
		NetworkMode:       mode,
	}, nil
}

func (c *common) GetNetNSPath(netNSName string) string {
	return c.nsUtil.GetNetNSPath(netNSName)
}

// buildAWSVPCNetworkNamespaces returns list of NetworkNamespace which will be used to
// create the task's network configuration.
// Use cases covered by this method are:
//  1. Single interface, network namespace (the only externally available config).
//  2. Single netns, multiple interfaces (For a non-managed multi-ENI experience. Eg EKS use case).
//  3. Multiple netns, multiple interfaces (future use case for internal customer who need
//     a managed multi-ENI experience).
//  4. Single netns, multiple interfaces (for V2N tasks on FoF).
func (c *common) buildAWSVPCNetworkNamespaces(
	taskID string,
	taskPayload *ecsacs.Task,
	singleNetNS bool,
	ifaceToGuestNetNS map[string]string,
) ([]*tasknetworkconfig.NetworkNamespace, error) {
	if len(taskPayload.ElasticNetworkInterfaces) == 0 {
		return nil, errors.New("interfaces list cannot be empty")
	}

	macToNames, err := c.interfacesMACToName()
	if err != nil {
		return nil, err
	}

	logger.Info("Building network configuration for awsvpc task", map[string]interface{}{
		"SingleNetNS":            singleNetNS,
		"ENICount":               len(taskPayload.ElasticNetworkInterfaces),
		"HasContainerENIMapping": len(taskPayload.Containers[0].NetworkInterfaceNames) == 0,
	})
	// If we require all interfaces to be in one single netns, the network configuration is straight forward.
	// This case is identified if the singleNetNS flag is set, or if the ENIs have an empty 'Name' field,
	// or if there is only on ENI in the payload.
	if singleNetNS || len(taskPayload.ElasticNetworkInterfaces) == 1 ||
		aws.ToString(taskPayload.ElasticNetworkInterfaces[0].Name) == "" ||
		len(taskPayload.Containers[0].NetworkInterfaceNames) == 0 {
		primaryNetNS, err := c.buildNetNS(taskID,
			0,
			taskPayload.ElasticNetworkInterfaces,
			taskPayload.ProxyConfiguration,
			macToNames,
			ifaceToGuestNetNS)
		if err != nil {
			return nil, err
		}

		return []*tasknetworkconfig.NetworkNamespace{primaryNetNS}, nil
	}

	// Create a map for easier lookup of ENIs by their names.
	ifNameMap := make(map[string]*ecsacs.ElasticNetworkInterface, len(taskPayload.ElasticNetworkInterfaces))
	for _, iface := range taskPayload.ElasticNetworkInterfaces {
		ifNameMap[networkinterface.GetInterfaceName(iface)] = iface
	}

	// Proxy configuration is not supported yet in a multi-ENI / multi-NetNS task.
	if taskPayload.ProxyConfiguration != nil {
		return nil, errors.New("unexpected proxy config found")
	}

	// The number of network namespaces required to create depends on the
	// number of unique interface names list across all container definitions
	// in the task payload. Meaning if two containers are linked with the same
	// set of network interface names, both those containers share the same namespace.
	// If not, they reside in two different namespaces. Also, an interface can only
	// belong to one NetworkNamespace object.

	// Order the containers such that the container attached to the interface with index 0 comes first.
	sort.Slice(taskPayload.Containers, func(i, j int) bool {
		iName := aws.ToString(taskPayload.Containers[i].NetworkInterfaceNames[0])
		jName := aws.ToString(taskPayload.Containers[j].NetworkInterfaceNames[0])
		return aws.ToInt64(ifNameMap[iName].Index) < aws.ToInt64(ifNameMap[jName].Index)
	})

	var netNSs []*tasknetworkconfig.NetworkNamespace
	nsIndex := 0
	// Loop through each container definition and their network interfaces.
	for _, container := range taskPayload.Containers {
		// ifaces holds all interfaces associated with a particular container.
		var ifaces []*ecsacs.ElasticNetworkInterface
		for _, ifNameP := range container.NetworkInterfaceNames {
			ifName := aws.ToString(ifNameP)
			if iface := ifNameMap[ifName]; iface != nil {
				ifaces = append(ifaces, iface)
				// Remove ENI from map to indicate that the ENI is assigned to
				// a namespace.
				delete(ifNameMap, ifName)
			} else {
				// If the ENI does not exist in the lookup map, it means the ENI
				// is already assigned to a namespace. The container will be run
				// in the same namespace.
				break
			}
		}

		if len(ifaces) == 0 {
			continue
		}

		netNS, err := c.buildNetNS(taskID, nsIndex, ifaces, nil, macToNames, nil)
		if err != nil {
			return nil, err
		}
		netNSs = append(netNSs, netNS)
		nsIndex += 1
	}

	return netNSs, nil
}

// buildNetNS creates a single network namespace object using the input network config data.
func (c *common) buildNetNS(
	taskID string,
	index int,
	networkInterfaces []*ecsacs.ElasticNetworkInterface,
	proxyConfig *ecsacs.ProxyConfiguration,
	macToName map[string]string,
	ifaceToGuestNetNS map[string]string,
) (*tasknetworkconfig.NetworkNamespace, error) {
	var primaryIF *networkinterface.NetworkInterface
	var ifaces []*networkinterface.NetworkInterface
	lowestIdx := int64(indexHighValue)
	for _, ni := range networkInterfaces {
		guestNetNS := ifaceToGuestNetNS[aws.ToString(ni.Name)]
		iface, err := networkinterface.New(ni, guestNetNS, networkInterfaces, macToName)
		if err != nil {
			return nil, err
		}
		if aws.ToInt64(ni.Index) < lowestIdx {
			primaryIF = iface
			lowestIdx = aws.ToInt64(ni.Index)
		}
		ifaces = append(ifaces, iface)
	}

	primaryIF.Default = true
	netNSName := networkinterface.NetNSName(taskID, primaryIF.Name)
	netNSPath := c.GetNetNSPath(netNSName)

	logger.Info("Building network namespace model", map[string]interface{}{
		"NetNSName": netNSName,
		"NetNSPath": netNSPath,
	})
	return tasknetworkconfig.NewNetworkNamespace(
		netNSName,
		netNSPath,
		index,
		proxyConfig,
		ifaces...)
}

// CreateNetNS creates a new network namespace with the specified path.
func (c *common) CreateNetNS(netNSPath string) error {
	logger.Info("Creating network namespace", map[string]interface{}{
		"NetNSPath": netNSPath,
	})
	nsExists, err := c.nsUtil.NSExists(netNSPath)
	if err != nil {
		return errors.Wrapf(err, "failed to check netns %s", netNSPath)
	}

	if nsExists {
		return nil
	}

	err = c.nsUtil.NewNetNS(netNSPath)
	if err != nil {
		return errors.Wrapf(err, "failed to create netns %s", netNSPath)
	}

	// The loopback interface in a new network namespace is down by default in Linux.
	// In case of a container launched using Docker, Docker itself ensures that loopback is up.
	// Manually set the operational state to up to allow loopback communication.
	err = c.nsUtil.ExecInNSPath(netNSPath, c.setUpLoFunc(netNSPath))

	return err
}

func (c *common) DeleteNetNS(netNSPath string) error {
	logger.Info("Deleting network namespace", map[string]interface{}{
		"NetNSPath": netNSPath,
	})
	nsExists, err := c.nsUtil.NSExists(netNSPath)
	if err != nil {
		return errors.Wrapf(err, "failed to check netns %s", netNSPath)
	}

	if !nsExists {
		return nil
	}

	err = c.nsUtil.DelNetNS(netNSPath)
	if err != nil {
		return errors.Wrapf(err, "failed to delete netns %s", netNSPath)
	}

	return nil
}

// DeleteDNSConfig deletes the directory at /etc/netns/<netns-name> and all its files.
func (c *common) DeleteDNSConfig(netNSName string) error {
	logger.Info("Deleting DNS config", map[string]interface{}{
		"NetNSName": netNSName,
	})

	if netNSName == "" {
		return errors.New("netns name cannot be empty")
	}
	netNSDir := filepath.Join(networkConfigFileDirectory, netNSName)
	_, err := c.os.Stat(netNSDir)
	if c.os.IsNotExist(err) {
		return errors.Wrap(err, "network config directory not found")
	} else if err != nil {
		return err
	}

	return c.os.RemoveAll(netNSDir)
}

// setUpLoFunc returns a method that sets the loop back interface inside a
// particular network namespace to the state "UP". This function is used to
// set up the loop back interface inside a task network namespace soon after
// its creation.
func (c *common) setUpLoFunc(netNSPath string) func(cnins.NetNS) error {
	return func(cnins.NetNS) error {
		// Get a handle to the loop back interface.
		link, err := c.netlink.LinkByName("lo")
		if err != nil {
			return errors.Wrapf(err, "failed to find loopback interface in %s", netNSPath)
		}

		// Bring up the interface (ip link set dev lo up).
		err = c.netlink.LinkSetUp(link)
		if err != nil {
			return errors.Wrapf(err, "failed to bring up loopback interface in %s", netNSPath)
		}

		return nil
	}
}

// createDNSConfig creates the DNS config files for a particular network namespace.
// If the agent is running on debug mode, it reuses the host's DNS config files.
// If not, it gathers the DNS related data from the netns primary interface.
// The DNS files are written to the network namespace dir "/etc/netns/<netns-name>/".
// Afterward, these files are copied into the task volume to be bind-mounted into
// task containers.
func (c *common) createDNSConfig(
	taskID string,
	reuseHostDNSConfig bool,
	netNS *tasknetworkconfig.NetworkNamespace) error {
	logger.Info("Creating DNS config", map[string]interface{}{
		"NetNSPath":          netNS.Path,
		"ReuseHostDNSConfig": reuseHostDNSConfig,
	})

	// For debug mode, resolv.conf and hosts files are same as the host machine if no available
	// data in the ENI, like DNS resolver.
	// But for non debug mode they are all created using the data available in the ENI.
	primaryIF := netNS.GetPrimaryInterface()
	if primaryIF == nil {
		return errors.New("unable to find primary interface")
	}
	if reuseHostDNSConfig {
		if err := c.generateNetworkConfigFiles(netNS.Name, primaryIF); err != nil {
			return errors.Wrap(err, "unable to copy dns config files")
		}
	} else {
		if err := c.createNetworkConfigFiles(netNS.Name, primaryIF); err != nil {
			return errors.Wrap(err, "unable to create dns config file")
		}
	}

	// Next, copy these files into a task volume, which can be used by containers as well, to
	// configure their network.
	if err := c.copyNetworkConfigFilesToTask(taskID, netNS.Name); err != nil {
		return err
	}
	return nil
}

// createNetworkConfigFiles gathers DNS config information from a network interface
// object and writes them into the following files:
// 1. /etc/netns/<netNSName>/resolv.conf
// 2. /etc/netns/<netNSName>/hostname
// 3. /etc/netns/<netNSName>/hosts
func (c *common) createNetworkConfigFiles(netNSName string, primaryIF *networkinterface.NetworkInterface) error {
	logger.Info("Creating DNS config files in netns directory", map[string]interface{}{
		"NetNSName": netNSName,
	})

	// Create the dns configuration file directory.
	_, err := c.os.Stat(filepath.Join(networkConfigFileDirectory, netNSName))
	if err != nil && c.os.IsNotExist(err) {
		err = c.os.MkdirAll(
			filepath.Join(networkConfigFileDirectory, netNSName),
			networkConfigFileMode)
	}
	if err != nil {
		return errors.Wrap(err, "unable to create the dns config directory")
	}

	err = c.createResolvConfigFile(netNSName, primaryIF)
	if err != nil {
		return errors.Wrap(err, "unable to create resolv conf for netns")
	}

	err = c.createHostnameFileForNetNS(netNSName, primaryIF)
	if err != nil {
		return errors.Wrap(err, "unable to create hostname file for netns")
	}

	err = c.createHostnameFileForDefaultNetNS()
	if err != nil {
		return errors.Wrap(err, "unable to verify the existence of /etc/hostname on the host")
	}
	err = c.createHostsFile(netNSName, primaryIF)
	if err != nil {
		return errors.Wrap(err, "unable to create hosts file for netns")
	}
	return nil
}

// copyNetworkConfigFilesToTask copies the contents of the DNS config files for a
// task into the task volume.
func (c *common) copyNetworkConfigFilesToTask(taskID, netNSName string) error {
	configFiles := []string{HostsFileName, ResolveConfFileName, HostnameFileName}
	for _, file := range configFiles {
		source := filepath.Join(networkConfigFileDirectory, netNSName, file)
		err := c.dnsVolumeAccessor.CopyToVolume(taskID, source, file, networkConfigFileMode)
		if err != nil {
			return errors.Wrapf(err, "unable to populate %s for task", file)
		}
	}
	return nil
}

// generateNetworkConfigFiles generates network configuration files needed by containers.
// In this case, instead of always creating new files, it just copies the relevant ones as required from
// the host except resolv.conf. The resolv.conf will be generated from associated payload if that has DNS
// resolver information otherwise it will be also copied from the host.
func (c *common) generateNetworkConfigFiles(
	filesDirName string,
	iface *networkinterface.NetworkInterface) error {
	err := c.createHostnameFileForDefaultNetNS()
	if err != nil {
		return errors.Wrap(err, "unable to verify the existence of /etc/hostname on the host")
	}

	netNSDir := filepath.Join(networkConfigFileDirectory, filesDirName)
	_, err = c.os.Stat(netNSDir)
	if err != nil && c.os.IsNotExist(err) {
		err = c.os.MkdirAll(netNSDir, networkConfigFileMode)
	}
	if err != nil {
		return errors.Wrap(err, "unable to create the dns config directory")
	}

	err = c.createHostnameFileForNetNS(filesDirName, iface)
	if err != nil {
		return errors.Wrap(err, "unable to create hostname file for netns")
	}

	if len(iface.DomainNameServers) > 0 {
		// Use DNS resolver information from the payload.
		logger.Info("Creating resolv.conf file", map[string]interface{}{
			"ResolveConfFile": filepath.Join(netNSDir, ResolveConfFileName),
		})

		data := c.nsUtil.BuildResolvConfig(iface.DomainNameServers, iface.DomainNameSearchList)
		err := c.ioutil.WriteFile(
			filepath.Join(netNSDir, ResolveConfFileName),
			[]byte(data),
			taskDNSConfigFileMode,
		)
		if err != nil {
			return err
		}
	} else {
		// Copy Host's resolv.conf file.
		err = c.copyFile(filepath.Join(netNSDir, ResolveConfFileName),
			filepath.Join(c.resolvConfPath, ResolveConfFileName),
			taskDNSConfigFileMode)
		if err != nil {
			return err
		}
	}

	err = c.copyFile(filepath.Join(netNSDir, HostsFileName), "/etc/hosts", taskDNSConfigFileMode)
	if err != nil {
		return err
	}
	return nil
}

func (c *common) copyFile(dst, src string, fileMode os.FileMode) error {
	contents, err := c.ioutil.ReadFile(src)
	if err != nil {
		return errors.Wrapf(err, "unable to read %s", src)
	}
	err = c.ioutil.WriteFile(dst, contents, fileMode)
	if err != nil {
		return errors.Wrapf(err, "unable to write to %s", dst)
	}
	return nil
}

// createHostnameFileForNetNS creates the hostname file for the given network namespace.
func (c *common) createHostnameFileForNetNS(netConfigFilesDir string, iface *networkinterface.NetworkInterface) error {
	logger.Info("Creating hostname file", map[string]interface{}{
		"NetConfigFilesDir": netConfigFilesDir,
	})

	// \n is used as line separater for hosts file. Therefore we add \n at the end.
	// Ref: https://github.com/moby/libnetwork/blob/v0.5.6/resolvconf/resolvconf.go#L209-L237
	hostname := fmt.Sprintf("%s\n", iface.GetHostname())

	return c.ioutil.WriteFile(
		filepath.Join(networkConfigFileDirectory, netConfigFilesDir, HostnameFileName),
		[]byte(hostname),
		networkConfigFileMode)
}

// createHostnameFileForDefaultNetNS creates the hostname file for the default namespace
// if required. "ip netns exec" emits an error message if it cannot find the /etc/hostname
// file on the host's filesystem. Depending on the AMI config, that file might sometimes
// be absent. This method creates an empty file in cases where the file cannot be found.
func (c *common) createHostnameFileForDefaultNetNS() error {
	f, err := c.os.OpenFile(networkConfigHostnameFilePath, os.O_RDONLY|os.O_CREATE, networkConfigFileMode)
	if err != nil {
		return err
	}

	defer f.Close()
	return nil
}

func (c *common) createResolvConfigFile(netConfigFilesDir string, iface *networkinterface.NetworkInterface) error {
	logger.Info("Creating resolv.conf file", map[string]interface{}{
		"NetConfigFilesDir": netConfigFilesDir,
	})

	data := c.nsUtil.BuildResolvConfig(iface.DomainNameServers, iface.DomainNameSearchList)

	return c.ioutil.WriteFile(
		filepath.Join(networkConfigFileDirectory, netConfigFilesDir, ResolveConfFileName),
		[]byte(data),
		networkConfigFileMode)
}

func (c *common) createHostsFile(netNSName string, iface *networkinterface.NetworkInterface) error {
	logger.Info("Creating hosts file", map[string]interface{}{
		"NetNSName": netNSName,
	})

	var contents bytes.Buffer
	if iface.IPv6Only() {
		fmt.Fprintf(&contents, "%s\n%s %s\n",
			HostsLocalhostEntryIPv6, iface.GetPrimaryIPv6Address(), iface.GetHostname())
	} else {
		// \n is used as line separater for hosts file. Therefore we add \n at the end.
		// Ref: https://github.com/moby/libnetwork/blob/v0.5.6/resolvconf/resolvconf.go#L209-L237
		fmt.Fprintf(&contents, "%s\n%s %s\n",
			HostsLocalhostEntryIPv4, iface.GetPrimaryIPv4Address(), iface.GetHostname())
	}

	// Add any additional DNS entries associated with the ENI. This is required
	// for service connect enabled tasks.
	for _, dnsMapping := range iface.DNSMappingList {
		fmt.Fprintf(&contents, "%s %s\n", dnsMapping.Address, dnsMapping.Hostname)
	}

	return c.ioutil.WriteFile(
		filepath.Join(networkConfigFileDirectory, netNSName, HostsFileName),
		contents.Bytes(),
		networkConfigFileMode)
}

// configureInterface initiates the workflow for setting up a network interface
// inside a network namespace.
func (c *common) configureInterface(
	ctx context.Context,
	netNSPath string,
	iface *networkinterface.NetworkInterface,
	netDAO netlibdata.NetworkDataClient,
) error {
	var err error
	switch iface.InterfaceAssociationProtocol {
	case networkinterface.DefaultInterfaceAssociationProtocol:
		err = c.configureRegularENI(ctx, netNSPath, iface)
	case networkinterface.VLANInterfaceAssociationProtocol:
		err = c.configureBranchENI(ctx, netNSPath, iface)
	case networkinterface.V2NInterfaceAssociationProtocol:
		err = c.configureGENEVEInterface(ctx, netNSPath, iface, netDAO)
	case networkinterface.VETHInterfaceAssociationProtocol:
		// Do nothing.
		return nil
	default:
		err = errors.New("invalid interface association protocol " + iface.InterfaceAssociationProtocol)
	}
	return err
}

// configureRegularENI configures a network interface for an ENI.
func (c *common) configureRegularENI(ctx context.Context, netNSPath string, eni *networkinterface.NetworkInterface) error {
	logger.Info("Configuring regular ENI", map[string]interface{}{
		"ENIName":   eni.Name,
		"NetNSPath": netNSPath,
	})

	var cniNetConf []ecscni.PluginConfig
	var add bool
	var err error

	c.os.Setenv(CNIPluginLogFileEnv, ecscni.PluginLogPath)
	c.os.Setenv(IPAMDataPathEnv, filepath.Join(c.stateDBDir, IPAMDataFileName))

	switch eni.DesiredStatus {
	case status.NetworkReadyPull:
		// The task metadata interface setup by bridge plugin is required only for the primary ENI.
		if eni.IsPrimary() {
			cniNetConf = append(cniNetConf, createBridgePluginConfig(netNSPath))
		}
		cniNetConf = append(cniNetConf, createENIPluginConfigs(netNSPath, eni))
		add = true
	case status.NetworkDeleted:
		// Regular ENIs are used in single-use warmpool instances, so cleanup isn't necessary.
		cniNetConf = nil
		add = false
	}

	_, err = c.executeCNIPlugin(ctx, add, cniNetConf...)
	if err != nil {
		err = errors.Wrap(err, "failed to setup regular eni")
	}

	return err
}

// configureBranchENI configures a network interface for a branch ENI.
func (c *common) configureBranchENI(ctx context.Context, netNSPath string, eni *networkinterface.NetworkInterface) error {
	logger.Info("Configuring branch ENI", map[string]interface{}{
		"ENIName":   eni.Name,
		"NetNSPath": netNSPath,
	})

	// Set the path for the IPAM CNI local db to track assigned IPs.
	// Default path is /data but in some linux distros (i.e.Amazon BottleRocket) the root volume is read-only.
	c.os.Setenv(IPAMDataPathEnv, filepath.Join(c.stateDBDir, IPAMDataFileName))

	var cniNetConf []ecscni.PluginConfig
	var err error
	add := true

	// Generate CNI network configuration based on the ENI's desired state.
	switch eni.DesiredStatus {
	case status.NetworkReadyPull:
		// Setup bridge to connect task network namespace to TMDS running in host's primary netns.
		if eni.IsPrimary() {
			cniNetConf = append(cniNetConf, createBridgePluginConfig(netNSPath))
		}
		// We block IMDS access in awsvpc tasks.
		cniNetConf = append(cniNetConf, createBranchENIConfig(netNSPath, eni, VPCBranchENIInterfaceTypeVlan, blockInstanceMetadataDefault))
	case status.NetworkDeleted:
		cniNetConf = append(cniNetConf, createBranchENIConfig(netNSPath, eni, VPCBranchENIInterfaceTypeVlan, blockInstanceMetadataDefault))
		add = false
	}

	_, err = c.executeCNIPlugin(ctx, add, cniNetConf...)
	if err != nil {
		err = errors.Wrap(err, "failed to setup branch eni")
	}

	return err
}

func (c *common) configureGENEVEInterface(
	ctx context.Context,
	netNSPath string,
	iface *networkinterface.NetworkInterface,
	netDAO netlibdata.NetworkDataClient,
) error {
	logger.Info("Configuring GENEVE interface", map[string]interface{}{
		"ENIName":   iface.Name,
		"NetNSPath": netNSPath,
	})

	var cniNetConf ecscni.PluginConfig
	add := true

	// Generate CNI network configuration based on the ENI's desired state.
	switch iface.DesiredStatus {
	case status.NetworkReadyPull:
		// Assign destination port and set device name for V2N ENI.
		if err := ecscni.SetV2NDstPortAndDeviceName(iface, netDAO); err != nil {
			return err
		}
		cniNetConf = NewTunnelConfig(netNSPath, iface, VPCTunnelInterfaceTypeGeneve)
	case status.NetworkReady:
		cniNetConf = NewTunnelConfig(netNSPath, iface, VPCTunnelInterfaceTypeTap)
	case status.NetworkDeleted:
		cniNetConf = NewTunnelConfig(netNSPath, iface, VPCTunnelInterfaceTypeTap)
		add = false
	}

	result, err := c.executeCNIPlugin(ctx, add, cniNetConf)
	if err != nil {
		return err
	}

	logger.Info("GENEVE interface configured", map[string]interface{}{
		"ENIName":   iface.Name,
		"NetNSPath": netNSPath,
	})

	switch iface.DesiredStatus {
	case status.NetworkReadyPull:
		// Once the ENI configuration for the READY_PULL state is complete, we need to read the
		// OS assigned MAC address of the GENEVE interface. This MAC address is then associated
		// with the V2N ENI. This MAC address will get persisted to the database once the ENI
		// manager's state transitions from NONE to READY_PULL.
		//
		// This MAC address will later be used to create the interface definition
		// in the request for creating the MicroVM. The benefit of this approach is that the agent
		// will be aware of the MAC address assigned to the TAP interface inside the MicroVM (the one
		// connected to the GENEVE interface) and can use this info for the network setup inside
		// the MicroVM at a later stage.
		if len(result) == 0 {
			return errors.New("eni pull configuration: empty result from network setup")
		}
		newResult, err := cnitypes.GetResult(*result[0])
		if err != nil {
			return err
		}
		if len(newResult.Interfaces) == 0 || newResult.Interfaces[0].Mac == "" {
			return errors.New("eni pull configuration: no mac address assigned")
		}
		iface.MacAddress = newResult.Interfaces[0].Mac
	case status.NetworkDeleted:
		// Once the task is stopped and the GENEVE interface is deleted, we can release the port so
		// that it can be re-used by another task.
		vni := iface.TunnelProperties.ID
		dstPort := iface.TunnelProperties.DestinationPort
		if err = netDAO.ReleaseGeneveDstPort(dstPort, vni); err != nil {
			return err
		}
	}

	return nil
}

// configureAppMesh configures AppMesh in a network namespace.
// This is used by warmpool and debug-warmpool platforms.
func (c *common) configureAppMesh(
	ctx context.Context,
	netNSPath string,
	cfg *appmesh.AppMesh,
) error {
	logger.Info("Configuring AppMesh", map[string]interface{}{
		"NetNSPath": netNSPath,
	})

	c.os.Setenv(CNIPluginLogFileEnv, ecscni.PluginLogPath)
	defer c.os.Unsetenv(CNIPluginLogFileEnv)

	cniNetConf := createAppMeshPluginConfig(netNSPath, cfg)

	_, err := c.executeCNIPlugin(ctx, true, cniNetConf)
	if err != nil {
		err = errors.Wrapf(err, "failed to setup appmesh netconfig %s", cniNetConf.String())
	}

	return err
}

// configureServiceConnect configures the task network namespace with service connect
// specific iptables rules.
func (c *common) configureServiceConnect(
	ctx context.Context,
	netNSPath string,
	taskENI *networkinterface.NetworkInterface,
	scConfig *serviceconnect.ServiceConnectConfig,
) error {
	logger.Info("Configuring ServiceConnect", map[string]interface{}{
		"NetNSPath": netNSPath,
	})

	c.os.Setenv(CNIPluginLogFileEnv, ecscni.PluginLogPath)
	defer c.os.Unsetenv(CNIPluginLogFileEnv)

	cniConf := createServiceConnectCNIConfig(taskENI, netNSPath, scConfig)
	_, err := c.executeCNIPlugin(ctx, true, cniConf)
	if err != nil {
		return errors.Wrapf(err, "failed to setup service connect CNI plugin %s", cniConf.String())
	}

	return nil
}
