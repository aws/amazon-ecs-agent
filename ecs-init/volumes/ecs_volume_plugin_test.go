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
	"testing"

	"github.com/docker/go-plugins-helpers/volume"
	"github.com/stretchr/testify/assert"
)

// TestVolumeDriver implements VolumeDriver interface for testing
type TestVolumeDriver struct{}

func NewTestVolumeDriver() *TestVolumeDriver {
	return &TestVolumeDriver{}
}

func (t *TestVolumeDriver) Create(r *CreateRequest) error {
	return nil
}

func (t *TestVolumeDriver) Remove(r *RemoveRequest) error {
	return nil
}

func (t *TestVolumeDriver) Setup(n string, v *Volume) {
	return
}

// TestVolumeDriverError implements VolumeDriver interface for testing
// Returns error for all methods
type TestVolumeDriverError struct{}

func NewTestVolumeDriverError() *TestVolumeDriverError {
	return &TestVolumeDriverError{}
}

func (t *TestVolumeDriverError) Create(r *CreateRequest) error {
	return errors.New("create error")
}

func (t *TestVolumeDriverError) Remove(r *RemoveRequest) error {
	return errors.New("remove error")
}

func (t *TestVolumeDriverError) Setup(n string, v *Volume) {
	return
}

func TestVolumeCreateHappyPath(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{
			"efs": NewTestVolumeDriver(),
		},
		volumes: make(map[string]*Volume),
		state:   NewStateManager(),
	}
	req := &volume.CreateRequest{
		Name: "vol",
		Options: map[string]string{
			"type": "efs",
		},
	}
	createMountPath = func(path string) error {
		return nil
	}
	saveStateToDisk = func(b []byte) error {
		return nil
	}
	defer func() {
		createMountPath = createMountDir
		saveStateToDisk = saveState
	}()
	err := plugin.Create(req)
	assert.NoError(t, err, "create volume should be successful")
	assert.Len(t, plugin.volumes, 1)
	vol, ok := plugin.volumes["vol"]
	assert.True(t, ok)
	assert.Equal(t, "efs", vol.Type)
	assert.Equal(t, VolumeMountPathPrefix+"vol", vol.Path)
	assert.Len(t, plugin.state.VolState.Volumes, 1)
	volInfo, ok := plugin.state.VolState.Volumes["vol"]
	assert.True(t, ok)
	assert.Equal(t, "efs", volInfo.Type)
	assert.Equal(t, VolumeMountPathPrefix+"vol", volInfo.Path)
}

func TestVolumeCreateTargetSpecified(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{
			"efs": NewTestVolumeDriver(),
		},
		volumes: make(map[string]*Volume),
		state:   NewStateManager(),
	}
	req := &volume.CreateRequest{
		Name: "vol",
		Options: map[string]string{
			"type":   "efs",
			"target": "/foo",
		},
	}
	saveStateToDisk = func(b []byte) error {
		return nil
	}
	defer func() {
		saveStateToDisk = saveState
	}()
	err := plugin.Create(req)
	assert.NoError(t, err, "create volume should be successful")
	assert.Len(t, plugin.volumes, 1)
	vol, ok := plugin.volumes["vol"]
	assert.True(t, ok)
	assert.Equal(t, "efs", vol.Type)
	assert.Equal(t, "/foo", vol.Path)
	assert.Len(t, plugin.state.VolState.Volumes, 1)
	volInfo, ok := plugin.state.VolState.Volumes["vol"]
	assert.True(t, ok)
	assert.Equal(t, "efs", volInfo.Type)
	assert.Equal(t, "/foo", volInfo.Path)
}

func TestVolumeCreateSaveFailure(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{
			"efs": NewTestVolumeDriver(),
		},
		volumes: make(map[string]*Volume),
		state:   NewStateManager(),
	}
	req := &volume.CreateRequest{
		Name: "vol",
		Options: map[string]string{
			"type": "efs",
		},
	}
	createMountPath = func(path string) error {
		return nil
	}
	saveStateToDisk = func(b []byte) error {
		return errors.New("save to disk failure")
	}
	defer func() {
		createMountPath = createMountDir
		saveStateToDisk = saveState
	}()
	err := plugin.Create(req)
	assert.Error(t, err, "create volume failed due to state save failure")
	assert.Len(t, plugin.volumes, 1)
	vol, ok := plugin.volumes["vol"]
	assert.True(t, ok)
	assert.Equal(t, "efs", vol.Type)
	assert.Equal(t, VolumeMountPathPrefix+"vol", vol.Path)
}

