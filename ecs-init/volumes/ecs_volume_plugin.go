// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package volumes

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/docker/go-plugins-helpers/volume"
)

const (
	// VolumeMountPathPrefix is the host path where amazon ECS plugin's volumes are mounted
	VolumeMountPathPrefix = "/var/lib/ecs/volumes/"
	// FilePerm is the file permissions for the host volume mount directory
	FilePerm = 0700
)

// AmazonECSVolumePlugin holds list of volume drivers and volumes information
type AmazonECSVolumePlugin struct {
	volumeDrivers map[string]VolumeDriver
	volumes       map[string]*Volume
	lock          sync.RWMutex
}

// NewAmazonECSVolumePlugin initiates the volume drivers
func NewAmazonECSVolumePlugin() *AmazonECSVolumePlugin {
	return &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{
			"efs": NewECSVolumeDriver(),
		},
		volumes: make(map[string]*Volume),
	}
}

// VolumeDriver contains the methods for volume drivers to implement
type VolumeDriver interface {
	Create(*CreateRequest) error
	Remove(*RemoveRequest) error
}

// Volume holds full details about a volume
type Volume struct {
	Type    string
	Path    string
	Options map[string]string
}

// CreateRequest holds fields necessary for creating a volume
type CreateRequest struct {
	Name    string
	Path    string
	Options map[string]string
}

// RemoveRequest holds fields necessary for removing a volume
type RemoveRequest struct {
	Name string
}

func (a *AmazonECSVolumePlugin) getVolumeDriver(driverType string) (VolumeDriver, error) {
	if _, ok := a.volumeDrivers[driverType]; !ok {
		return nil, fmt.Errorf("no volume driver found for type %s", driverType)
	}
	return a.volumeDrivers[driverType], nil
}

// Create implements Docker volume plugin's Create Method
func (a *AmazonECSVolumePlugin) Create(r *volume.CreateRequest) error {
	a.lock.Lock()
	defer a.lock.Unlock()
	// get driver type from options to get the corresponding volume driver
	var driverType string
	for k, v := range r.Options {
		switch k {
		case "type":
			driverType = v
		}
	}
	if driverType == "" {
		return errors.New("volume type not specified")
	}
	volDriver, err := a.getVolumeDriver(driverType)
	if err != nil {
		return err
	}
	if volDriver == nil {
		return fmt.Errorf("no volume driver found for type %s", driverType)
	}
	volPath, err := a.GetMountPath(r.Name)
	if err != nil {
		return err
	}
	req := &CreateRequest{
		Name:    r.Name,
		Path:    volPath,
		Options: r.Options,
	}
	err = volDriver.Create(req)
	if err != nil {
		return err
	}
	vol := &Volume{
		Type:    driverType,
		Path:    volPath,
		Options: r.Options,
	}
	a.volumes[r.Name] = vol
	return nil
}

// GetMountPath returns the host path where volume will be mounted
func (a *AmazonECSVolumePlugin) GetMountPath(name string) (string, error) {
	path := VolumeMountPathPrefix + name
	err := CreateMountPath(path)
	if err != nil {
		return "", fmt.Errorf("cannot create mount point for volume: %s", err)
	}
	return path, nil
}

var CreateMountPath = CreateMountDir

func CreateMountDir(path string) error {
	return os.MkdirAll(path, FilePerm)
}

// CleanupMountPath cleans up the volume's host path
func (a *AmazonECSVolumePlugin) CleanupMountPath(name string) error {
	return RemoveMountPath(name)
}

var RemoveMountPath = DeleteMountPath

func DeleteMountPath(path string) error {
	return os.Remove(path)
}

// Mount implements Docker volume plugin's Mount Method
func (a *AmazonECSVolumePlugin) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	a.lock.RLock()
	defer a.lock.RUnlock()
	vol, ok := a.volumes[r.Name]
	if !ok {
		return nil, fmt.Errorf("volume %s not found", r.Name)
	}
	return &volume.MountResponse{Mountpoint: vol.Path}, nil
}

// Unmount implements Docker volume plugin's Unmount Method
func (a *AmazonECSVolumePlugin) Unmount(r *volume.UnmountRequest) error {
	a.lock.RLock()
	defer a.lock.RUnlock()
	_, ok := a.volumes[r.Name]
	if !ok {
		return fmt.Errorf("volume %s not found", r.Name)
	}
	return nil
}

// Remove implements Docker volume plugin's Remove Method
func (a *AmazonECSVolumePlugin) Remove(r *volume.RemoveRequest) error {
	a.lock.Lock()
	defer a.lock.Unlock()
	vol, ok := a.volumes[r.Name]
	if !ok {
		return fmt.Errorf("volume %s not found", r.Name)
	}
	volDriver, err := a.getVolumeDriver(vol.Type)
	if err != nil {
		return err
	}
	if volDriver == nil {
		return fmt.Errorf("no corresponding volume driver found for type %s", vol.Type)
	}
	req := &RemoveRequest{
		Name: r.Name,
	}
	err = volDriver.Remove(req)
	if err != nil {
		return err
	}
	delete(a.volumes, r.Name)
	err = a.CleanupMountPath(vol.Path)
	// TODO: capture error for above and log
	return nil
}

// List implements Docker volume plugin's List Method
func (a *AmazonECSVolumePlugin) List() (*volume.ListResponse, error) {
	a.lock.RLock()
	defer a.lock.RUnlock()
	vols := make([]*volume.Volume, len(a.volumes))
	i := 0
	for volName := range a.volumes {
		vols[i] = &volume.Volume{
			Name: volName,
		}
		i++
	}
	res := &volume.ListResponse{
		Volumes: vols,
	}
	return res, nil
}

// Get implements Docker volume plugin's Get Method
func (a *AmazonECSVolumePlugin) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	a.lock.RLock()
	defer a.lock.RUnlock()
	_, ok := a.volumes[r.Name]
	if !ok {
		return nil, fmt.Errorf("volume %s not found", r.Name)
	}
	vol := &volume.Volume{
		Name: r.Name,
	}
	return &volume.GetResponse{Volume: vol}, nil
}

// Path implements Docker volume plugin's Path Method
func (a *AmazonECSVolumePlugin) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	a.lock.RLock()
	defer a.lock.RUnlock()
	vol, ok := a.volumes[r.Name]
	if !ok {
		return nil, fmt.Errorf("volume %s not found", r.Name)
	}

	return &volume.PathResponse{Mountpoint: vol.Path}, nil
}

// Capabilities implements Docker volume plugin's Capabilities Method
func (a *AmazonECSVolumePlugin) Capabilities() *volume.CapabilitiesResponse {
	return nil
}
