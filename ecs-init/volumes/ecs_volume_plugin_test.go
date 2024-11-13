// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
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

	"github.com/aws/amazon-ecs-agent/ecs-init/volumes/driver"
	mock_driver "github.com/aws/amazon-ecs-agent/ecs-init/volumes/driver/mock"
	"github.com/aws/amazon-ecs-agent/ecs-init/volumes/types"
	"github.com/docker/go-plugins-helpers/volume"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVolumeDriver implements VolumeDriver interface for testing
type TestVolumeDriver struct{}

func NewTestVolumeDriver() *TestVolumeDriver {
	return &TestVolumeDriver{}
}

func (t *TestVolumeDriver) Create(r *driver.CreateRequest) error {
	return nil
}

func (t *TestVolumeDriver) Remove(r *driver.RemoveRequest) error {
	return nil
}

func (t *TestVolumeDriver) Setup(n string, v *types.Volume) {
	return
}

func (t *TestVolumeDriver) IsMounted(volumeName string) bool {
	return false
}

// TestVolumeDriverError implements VolumeDriver interface for testing
// Returns error for all methods
type TestVolumeDriverError struct{}

func NewTestVolumeDriverError() *TestVolumeDriverError {
	return &TestVolumeDriverError{}
}

func (t *TestVolumeDriverError) Create(r *driver.CreateRequest) error {
	return errors.New("create error")
}

func (t *TestVolumeDriverError) Remove(r *driver.RemoveRequest) error {
	return errors.New("remove error")
}

func (t *TestVolumeDriverError) Setup(n string, v *types.Volume) {
	return
}

func (t *TestVolumeDriverError) IsMounted(volumeName string) bool {
	return false
}

func TestVolumeCreateHappyPath(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{
			"efs": NewTestVolumeDriver(),
		},
		volumes: make(map[string]*types.Volume),
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
	assert.NotEmpty(t, vol.CreatedAt)
	assert.Len(t, plugin.state.VolState.Volumes, 1)
	volInfo, ok := plugin.state.VolState.Volumes["vol"]
	assert.True(t, ok)
	assert.Equal(t, "efs", volInfo.Type)
	assert.Equal(t, VolumeMountPathPrefix+"vol", volInfo.Path)
	assert.Equal(t, vol.CreatedAt, volInfo.CreatedAt)
}

func TestVolumeCreateTargetSpecified(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{
			"efs": NewTestVolumeDriver(),
		},
		volumes: make(map[string]*types.Volume),
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
		volumeDrivers: map[string]driver.VolumeDriver{
			"efs": NewTestVolumeDriver(),
		},
		volumes: make(map[string]*types.Volume),
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
	assert.NoError(t, err, "create volume successful but save state failed")
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

func TestVolumeCreateFailure(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{
			"efs": NewTestVolumeDriverError(),
		},
		volumes: make(map[string]*types.Volume),
	}
	req := &volume.CreateRequest{
		Name: "vol",
		Options: map[string]string{
			"type": "unknown",
		},
	}
	assert.Error(t, plugin.Create(req), "expected error while creating volume")
	assert.Len(t, plugin.volumes, 0)
}

func TestCreateNoVolumeType(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{
			"efs": NewTestVolumeDriver(),
		},
		volumes: make(map[string]*types.Volume),
		state:   NewStateManager(),
	}
	req := &volume.CreateRequest{
		Name: "vol",
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
	assert.NoError(t, plugin.Create(req), "expected no create error when no volume type specified")
	assert.Len(t, plugin.volumes, 1)
}