func TestVolumeCreateFailure(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{
			"efs": NewTestVolumeDriverError(),
		},
		volumes: make(map[string]*Volume),
	}
	req := &volume.CreateRequest{
		Name: "vol",
		Options: map[string]string{
			"type": "efs",
		},
	}
	createMountPath = func(path string) error {
		return nil
	}
	defer func() {
		createMountPath = createMountDir
	}()
	assert.Error(t, plugin.Create(req), "expected error while creating volume")
	assert.Len(t, plugin.volumes, 0)
}

func TestCreateNoVolumeTypeFailure(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{
			"efs": NewTestVolumeDriver(),
		},
		volumes: make(map[string]*Volume),
	}
	req := &volume.CreateRequest{
		Name: "vol",
	}
	assert.Error(t, plugin.Create(req), "expected create error when no volume type specified")
	assert.Len(t, plugin.volumes, 0)
}

func TestCreateNoDriverFailure(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{},
		volumes:       make(map[string]*Volume),
	}
	req := &volume.CreateRequest{
		Name: "vol",
		Options: map[string]string{
			"type": "efs",
		},
	}
	assert.Error(t, plugin.Create(req), "expected create error when no corresponding volume driver present")
	assert.Len(t, plugin.volumes, 0)
}

func TestCreateMountCreationFailure(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{
			"efs": NewTestVolumeDriverError(),
		},
		volumes: make(map[string]*Volume),
	}
	req := &volume.CreateRequest{
		Name: "vol",
		Options: map[string]string{
			"type": "efs",
		},
	}
	createMountPath = func(path string) error {
		return errors.New("cannot create mount path")
	}
	defer func() {
		createMountPath = createMountDir
	}()
	assert.Error(t, plugin.Create(req), "expected create error when mount path cannot be created")
	assert.Len(t, plugin.volumes, 0)
}

func TestGetMountPathSuccess(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{},
		volumes:       make(map[string]*Volume),
	}
	createMountPath = func(path string) error {
		return nil
	}
	defer func() {
		createMountPath = createMountDir
	}()
	path, err := plugin.GetMountPath("vol")
	assert.NoError(t, err)
	assert.Equal(t, VolumeMountPathPrefix+"vol", path)
}

func TestGetMountPathFailure(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{},
		volumes:       make(map[string]*Volume),
	}
	createMountPath = func(path string) error {
		return errors.New("cannot create mount path")
	}
	defer func() {
		createMountPath = createMountDir
	}()
	path, err := plugin.GetMountPath("vol")
	assert.Error(t, err, "expected error when mount path cannot be created")
	assert.Empty(t, path)
}

func TestCleanMountPathSuccess(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{},
		volumes:       make(map[string]*Volume),
	}
	removeMountPath = func(path string) error {
		return nil
	}
	defer func() {
		removeMountPath = deleteMountPath
	}()
	assert.NoError(t, plugin.CleanupMountPath("vol"))
}

func TestCleanMountPathFailure(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{},
		volumes:       make(map[string]*Volume),
	}
	removeMountPath = func(path string) error {
		return errors.New("cannot remove dir")
	}
	defer func() {
		removeMountPath = deleteMountPath
	}()
	assert.Error(t, plugin.CleanupMountPath("vol"), "expected error when host mount path cannot be removed")
}

func TestVolumeMountSuccess(t *testing.T) {
	volName := "vol"
	path := VolumeMountPathPrefix + volName
	vol := &Volume{
		Path: path,
	}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{},
		volumes: map[string]*Volume{
			volName: vol,
		},
	}
	req := &volume.MountRequest{Name: volName}
	res, err := plugin.Mount(req)
	assert.NoError(t, err)
	assert.Equal(t, path, res.Mountpoint)
}

func TestVolumeMountFailure(t *testing.T) {
	volName := "vol"
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{},
		volumes:       map[string]*Volume{},
	}
	req := &volume.MountRequest{Name: volName}
	res, err := plugin.Mount(req)
	assert.Error(t, err)
	assert.Nil(t, res)
}

