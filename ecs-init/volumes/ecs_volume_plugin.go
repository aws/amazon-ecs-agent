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
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/cihub/seelog"
	"github.com/docker/go-plugins-helpers/volume"
)

const (
	// VolumeMountPathPrefix is the host path where amazon ECS plugin's volumes are mounted
	VolumeMountPathPrefix = "/var/lib/ecs/volumes/"
	// FilePerm is the file permissions for the host volume mount directory
	FilePerm          = 0700
	defaultDriverType = "efs"
)

// AmazonECSVolumePlugin holds list of volume drivers and volumes information
type AmazonECSVolumePlugin struct {
	volumeDrivers map[string]VolumeDriver
	volumes       map[string]*Volume
	state         *StateManager
	lock          sync.RWMutex
}

// NewAmazonECSVolumePlugin initiates the volume drivers
func NewAmazonECSVolumePlugin() *AmazonECSVolumePlugin {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{
			"efs": NewECSVolumeDriver(),
		},
		volumes: make(map[string]*Volume),
		state:   NewStateManager(),
	}
	return plugin
}

// VolumeDriver contains the methods for volume drivers to implement
type VolumeDriver interface {
	Setup(string, *Volume)
	Create(*CreateRequest) error
	Remove(*RemoveRequest) error
}

// Volume holds full details about a volume
type Volume struct {
	Type      string
	Path      string
	Options   map[string]string
	CreatedAt string
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

// LoadState loads past state information of the plugin
func (a *AmazonECSVolumePlugin) LoadState() error {
	a.lock.Lock()
	defer a.lock.Unlock()
	seelog.Info("Loading plugin state information")
	oldState := &VolumeState{}
	if !fileExists(PluginStateFileAbsPath) {
		return nil
	}
	if err := a.state.load(oldState); err != nil {
		seelog.Errorf("Could not load state: %v", err)
		return fmt.Errorf("could not load plugin state: %v", err)
	}
	// empty state file
	if oldState.Volumes == nil {
		return nil
	}
	for volName, vol := range oldState.Volumes {
		voldriver, err := a.getVolumeDriver(vol.Type)
		if err != nil {
			seelog.Errorf("Could not load state: %v", err)
			return fmt.Errorf("could not load plugin state: %v", err)
		}
		volume := &Volume{
			Type:      vol.Type,
			Path:      vol.Path,
			Options:   vol.Options,
			CreatedAt: vol.CreatedAt,
		}
		a.volumes[volName] = volume
		voldriver.Setup(volName, volume)
	}
	a.state.VolState = oldState
	return nil
}

func (a *AmazonECSVolumePlugin) getVolumeDriver(driverType string) (VolumeDriver, error) {
	if driverType == "" {
		return a.volumeDrivers[defaultDriverType], nil
	}
	if _, ok := a.volumeDrivers[driverType]; !ok {
		return nil, fmt.Errorf("volume %s type not supported", driverType)
	}
	return a.volumeDrivers[driverType], nil
}

// Create implements Docker volume plugin's Create Method
func (a *AmazonECSVolumePlugin) Create(r *volume.CreateRequest) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	seelog.Infof("Creating new volume %s", r.Name)
	_, ok := a.volumes[r.Name]
	if ok {
		return fmt.Errorf("volume %s already exists", r.Name)
	}

	// get driver type from options to get the corresponding volume driver
	var driverType, target string
	for k, v := range r.Options {
		switch k {
		case "type":
			driverType = v
		case "target":
			target = v
		}
	}
	volDriver, err := a.getVolumeDriver(driverType)
	if err != nil {
		seelog.Errorf("Volume %s's driver type %s not supported", r.Name, driverType)
		return err
	}
	if volDriver == nil {
		// this case should not happen normally
		return fmt.Errorf("no volume driver found for type %s", driverType)
	}

	if target == "" {
		seelog.Infof("Creating mount target for new volume %s", r.Name)
		// create the mount path on the host for the volume to be created
		target, err = a.GetMountPath(r.Name)
		if err != nil {
			seelog.Errorf("Volume %s creation failure: %v", r.Name, err)
			return err
		}
	}

	req := &CreateRequest{
		Name:    r.Name,
		Path:    target,
		Options: r.Options,
	}
	err = volDriver.Create(req)
	if err != nil {
		seelog.Errorf("Volume %s creation failure: %v", r.Name, err)
		cErr := a.CleanupMountPath(target)
		if cErr != nil {
			seelog.Warnf("Failed to cleanup mount path for volume %s: %v", r.Name, cErr)
		}
		return err
	}
	seelog.Infof("Volume %s created successfully", r.Name)
	vol := &Volume{
		Type:      driverType,
		Path:      target,
		Options:   r.Options,
		CreatedAt: time.Now().Format(time.RFC3339Nano),
	}
	// record the volume information
	a.volumes[r.Name] = vol
	seelog.Infof("Saving state of new volume %s", r.Name)
	// save the state of new volume
	err = a.state.recordVolume(r.Name, vol)
	if err != nil {
		seelog.Errorf("Error saving state of new volume %s: %v", r.Name, err)
	}
	return nil
}