func TestCreateNoDriverFailure(t *testing.T) {
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{},
		volumes:       make(map[string]*types.Volume),
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
		volumeDrivers: map[string]driver.VolumeDriver{
			"efs": NewTestVolumeDriverError(),
		},
		volumes: make(map[string]*types.Volume),
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
		volumeDrivers: map[string]driver.VolumeDriver{},
		volumes:       make(map[string]*types.Volume),
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
		volumeDrivers: map[string]driver.VolumeDriver{},
		volumes:       make(map[string]*types.Volume),
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
		volumeDrivers: map[string]driver.VolumeDriver{},
		volumes:       make(map[string]*types.Volume),
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
		volumeDrivers: map[string]driver.VolumeDriver{},
		volumes:       make(map[string]*types.Volume),
	}
	removeMountPath = func(path string) error {
		return errors.New("cannot remove dir")
	}
	defer func() {
		removeMountPath = deleteMountPath
	}()
	assert.Error(t, plugin.CleanupMountPath("vol"), "expected error when host mount path cannot be removed")
}

func TestVolumeRemoveHappyPath(t *testing.T) {
	volName := "vol"
	path := VolumeMountPathPrefix + volName
	vol := &types.Volume{
		Path: path,
		Type: "efs",
	}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{
			"efs": NewTestVolumeDriver(),
		},
		volumes: map[string]*types.Volume{
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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	volName := "vol"
	path := VolumeMountPathPrefix + volName
	vol := &types.Volume{
		Path: path,
		Type: "efs",
	}
	efsDriver := mock_driver.NewMockVolumeDriver(ctrl)
	efsDriver.EXPECT().IsMounted(volName).Return(true)
	efsDriver.EXPECT().Remove(&driver.RemoveRequest{Name: volName}).Return(errors.New("error"))
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{
			"efs": efsDriver,
		},
		volumes: map[string]*types.Volume{
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
		volumeDrivers: map[string]driver.VolumeDriver{
			"efs": NewTestVolumeDriver(),
		},
		volumes: map[string]*types.Volume{},
		state:   NewStateManager(),
	}
	req := &volume.RemoveRequest{Name: "vol"}
	assert.Error(t, plugin.Remove(req), "expected error when volume to remove is not found")
}

func TestRemoveVolumeDriverNotFound(t *testing.T) {
	volName := "vol"
	path := VolumeMountPathPrefix + volName
	vol := &types.Volume{
		Path: path,
		Type: "efs",
	}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{
			"xyz": NewTestVolumeDriver(),
		},
		volumes: map[string]*types.Volume{
			volName: vol,
		},
		state: NewStateManager(),
	}
	req := &volume.RemoveRequest{Name: volName}
	assert.Error(t, plugin.Remove(req), "expected error when corresponding volume driver not found")
}

// Tests that Remove method does not attempt to unmount a volume that's already unmounted.
func TestVolumeRemoveNoUnmountIfAlreadyUnmounted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	volName := "vol"
	path := VolumeMountPathPrefix + volName
	vol := &types.Volume{
		Path: path,
		Type: "efs",
	}
	efsDriver := mock_driver.NewMockVolumeDriver(ctrl)
	efsDriver.EXPECT().IsMounted(volName).Return(false) // Not mounted
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{
			"efs": efsDriver,
		},
		volumes: map[string]*types.Volume{
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
	assert.NoError(t, plugin.Remove(req))
	assert.Len(t, plugin.volumes, 0)
}

func TestVolumeRemoveMountPathFailure(t *testing.T) {
	volName := "vol"
	path := VolumeMountPathPrefix + volName
	vol := &types.Volume{
		Path: path,
		Type: "efs",
	}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{
			"efs": NewTestVolumeDriver(),
		},
		volumes: map[string]*types.Volume{
			volName: vol,
		},
		state: NewStateManager(),
	}
	req := &volume.RemoveRequest{Name: volName}
	removeMountPath = func(path string) error {
		return errors.New("removing path failed")
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

func TestVolumeRemoveStateSaveFailure(t *testing.T) {
	volName := "vol"
	path := VolumeMountPathPrefix + volName
	vol := &types.Volume{
		Path: path,
		Type: "efs",
	}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{
			"efs": NewTestVolumeDriver(),
		},
		volumes: map[string]*types.Volume{
			volName: vol,
		},
		state: NewStateManager(),
	}
	req := &volume.RemoveRequest{Name: volName}
	removeMountPath = func(path string) error {
		return nil
	}
	saveStateToDisk = func(b []byte) error {
		return errors.New("save to disk failed")
	}
	defer func() {
		removeMountPath = deleteMountPath
		saveStateToDisk = saveState
	}()
	assert.NoError(t, plugin.Remove(req))
	assert.Len(t, plugin.volumes, 0)
	assert.Len(t, plugin.state.VolState.Volumes, 0)
}

