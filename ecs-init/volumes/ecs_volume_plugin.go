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

	"github.com/aws/amazon-ecs-agent/ecs-init/volumes/driver"
	"github.com/aws/amazon-ecs-agent/ecs-init/volumes/types"
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
	volumeDrivers map[string]driver.VolumeDriver
	volumes       map[string]*types.Volume
	state         *StateManager
	lock          sync.RWMutex
}

// NewAmazonECSVolumePlugin initiates the volume drivers
func NewAmazonECSVolumePlugin() *AmazonECSVolumePlugin {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{
			"efs": NewECSVolumeDriver(),
		},
		volumes: make(map[string]*types.Volume),
		state:   NewStateManager(),
	}
	return plugin
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

	// Reset volume mount reference count. This is for backwards-compatibility with old
	// state file format which did not have reference counting of volume mounts.
	for _, vol := range oldState.Volumes {
		for mountId, count := range vol.Mounts {
			if count == 0 {
				vol.Mounts[mountId] = 1
			}
		}
	}

	for volName, vol := range oldState.Volumes {
		voldriver, err := a.getVolumeDriver(vol.Type)
		if err != nil {
			seelog.Errorf("Could not load state: %v", err)
			return fmt.Errorf("could not load plugin state: %v", err)
		}
		volume := &types.Volume{
			Type:      vol.Type,
			Path:      vol.Path,
			Options:   vol.Options,
			CreatedAt: vol.CreatedAt,
			Mounts:    vol.Mounts,
		}
		a.volumes[volName] = volume
		voldriver.Setup(volName, volume)
	}
	a.state.VolState = oldState
	return nil
}

func (a *AmazonECSVolumePlugin) getVolumeDriver(driverType string) (driver.VolumeDriver, error) {
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

	vol := &types.Volume{
		Type:      driverType,
		Path:      target,
		Options:   r.Options,
		CreatedAt: time.Now().Format(time.RFC3339Nano),
		Mounts:    map[string]int{},
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
	seelog.Infof("Received mount request %+v", r)

	// Validate the request
	if len(r.Name) == 0 {
		return nil, fmt.Errorf("no volume in the request")
	}
	if len(r.ID) == 0 {
		return nil, fmt.Errorf("no mount ID in the request")
	}

	// Acquire write lock
	a.lock.Lock()
	defer a.lock.Unlock()

	// Find the volume
	vol, ok := a.volumes[r.Name]
	if !ok {
		seelog.Errorf("Volume %s to mount is not found", r.Name)
		return nil, fmt.Errorf("volume %s not found", r.Name)
	}

	// Find the volume driver
	volDriver, err := a.getVolumeDriver(vol.Type)
	if err != nil {
		seelog.Errorf("Volume %s's driver type %s not supported: %v", r.Name, vol.Type, err)
		return nil, fmt.Errorf("Volume %s's driver type %s not supported: %w", r.Name, vol.Type, err)
	}
	if volDriver == nil {
		// This case shouldn't happen normally
		return nil, fmt.Errorf("no volume driver found for type %s", vol.Type)
	}

	// Mount the volume on the host if there are no active mounts for the volume.
	if len(vol.Mounts) == 0 {
		seelog.Infof("Mounting volume %s as there are no existing mounts for it", r.Name)
		createReq := &driver.CreateRequest{Name: r.Name, Path: vol.Path, Options: vol.Options}
		if err := volDriver.Create(createReq); err != nil {
			seelog.Errorf("Volume %s creation failure: %v", r.Name, err)
			return nil, fmt.Errorf("failed to mount volume %s: %w", r.Name, err)
		}
		seelog.Infof("Volume %s mounted successfully", r.Name)
	}

	// Update state
	seelog.Infof("Adding mount %s to volume %s", r.ID, r.Name)
	vol.AddMount(r.ID)
	if err := a.state.recordVolume(r.Name, vol); err != nil {
		// State update failed, so roll back the changes made so far to make state consistent
		seelog.Errorf("Failed to save volume %s, rolling back changes: %v", r.Name, err)
		vol.RemoveMount(r.ID)
		if len(vol.Mounts) == 0 {
			seelog.Warnf("Rolling back mounting of volume %s", r.Name)
			if err := volDriver.Remove(&driver.RemoveRequest{Name: r.Name}); err != nil {
				seelog.Errorf("Volume %s removal failure: %v", r.Name, err)
			}
		}
		a.state.recordVolume(r.Name, vol)
		return nil, fmt.Errorf("mount failed due to an error while saving state: %w", err)
	}

	// All good
	return &volume.MountResponse{Mountpoint: vol.Path}, nil
}

// Unmount implements Docker volume plugin's Unmount Method
func (a *AmazonECSVolumePlugin) Unmount(r *volume.UnmountRequest) error {
	seelog.Infof("Received unmount request %+v", r)

	// Validate the request
	if len(r.Name) == 0 {
		return fmt.Errorf("no volume in the request")
	}
	if len(r.ID) == 0 {
		return fmt.Errorf("no mount ID in the request")
	}

	// Acquire write lock
	a.lock.Lock()
	defer a.lock.Unlock()

	// Find the volume
	vol, ok := a.volumes[r.Name]
	if !ok {
		seelog.Errorf("Volume %s to unmount is not found", r.Name)
		return fmt.Errorf("volume %s not found", r.Name)
	}

	// Get the corresponding volume driver
	volDriver, err := a.getVolumeDriver(vol.Type)
	if err != nil {
		seelog.Errorf("Volume %s removal failure: %v", r.Name, err)
		return fmt.Errorf("volume %v of type %s is unsupported: %w", r.Name, vol.Type, err)
	}
	if volDriver == nil {
		// this case should not happen normally
		return fmt.Errorf("no corresponding volume driver found for type %s", vol.Type)
	}

	// Remove the mount from the volume
	seelog.Infof("Removing mount %s from volume %s", r.ID, r.Name)
	if exists := vol.RemoveMount(r.ID); !exists {
		seelog.Warnf("Mount %s was not found on volume %s, this is a no-op", r.ID, r.Name)
		return nil
	}

	// If there are no more mounts left on the volume then unmount the volume from the host
	if len(vol.Mounts) == 0 {
		seelog.Infof("No active mounts left on volume %s, unmounting it", r.Name)
		if err := volDriver.Remove(&driver.RemoveRequest{Name: r.Name}); err != nil {
			seelog.Errorf("Failed to unmount volume %v: %v", r.Name, err)
			return fmt.Errorf("failed to unmount volume %v: %w", r.Name, err)
		}
	}

	// Save state
	if err := a.state.recordVolume(r.Name, vol); err != nil {
		// State save failed, so roll back the changes made so far to make state consistent
		seelog.Errorf("Error saving state of volume %s", r.Name, err)
	}

	// All good
	return nil
}

// Remove implements Docker volume plugin's Remove Method
func (a *AmazonECSVolumePlugin) Remove(r *volume.RemoveRequest) error {
	seelog.Infof("Received Remove request %+v", r)

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

	// Although unmounts are handled by Unmount method, unmount the volume if it's still
	// mounted. This is mainly to unmount volumes created by an older version of the
	// plugin in which unmounts were not handled by Unmount method.
	if volDriver.IsMounted(r.Name) {
		seelog.Infof("Volume %s is currently mounted, unmouting it", r.Name)
		if err := volDriver.Remove(&driver.RemoveRequest{Name: r.Name}); err != nil {
			seelog.Errorf("Volume %s removal failure: %v", r.Name, err)
			return err
		}
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