func TestVolumeUnmountSuccess(t *testing.T) {
	volName := "vol"
	path := VolumeMountPathPrefix + volName
	vol := &Volume{
		Path: path,
	}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{},
		volumes: map[string]*Volume{
			volName: vol,
		},
	}
	req := &volume.UnmountRequest{Name: volName}
	assert.NoError(t, plugin.Unmount(req))
}

func TestVolumeUnmountFailure(t *testing.T) {
	volName := "vol"
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{},
		volumes:       map[string]*Volume{},
	}
	req := &volume.UnmountRequest{Name: volName}
	assert.Error(t, plugin.Unmount(req), "expected error when volume to unmount is not present")
}

func TestVolumeRemoveHappyPath(t *testing.T) {
	volName := "vol"
	path := VolumeMountPathPrefix + volName
	vol := &Volume{
		Path: path,
		Type: "efs",
	}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{
			"efs": NewTestVolumeDriver(),
		},
		volumes: map[string]*Volume{
			volName: vol,
		},
		state: NewStateManager(),
	}
	req := &volume.RemoveRequest{Name: volName}
	removeMountPath = func(path string) error {
		return nil
	}
	saveStateToDisk = func(b []byte) error {
		return nil
	}
	defer func() {
		removeMountPath = deleteMountPath
		saveStateToDisk = saveState
	}()
	assert.NoError(t, plugin.Remove(req))
	assert.Len(t, plugin.volumes, 0)
	assert.Len(t, plugin.state.VolState.Volumes, 0)
}

func TestVolumeRemoveFailure(t *testing.T) {
	volName := "vol"
	path := VolumeMountPathPrefix + volName
	vol := &Volume{
		Path: path,
		Type: "efs",
	}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{
			"efs": NewTestVolumeDriverError(),
		},
		volumes: map[string]*Volume{
			volName: vol,
		},
		state: NewStateManager(),
	}
	saveStateToDisk = func(b []byte) error {
		return nil
	}
	defer func() {
		saveStateToDisk = saveState
	}()
	req := &volume.RemoveRequest{Name: volName}
	assert.Error(t, plugin.Remove(req), "expected error when remove volume fails")
	assert.Len(t, plugin.volumes, 1)
}

func TestRemoveVolumeNotFound(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{
			"efs": NewTestVolumeDriver(),
		},
		volumes: map[string]*Volume{},
		state:   NewStateManager(),
	}
	req := &volume.RemoveRequest{Name: "vol"}
	assert.Error(t, plugin.Remove(req), "expected error when volume to remove is not found")
}

func TestRemoveVolumeDriverNotFound(t *testing.T) {
	volName := "vol"
	path := VolumeMountPathPrefix + volName
	vol := &Volume{
		Path: path,
		Type: "efs",
	}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{
			"xyz": NewTestVolumeDriver(),
		},
		volumes: map[string]*Volume{
			volName: vol,
		},
		state: NewStateManager(),
	}
	req := &volume.RemoveRequest{Name: volName}
	assert.Error(t, plugin.Remove(req), "expected error when corresponding volume driver not found")
}

func TestListVolumes(t *testing.T) {
	vol := &Volume{}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{},
		volumes: map[string]*Volume{
			"vol":  vol,
			"vol1": vol,
			"vol2": vol,
		},
	}
	resp, err := plugin.List()
	assert.NoError(t, err)
	assert.Equal(t, 3, len(resp.Volumes))
}

func TestGetVolume(t *testing.T) {
	vol := &Volume{}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{},
		volumes: map[string]*Volume{
			"vol":  vol,
			"vol1": vol,
			"vol2": vol,
		},
	}
	req := &volume.GetRequest{Name: "vol1"}
	resp, err := plugin.Get(req)
	assert.NoError(t, err)
	assert.Equal(t, "vol1", resp.Volume.Name)
}

func TestGetVolumeError(t *testing.T) {
	vol := &Volume{}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{},
		volumes: map[string]*Volume{
			"vol":  vol,
			"vol1": vol,
			"vol2": vol,
		},
	}
	req := &volume.GetRequest{Name: "vol4"}
	_, err := plugin.Get(req)
	assert.Error(t, err, "expected error when volume info is not found")
}