func TestListVolumes(t *testing.T) {
	vol := &types.Volume{}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{},
		volumes: map[string]*types.Volume{
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
	vol := &types.Volume{
		Path:      "/var/lib/ecs/volume/vol",
		CreatedAt: "2020-01-17T21:20:04Z",
	}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{},
		volumes: map[string]*types.Volume{
			"vol":  vol,
			"vol1": vol,
			"vol2": vol,
		},
	}
	req := &volume.GetRequest{Name: "vol1"}
	resp, err := plugin.Get(req)
	assert.NoError(t, err)
	assert.Equal(t, "vol1", resp.Volume.Name)
	assert.Equal(t, vol.Path, resp.Volume.Mountpoint)
	assert.Equal(t, vol.CreatedAt, resp.Volume.CreatedAt)
}

func TestGetVolumeError(t *testing.T) {
	vol := &types.Volume{}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{},
		volumes: map[string]*types.Volume{
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
	vol1 := &types.Volume{
		Path: path,
	}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{},
		volumes: map[string]*types.Volume{
			"vol":   {},
			volName: vol1,
			"vol2":  {},
		},
	}
	req := &volume.PathRequest{Name: volName}
	resp, err := plugin.Path(req)
	assert.NoError(t, err)
	assert.Equal(t, path, resp.Mountpoint)
}