// GetMountPath returns the host path where volume will be mounted
func (a *AmazonECSVolumePlugin) GetMountPath(name string) (string, error) {
	path := VolumeMountPathPrefix + name
	err := createMountPath(path)
	if err != nil {
		return "", fmt.Errorf("cannot create mount point: %v", err)
	}
	return path, nil
}

var createMountPath = createMountDir

func createMountDir(path string) error {
	return os.MkdirAll(path, FilePerm)
}

// CleanupMountPath cleans up the volume's host path
func (a *AmazonECSVolumePlugin) CleanupMountPath(name string) error {
	return removeMountPath(name)
}

var removeMountPath = deleteMountPath

func deleteMountPath(path string) error {
	return os.Remove(path)
}

// Mount implements Docker volume plugin's Mount Method
func (a *AmazonECSVolumePlugin) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	a.lock.RLock()
	defer a.lock.RUnlock()
	vol, ok := a.volumes[r.Name]
	if !ok {
		seelog.Errorf("Volume %s to mount is not found", r.Name)
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
		seelog.Errorf("Volume %s to unmount is not found", r.Name)
		return fmt.Errorf("volume %s not found", r.Name)
	}
	return nil
}

// Remove implements Docker volume plugin's Remove Method
func (a *AmazonECSVolumePlugin) Remove(r *volume.RemoveRequest) error {
	a.lock.Lock()
	defer a.lock.Unlock()
	seelog.Infof("Removing volume %s", r.Name)
	vol, ok := a.volumes[r.Name]
	if !ok {
		seelog.Errorf("Volume %s to remove is not found", r.Name)
		return fmt.Errorf("volume %s not found", r.Name)
	}

	// get corresponding volume driver to unmount
	volDriver, err := a.getVolumeDriver(vol.Type)
	if err != nil {
		seelog.Errorf("Volume %s removal failure: %s", r.Name, err)
		return err
	}
	if volDriver == nil {
		// this case should not happen normally
		return fmt.Errorf("no corresponding volume driver found for type %s", vol.Type)
	}

	req := &RemoveRequest{
		Name: r.Name,
	}
	err = volDriver.Remove(req)
	if err != nil {
		seelog.Errorf("Volume %s removal failure: %v", r.Name, err)
		return err
	}

	// remove the volume information
	delete(a.volumes, r.Name)
	// cleanup the volume's host mount path
	err = a.CleanupMountPath(vol.Path)
	if err != nil {
		seelog.Errorf("Cleaning mount path failed for volume %s: %v", r.Name, err)
	}
	seelog.Infof("Saving state after removing volume %s", r.Name)
	// remove the state of deleted volume
	err = a.state.removeVolume(r.Name)
	if err != nil {
		seelog.Errorf("Error saving state after removing volume %s: %v", r.Name, err)
	}
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
	vol, ok := a.volumes[r.Name]
	if !ok {
		return nil, fmt.Errorf("volume %s not found", r.Name)
	}
	resp := &volume.Volume{
		Name:       r.Name,
		Mountpoint: vol.Path,
		CreatedAt:  vol.CreatedAt,
	}
	seelog.Infof("Returning volume information for %s", resp.Name)
	return &volume.GetResponse{Volume: resp}, nil
}

// Path implements Docker volume plugin's Path Method
func (a *AmazonECSVolumePlugin) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	a.lock.RLock()
	defer a.lock.RUnlock()
	vol, ok := a.volumes[r.Name]
	if !ok {
		seelog.Errorf("Could not find mount path for volume %s", r.Name)
		return nil, fmt.Errorf("volume %s not found", r.Name)
	}

	return &volume.PathResponse{Mountpoint: vol.Path}, nil
}

// Capabilities implements Docker volume plugin's Capabilities Method
func (a *AmazonECSVolumePlugin) Capabilities() *volume.CapabilitiesResponse {
	// Note: This is currently not supported
	return nil
}
