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

//go:build linux
// +build linux

package ecscni

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	cnins "github.com/containernetworking/plugins/pkg/ns"
	"github.com/pkg/errors"
)

const (
	// dnsNameServerFormat defines the entry of nameserver in resov.conf file
	dnsNameServerFormat = "nameserver %s\n"
	// dnsNameServerFormat defines the entry of search in resov.conf file
	dnsSearchDomainFormat = "search %s\n"
)

// NetNSUtil provides some basic methods for agent to deal with network namespace
type NetNSUtil interface {
	// NewNetNS creates a new network namespace in the system
	NewNetNS(nsPath string) error
	// DelNetNS deletes the network namespace from the system
	DelNetNS(nsPath string) error
	// GetNetNSPath cretes the network namespace path from named namespace
	GetNetNSPath(nsName string) string
	// GetNetNSName extract the ns name from the netns path
	GetNetNSName(nsPath string) string
	// NSExists checks if the given ns path exists or not
	NSExists(nsPath string) (bool, error)
	// ExecInNSPath invokes the function in the given network namespace
	ExecInNSPath(nsPath string, cb func(cnins.NetNS) error) error
	// BuildResolvConfig constructs the content of dns configuration file resolv.conf
	BuildResolvConfig(nameservers, searchDomains []string) string
}

type netnsutil struct {
}

func NewNetNSUtil() NetNSUtil {
	return &netnsutil{}
}

// NewNetNS create a new network namespace with given path
func (*netnsutil) NewNetNS(nspath string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	currentNS, err := cnins.GetCurrentNS()
	if err != nil {
		return errors.Wrap(err, "unable to get the current network namespace")
	}

	_, err = os.Stat(NETNS_PATH_DEFAULT)
	if err != nil {
		// Create the default network namespace directory path if not exists
		if os.IsNotExist(err) {
			err = os.Mkdir(NETNS_PATH_DEFAULT, NsFileMode)
		}
		if err != nil {
			return errors.Wrap(err, "unable to get status of default ns directory")
		}
	}

	// Create a new network namespace
	err = syscall.Unshare(syscall.CLONE_NEWNET)
	if err != nil {
		return errors.Wrap(err, "unable to create new ns with unshare")
	}

	// Make this network namespace persistent
	f, err := os.OpenFile(nspath, os.O_CREATE|os.O_EXCL, NsFileMode)
	if err != nil {
		return errors.Wrap(err, "unable to create ns file")
	}
	f.Close()

	nsPath := fmt.Sprintf(NETNS_PROC_FORMAT, os.Getpid(), syscall.Gettid())
	if err = syscall.Mount(nsPath, nspath, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return errors.Wrap(err, "unable to mount the ns path")
	}

	return currentNS.Set()
}

// DelNetNS remove the given network namespace
func (*netnsutil) DelNetNS(nspath string) error {
	if err := syscall.Unmount(nspath, syscall.MNT_DETACH); err != nil {
		return errors.Wrap(err, "unable to unmount the ns path")
	}

	if err := syscall.Unlink(nspath); err != nil {
		return errors.Wrap(err, "unable to del the ns file")
	}
	return nil
}

// GetNetNSPath returns the full path for the given named network namespace
func (*netnsutil) GetNetNSPath(name string) string {
	return filepath.Join(NETNS_PATH_DEFAULT, name)
}

func (*netnsutil) GetNetNSName(path string) string {
	return filepath.Base(path)
}

// NSExists checks if the given namespace exists
func (nu *netnsutil) NSExists(nspath string) (bool, error) {
	stat := &syscall.Statfs_t{}
	err := syscall.Statfs(nspath, stat)
	if os.IsNotExist(err) {
		return false, nil
	}

	return true, errors.Wrap(err, "unable to get the status of ns file")
}

func (nu *netnsutil) ExecInNSPath(netNSPath string, toRun func(cnins.NetNS) error) error {
	return cnins.WithNetNSPath(netNSPath, toRun)
}

// BuildResolvConfig constructs the content of dns configuration file resolv.conf
func (nu *netnsutil) BuildResolvConfig(nameservers, searchDomains []string) string {
	var bf strings.Builder
	for _, nameserver := range nameservers {
		bf.WriteString(fmt.Sprintf(dnsNameServerFormat, nameserver))
	}

	if len(searchDomains) != 0 {
		searchDomainsList := strings.Join(searchDomains, " ")
		bf.WriteString(fmt.Sprintf(dnsSearchDomainFormat, searchDomainsList))
	}

	return bf.String()
}