func TestVolumePathError(t *testing.T) {
	vol := &types.Volume{}
	plugin := &AmazonECSVolumePlugin{
		volumeDrivers: map[string]driver.VolumeDriver{},
		volumes: map[string]*types.Volume{
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
	tcs := []struct {
		name              string
		stateFileContents string
		pluginAssertions  func(*testing.T, *AmazonECSVolumePlugin)
	}{
		{
			name: "backwards compatibility with state format without reference counting of mounts",
			stateFileContents: `
            {
                "volumes": {
                    "efsVolume": {
                        "type":"efs",
                        "path":"/var/lib/ecs/volumes/efsVolume",
                        "options": {"device":"fs-123","o":"tls","type":"efs"},
                        "mounts": {"id1": null}
                    }
                }
            }`,
			pluginAssertions: func(t *testing.T, plugin *AmazonECSVolumePlugin) {
				assert.Len(t, plugin.volumes, 1)
				vol, ok := plugin.volumes["efsVolume"]
				assert.True(t, ok)
				assert.Equal(t, "efs", vol.Type)
				assert.Equal(t, VolumeMountPathPrefix+"efsVolume", vol.Path)
				vols := plugin.state.VolState.Volumes
				assert.Len(t, vols, 1)
				volInfo, ok := vols["efsVolume"]
				require.True(t, ok)
				assert.Equal(t, "efs", volInfo.Type)
				assert.Equal(t, VolumeMountPathPrefix+"efsVolume", volInfo.Path)

				// Test for backwards compatibility of old state format following implementation of
				// reference counting of volume mounts null value for mount IDs should be converted to 1.
				assert.Equal(t, map[string]int{"id1": 1}, vols["efsVolume"].Mounts)
			},
		},
		{
			name: "current state format",
			stateFileContents: `
            {
                "volumes": {
                    "efsVolume": {
                        "type":"efs",
                        "path":"/var/lib/ecs/volumes/efsVolume",
                        "options": {"device":"fs-123","o":"tls","type":"efs"},
                        "mounts": {"id1": 1, "id2": 2}
                    }
                }
            }`,
			pluginAssertions: func(t *testing.T, plugin *AmazonECSVolumePlugin) {
				assert.Len(t, plugin.volumes, 1)
				vol, ok := plugin.volumes["efsVolume"]
				assert.True(t, ok)
				assert.Equal(t, "efs", vol.Type)
				assert.Equal(t, VolumeMountPathPrefix+"efsVolume", vol.Path)
				vols := plugin.state.VolState.Volumes
				assert.Len(t, vols, 1)
				volInfo, ok := vols["efsVolume"]
				require.True(t, ok)
				assert.Equal(t, "efs", volInfo.Type)
				assert.Equal(t, VolumeMountPathPrefix+"efsVolume", volInfo.Path)
				assert.Equal(t, map[string]int{"id1": 1, "id2": 2}, vols["efsVolume"].Mounts)
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &AmazonECSVolumePlugin{
				volumeDrivers: map[string]driver.VolumeDriver{
					"efs": NewECSVolumeDriver(),
				},
				volumes: make(map[string]*types.Volume),
				state:   NewStateManager(),
			}
			fileExists = func(path string) bool {
				return true
			}
			readStateFile = func() ([]byte, error) {
				return []byte(tc.stateFileContents), nil
			}
			defer func() {
				fileExists = checkFile
				readStateFile = readFile
			}()
			assert.NoError(t, plugin.LoadState(), "expected no error when loading state")
			tc.pluginAssertions(t, plugin)
		})
	}
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
		volumeDrivers: map[string]driver.VolumeDriver{
			"efs": NewTestVolumeDriver(),
		},
		volumes: make(map[string]*types.Volume),
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

// Tests for plugin's Mount method.
func TestPluginMount(t *testing.T) {
	const (
		volName       = "volume"
		volPath       = "path"
		driverTypeEFS = "efs"
		reqMountID    = "mountID"
	)
	volOpts := map[string]string{"opt1": "opt1"}

	tcs := []struct {
		name                  string
		pluginVolumes         map[string]*types.Volume
		setDriverExpectations func(d *mock_driver.MockVolumeDriver)
		mockSaveStateFn       func(b []byte) error
		req                   *volume.MountRequest
		expectedResponse      *volume.MountResponse
		expectedError         string
		assertPluginState     func(t *testing.T, plugin *AmazonECSVolumePlugin)
	}{
		{
			name:          "volume not found",
			req:           &volume.MountRequest{Name: "unknown", ID: reqMountID},
			expectedError: "volume unknown not found",
		},
		{
			name: "first mount on the volume",
			setDriverExpectations: func(d *mock_driver.MockVolumeDriver) {
				d.EXPECT().
					Create(&driver.CreateRequest{Name: volName, Path: volPath, Options: volOpts}).
					Return(nil)
			},
			pluginVolumes:    map[string]*types.Volume{volName: {Path: volPath, Options: volOpts}},
			req:              &volume.MountRequest{Name: volName, ID: reqMountID},
			expectedResponse: &volume.MountResponse{Mountpoint: volPath},
			assertPluginState: func(t *testing.T, plugin *AmazonECSVolumePlugin) {
				mounts := map[string]int{reqMountID: 1}
				assert.Equal(t,
					map[string]*types.Volume{
						volName: {Path: volPath, Options: volOpts, Mounts: mounts},
					},
					plugin.volumes)
				assert.Equal(t,
					&VolumeState{
						Volumes: map[string]*VolumeInfo{
							volName: {Path: volPath, Options: volOpts, Mounts: mounts},
						},
					},
					plugin.state.VolState)
			},
		},
		{
			name: "volume already has mounts - no interaction with driver",
			pluginVolumes: map[string]*types.Volume{
				volName: {
					Path:   volPath,
					Mounts: map[string]int{"someMount": 1},
				},
			},
			req:              &volume.MountRequest{Name: volName, ID: reqMountID},
			expectedResponse: &volume.MountResponse{Mountpoint: volPath},
			assertPluginState: func(t *testing.T, plugin *AmazonECSVolumePlugin) {
				mounts := map[string]int{reqMountID: 1, "someMount": 1}
				assert.Equal(t,
					map[string]*types.Volume{volName: {Path: volPath, Mounts: mounts}},
					plugin.volumes)
				assert.Equal(t,
					&VolumeState{
						Volumes: map[string]*VolumeInfo{volName: {Path: volPath, Mounts: mounts}},
					},
					plugin.state.VolState)
			},
		},
		{
			name:          "invalid driver type",
			pluginVolumes: map[string]*types.Volume{volName: {Path: volPath, Type: "unknown"}},
			req:           &volume.MountRequest{Name: volName, ID: reqMountID},
			expectedError: "Volume volume's driver type unknown not supported: volume unknown type not supported",
		},
		{
			name:          "no ID in the request",
			req:           &volume.MountRequest{Name: volName},
			expectedError: "no mount ID in the request",
		},
		{
			name:          "no volume in the request",
			req:           &volume.MountRequest{},
			expectedError: "no volume in the request",
		},
		{
			name: "driver fails to mount",
			setDriverExpectations: func(d *mock_driver.MockVolumeDriver) {
				d.EXPECT().
					Create(&driver.CreateRequest{Name: volName, Path: volPath}).
					Return(errors.New("some error"))
			},
			pluginVolumes: map[string]*types.Volume{volName: {Path: volPath}},
			req:           &volume.MountRequest{Name: volName, ID: reqMountID},
			expectedError: "failed to mount volume volume: some error",
			assertPluginState: func(t *testing.T, plugin *AmazonECSVolumePlugin) {
				// No mounts expected on the volume
				assert.Equal(t, map[string]*types.Volume{volName: {Path: volPath}}, plugin.volumes)
				assert.Equal(t,
					&VolumeState{Volumes: map[string]*VolumeInfo{
						volName: {Path: volPath, Mounts: map[string]int{}},
					}},
					plugin.state.VolState)
			},
		},
		{
			name: "duplicate mount increments mount reference count",
			pluginVolumes: map[string]*types.Volume{
				volName: {
					Path:    volPath,
					Mounts:  map[string]int{reqMountID: 1},
					Options: volOpts,
				},
			},
			req:              &volume.MountRequest{Name: volName, ID: reqMountID},
			expectedResponse: &volume.MountResponse{Mountpoint: volPath},
			assertPluginState: func(t *testing.T, plugin *AmazonECSVolumePlugin) {
				mounts := map[string]int{reqMountID: 2}
				assert.Equal(t,
					map[string]*types.Volume{
						volName: {Path: volPath, Options: volOpts, Mounts: mounts},
					},
					plugin.volumes)
				assert.Equal(t,
					&VolumeState{
						Volumes: map[string]*VolumeInfo{
							volName: {Path: volPath, Options: volOpts, Mounts: mounts},
						},
					},
					plugin.state.VolState)
			},
		},
		{
			name: "roll back changes if saving state fails",
			setDriverExpectations: func(d *mock_driver.MockVolumeDriver) {
				d.EXPECT().Create(&driver.CreateRequest{Name: volName, Path: volPath}).Return(nil)
				d.EXPECT().Remove(&driver.RemoveRequest{Name: volName}).Return(nil) // mount rollback
			},
			pluginVolumes:   map[string]*types.Volume{volName: {Path: volPath}},
			mockSaveStateFn: func(b []byte) error { return errors.New("some error") },
			req:             &volume.MountRequest{Name: volName, ID: reqMountID},
			expectedError:   "mount failed due to an error while saving state: some error",
			assertPluginState: func(t *testing.T, plugin *AmazonECSVolumePlugin) {
				// No mounts expected on the volume
				mounts := map[string]int{}
				assert.Equal(t,
					map[string]*types.Volume{volName: {Path: volPath, Mounts: mounts}},
					plugin.volumes)
				assert.Equal(t,
					&VolumeState{
						Volumes: map[string]*VolumeInfo{volName: {Path: volPath, Mounts: mounts}},
					},
					plugin.state.VolState)
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Prepare a mock driver
			efsDriver := mock_driver.NewMockVolumeDriver(ctrl)
			if tc.setDriverExpectations != nil {
				tc.setDriverExpectations(efsDriver)
			}

			// Mock saveState function for preparing plugin for testing
			saveStateToDisk = func(b []byte) error {
				return nil
			}
			defer func() {
				saveStateToDisk = saveState
			}()

			// Prepare a plugin for testing with state loaded
			pluginState := NewStateManager()
			plugin := AmazonECSVolumePlugin{
				volumes:       tc.pluginVolumes,
				volumeDrivers: map[string]driver.VolumeDriver{driverTypeEFS: efsDriver},
				state:         pluginState,
			}
			for volName, vol := range tc.pluginVolumes {
				pluginState.recordVolume(volName, vol)
			}

			// Mock saveState function for the test case
			if tc.mockSaveStateFn != nil {
				saveStateToDisk = tc.mockSaveStateFn
			}

			// Test
			res, err := plugin.Mount(tc.req)
			if tc.expectedError == "" {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedResponse, res)
			} else {
				assert.EqualError(t, err, tc.expectedError)
			}
			if tc.assertPluginState != nil {
				tc.assertPluginState(t, &plugin)
			}
		})
	}
}

// Tests for plugin's Unmount method.
func TestPluginUnmount(t *testing.T) {
	const (
		volName       = "volume"
		volPath       = "path"
		driverTypeEFS = "efs"
		reqMountID    = "mountID"
	)
	volOpts := map[string]string{"opt1": "opt1"}

	tcs := []struct {
		name                  string
		pluginVolumes         map[string]*types.Volume
		setDriverExpectations func(d *mock_driver.MockVolumeDriver)
		mockSaveStateFn       func(b []byte) error
		req                   *volume.UnmountRequest
		expectedError         string
		assertPluginState     func(t *testing.T, plugin *AmazonECSVolumePlugin)
	}{
		{
			name:          "volume not found",
			req:           &volume.UnmountRequest{Name: "unknown", ID: reqMountID},
			expectedError: "volume unknown not found",
		},
		{
			name: "only one mount on the volume",
			setDriverExpectations: func(d *mock_driver.MockVolumeDriver) {
				d.EXPECT().Remove(&driver.RemoveRequest{Name: volName}).Return(nil)
			},
			pluginVolumes: map[string]*types.Volume{
				volName: {Path: volPath, Mounts: map[string]int{reqMountID: 1}},
			},
			req: &volume.UnmountRequest{Name: volName, ID: reqMountID},
			assertPluginState: func(t *testing.T, plugin *AmazonECSVolumePlugin) {
				mounts := map[string]int{}
				assert.Equal(t,
					map[string]*types.Volume{volName: {Path: volPath, Mounts: mounts}},
					plugin.volumes)
				assert.Equal(t,
					&VolumeState{
						Volumes: map[string]*VolumeInfo{volName: {Path: volPath, Mounts: mounts}},
					},
					plugin.state.VolState)
			},
		},
		{
			name: "more than one mount on the volume - no interaction with the driver",
			pluginVolumes: map[string]*types.Volume{
				volName: {
					Path:   volPath,
					Mounts: map[string]int{"someMount": 1, reqMountID: 1},
				},
			},
			req: &volume.UnmountRequest{Name: volName, ID: reqMountID},
			assertPluginState: func(t *testing.T, plugin *AmazonECSVolumePlugin) {
				mounts := map[string]int{"someMount": 1}
				assert.Equal(t,
					map[string]*types.Volume{volName: {Path: volPath, Mounts: mounts}},
					plugin.volumes)
				assert.Equal(t,
					&VolumeState{
						Volumes: map[string]*VolumeInfo{volName: {Path: volPath, Mounts: mounts}},
					},
					plugin.state.VolState)
			},
		},
		{
			name: "mount reference count decrements",
			pluginVolumes: map[string]*types.Volume{
				volName: {
					Path:   volPath,
					Mounts: map[string]int{reqMountID: 2},
				},
			},
			req: &volume.UnmountRequest{Name: volName, ID: reqMountID},
			assertPluginState: func(t *testing.T, plugin *AmazonECSVolumePlugin) {
				mounts := map[string]int{reqMountID: 1}
				assert.Equal(t,
					map[string]*types.Volume{volName: {Path: volPath, Mounts: mounts}},
					plugin.volumes)
				assert.Equal(t,
					&VolumeState{
						Volumes: map[string]*VolumeInfo{volName: {Path: volPath, Mounts: mounts}},
					},
					plugin.state.VolState)
			},
		},
		{
			name:          "invalid driver type",
			pluginVolumes: map[string]*types.Volume{volName: {Path: volPath, Type: "unknown"}},
			req:           &volume.UnmountRequest{Name: volName, ID: reqMountID},
			expectedError: "volume volume of type unknown is unsupported: volume unknown type not supported",
		},
		{
			name:          "no ID in the request",
			req:           &volume.UnmountRequest{Name: volName},
			expectedError: "no mount ID in the request",
		},
		{
			name:          "no volume in the request",
			req:           &volume.UnmountRequest{},
			expectedError: "no volume in the request",
		},
		{
			name: "driver fails to unmount",
			setDriverExpectations: func(d *mock_driver.MockVolumeDriver) {
				d.EXPECT().
					Remove(&driver.RemoveRequest{Name: volName}).
					Return(errors.New("some error"))
			},
			pluginVolumes: map[string]*types.Volume{
				volName: {Path: volPath, Mounts: map[string]int{reqMountID: 1}},
			},
			req:           &volume.UnmountRequest{Name: volName, ID: reqMountID},
			expectedError: "failed to unmount volume volume: some error",
			assertPluginState: func(t *testing.T, plugin *AmazonECSVolumePlugin) {
				// Mount should not exist in the plugin state
				mounts := map[string]int{}
				assert.Equal(t,
					map[string]*types.Volume{volName: {Path: volPath, Mounts: mounts}},
					plugin.volumes)
			},
		},
		{
			name:          "no-op when mount not found on the volume",
			pluginVolumes: map[string]*types.Volume{volName: {Path: volPath, Options: volOpts}},
			req:           &volume.UnmountRequest{Name: volName, ID: reqMountID},
			assertPluginState: func(t *testing.T, plugin *AmazonECSVolumePlugin) {
				mounts := map[string]int{}
				assert.Equal(t,
					map[string]*types.Volume{
						volName: {Path: volPath, Mounts: nil, Options: volOpts},
					},
					plugin.volumes)
				assert.Equal(t,
					&VolumeState{
						Volumes: map[string]*VolumeInfo{
							volName: {Path: volPath, Mounts: mounts, Options: volOpts},
						},
					},
					plugin.state.VolState)
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Prepare a mock driver
			efsDriver := mock_driver.NewMockVolumeDriver(ctrl)
			if tc.setDriverExpectations != nil {
				tc.setDriverExpectations(efsDriver)
			}

			// Mock saveState function for preparing plugin for testing
			saveStateToDisk = func(b []byte) error {
				return nil
			}
			defer func() {
				saveStateToDisk = saveState
			}()

			// Prepare a plugin for testing with state loaded
			pluginState := NewStateManager()
			plugin := AmazonECSVolumePlugin{
				volumes:       tc.pluginVolumes,
				volumeDrivers: map[string]driver.VolumeDriver{driverTypeEFS: efsDriver},
				state:         pluginState,
			}
			for volName, vol := range tc.pluginVolumes {
				pluginState.recordVolume(volName, vol)
			}

			// Mock saveState function from the test case
			if tc.mockSaveStateFn != nil {
				saveStateToDisk = tc.mockSaveStateFn
			}

			// Test
			err := plugin.Unmount(tc.req)
			if tc.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.expectedError)
			}
			if tc.assertPluginState != nil {
				tc.assertPluginState(t, &plugin)
			}
		})
	}
}