func TestVolumePath(t *testing.T) {
	volName := "vol1"
	path := VolumeMountPathPrefix + volName
	vol1 := &Volume{
		Path: path,
	}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{},
		volumes: map[string]*Volume{
			"vol":   &Volume{},
			volName: vol1,
			"vol2":  &Volume{},
		},
	}
	req := &volume.PathRequest{Name: volName}
	resp, err := plugin.Path(req)
	assert.NoError(t, err)
	assert.Equal(t, path, resp.Mountpoint)
}

func TestVolumePathError(t *testing.T) {
	vol := &Volume{}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{},
		volumes: map[string]*Volume{
			"vol":  vol,
			"vol1": vol,
			"vol2": vol,
		},
	}
	req := &volume.PathRequest{Name: "vol4"}
	_, err := plugin.Path(req)
	assert.Error(t, err, "expected error when volume info is not found")
}

func TestCapabilities(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{}
	resp := plugin.Capabilities()
	assert.Nil(t, resp)
}

func TestPluginLoadState(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{
			"efs": NewECSVolumeDriver(),
		},
		volumes: make(map[string]*Volume),
		state:   NewStateManager(),
	}
	fileExists = func(path string) bool {
		return true
	}
	readStateFile = func() ([]byte, error) {
		return []byte(`{"volumes":{"efsVolume":{"type":"efs","path":"/var/lib/ecs/volumes/efsVolume","options":{"device":"fs-123","o":"tls","type":"efs"}}}}`), nil
	}
	defer func() {
		fileExists = checkFile
		readStateFile = readFile
	}()
	assert.NoError(t, plugin.LoadState(), "expected no error when loading state")
	assert.Len(t, plugin.volumes, 1)
	vol, ok := plugin.volumes["efsVolume"]
	assert.True(t, ok)
	assert.Equal(t, "efs", vol.Type)
	assert.Equal(t, VolumeMountPathPrefix+"efsVolume", vol.Path)
}

func TestPluginNoStateFile(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		state: NewStateManager(),
	}
	fileExists = func(path string) bool {
		return false
	}
	defer func() {
		fileExists = checkFile
	}()
	assert.NoError(t, plugin.LoadState())
}

func TestPluginInvalidState(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		state: NewStateManager(),
	}
	fileExists = func(path string) bool {
		return true
	}
	readStateFile = func() ([]byte, error) {
		return []byte(`{"junk"}`), nil
	}
	defer func() {
		fileExists = checkFile
		readStateFile = readFile
	}()
	assert.Error(t, plugin.LoadState(), "expected error when loading invalid state")
}

func TestPluginEmptyState(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]VolumeDriver{
			"efs": NewTestVolumeDriver(),
		},
		volumes: make(map[string]*Volume),
		state:   NewStateManager(),
	}
	fileExists = func(path string) bool {
		return true
	}
	readStateFile = func() ([]byte, error) {
		return []byte(`{}`), nil
	}
	defer func() {
		fileExists = checkFile
		readStateFile = readFile
	}()
	assert.NoError(t, plugin.LoadState(), "expected no error when loading empty state")
	assert.Len(t, plugin.volumes, 0)
	req := &volume.CreateRequest{
		Name: "vol",
		Options: map[string]string{
			"type": "efs",
		},
	}
	createMountPath = func(path string) error {
		return nil
	}
	saveStateToDisk = func(b []byte) error {
		return nil
	}
	defer func() {
		createMountPath = createMountDir
		saveStateToDisk = saveState
	}()
	err := plugin.Create(req)
	assert.NoError(t, err, "create volume should be successful after loading empty state")
	assert.Len(t, plugin.volumes, 1)
	vol, ok := plugin.volumes["vol"]
	assert.True(t, ok)
	assert.Equal(t, "efs", vol.Type)
	assert.Equal(t, VolumeMountPathPrefix+"vol", vol.Path)
	assert.Len(t, plugin.state.VolState.Volumes, 1)
	volInfo, ok := plugin.state.VolState.Volumes["vol"]
	assert.True(t, ok)
	assert.Equal(t, "efs", volInfo.Type)
	assert.Equal(t, VolumeMountPathPrefix+"vol", volInfo.Path)
}
