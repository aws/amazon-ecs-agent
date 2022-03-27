//go:build windows && unit
// +build windows,unit

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
package statemanager

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/config"
	mock_dependencies "github.com/aws/amazon-ecs-agent/agent/statemanager/dependencies/mocks"
	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper"
	"github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"
	mock_oswrapper "github.com/aws/amazon-ecs-agent/agent/utils/oswrapper/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows/registry"
)

var (
	testError = errors.New("test error")

	mockNameImpl = func() string {
		return `C:\new.json`
	}
	mockSyncImpl = func() error {
		return nil
	}
)

func mockTempFileError() func() {
	otempFile := tempFile
	tempFile = func(dir, pattern string) (oswrapper.File, error) {
		return nil, testError
	}

	return func() {
		tempFile = otempFile
	}
}

func mockOpenError() func() {
	tempOpen := open
	open = func(name string) (oswrapper.File, error) {
		return nil, testError
	}

	return func() {
		open = tempOpen
	}
}

func setup(t *testing.T, options ...Option) (
	*mock_dependencies.MockWindowsRegistry,
	*mock_dependencies.MockRegistryKey,
	oswrapper.File,
	StateManager) {
	ctrl := gomock.NewController(t)
	mockRegistry := mock_dependencies.NewMockWindowsRegistry(ctrl)
	mockKey := mock_dependencies.NewMockRegistryKey(ctrl)
	mockFile := mocks.NewMockFile()

	// TODO set this to platform-specific tmpdir
	tmpDir := t.TempDir()
	t.Cleanup(func() {
		ctrl.Finish()
	})
	cfg := &config.Config{DataDir: tmpDir}
	manager, err := NewStateManager(cfg, options...)
	assert.Nil(t, err)
	basicManager := manager.(*basicStateManager)
	basicManager.platformDependencies = windowsDependencies{
		registry: mockRegistry,
	}
	return mockRegistry, mockKey, mockFile, basicManager
}

func TestStateManagerLoadNoRegistryKey(t *testing.T) {
	mockRegistry, _, _, manager := setup(t)

	// TODO figure out why gomock does not like registry.READ as the mode
	mockRegistry.EXPECT().OpenKey(ecsDataFileRootKey, ecsDataFileKeyPath, gomock.Any()).Return(nil, registry.ErrNotExist)

	err := manager.Load()
	assert.Nil(t, err, "Expected loading a non-existent file to not be an error")
}

func TestStateManagerLoadNoFile(t *testing.T) {
	mockRegistry, mockKey, _, manager := setup(t)

	defer mockOpenError()()
	isNotExist = func(err error) bool {
		return true
	}
	defer func() {
		isNotExist = os.IsNotExist
	}()

	// TODO figure out why gomock does not like registry.READ as the mode
	mockRegistry.EXPECT().OpenKey(ecsDataFileRootKey, ecsDataFileKeyPath, gomock.Any()).Return(mockKey, nil)
	mockKey.EXPECT().GetStringValue(ecsDataFileValueName).Return(`C:\data.json`, uint32(0), nil)
	mockKey.EXPECT().Close()

	err := manager.Load()
	assert.Nil(t, err, "Expected loading a non-existent file to not be an error")
}

func TestStateManagerLoadError(t *testing.T) {
	mockRegistry, mockKey, _, manager := setup(t)

	defer mockOpenError()()
	isNotExist = func(err error) bool {
		return false
	}
	defer func() {
		isNotExist = os.IsNotExist
	}()

	// TODO figure out why gomock does not like registry.READ as the mode
	mockRegistry.EXPECT().OpenKey(ecsDataFileRootKey, ecsDataFileKeyPath, gomock.Any()).Return(mockKey, nil)
	mockKey.EXPECT().GetStringValue(ecsDataFileValueName).Return(`C:\data.json`, uint32(0), nil)
	mockKey.EXPECT().Close()

	err := manager.Load()
	assert.Equal(t, testError, err, "Expected error opening file to be an error")
}

func TestStateManagerLoadState(t *testing.T) {
	containerInstanceArn := ""
	mockRegistry, mockKey, mockFile, manager := setup(t, AddSaveable("ContainerInstanceArn", &containerInstanceArn))

	data := `{"Version":1,"Data":{"ContainerInstanceArn":"foo"}}`
	// TODO figure out why gomock does not like registry.READ as the mode
	mockRegistry.EXPECT().OpenKey(ecsDataFileRootKey, ecsDataFileKeyPath, gomock.Any()).Return(mockKey, nil)
	mockKey.EXPECT().GetStringValue(ecsDataFileValueName).Return(`C:\data.json`, uint32(0), nil)
	mockKey.EXPECT().Close()

	tempOpen := open
	open = func(name string) (oswrapper.File, error) {
		return mockFile, nil
	}
	readAll = func(r io.Reader) ([]byte, error) {
		return []byte(data), nil
	}
	defer func() {
		open = tempOpen
		readAll = ioutil.ReadAll
	}()

	err := manager.Load()
	assert.Nil(t, err, "Expected loading correctly")
	assert.Equal(t, "foo", containerInstanceArn)
}

func TestStateManagerLoadV1Data(t *testing.T) {
	var containerInstanceArn, cluster, savedInstanceID string
	var sequenceNumber int64
	mockRegistry, mockKey, _, manager := setup(t,
		AddSaveable("ContainerInstanceArn", &containerInstanceArn),
		AddSaveable("Cluster", &cluster),
		AddSaveable("EC2InstanceID", &savedInstanceID),
		AddSaveable("SeqNum", &sequenceNumber))
	dataFile, err := os.Open(filepath.Join(".", "testdata", "v1", "1", ecsDataFile))
	assert.Nil(t, err, "Error opening test data")
	defer dataFile.Close()

	// TODO figure out why gomock does not like registry.READ as the mode
	mockRegistry.EXPECT().OpenKey(ecsDataFileRootKey, ecsDataFileKeyPath, gomock.Any()).Return(mockKey, nil)
	mockKey.EXPECT().GetStringValue(ecsDataFileValueName).Return(`C:\data.json`, uint32(0), nil)
	mockKey.EXPECT().Close()

	tempOpen := open
	open = func(name string) (oswrapper.File, error) {
		return dataFile, nil
	}
	readAll = func(r io.Reader) ([]byte, error) {
		return ioutil.ReadAll(dataFile)
	}
	defer func() {
		open = tempOpen
		readAll = ioutil.ReadAll
	}()
	err = manager.Load()
	assert.Nil(t, err, "Error loading state")
	assert.Equal(t, "test", cluster, "Wrong cluster")
	assert.Equal(t, int64(0), sequenceNumber, "v1 should give a sequence number of 0")
	assert.Equal(t, "arn:aws:ecs:us-west-2:1234567890:container-instance/a9f8e650-e66e-466d-9b0e-3cbce3ba5245", containerInstanceArn)
	assert.Equal(t, "i-00000000", savedInstanceID)
}

func TestStateManagerLoadV13Data(t *testing.T) {
	var containerInstanceArn, cluster, savedInstanceID string
	var sequenceNumber int64

	mockRegistry, mockKey, _, manager := setup(t,
		AddSaveable("ContainerInstanceArn", &containerInstanceArn),
		AddSaveable("Cluster", &cluster),
		AddSaveable("EC2InstanceID", &savedInstanceID),
		AddSaveable("SeqNum", &sequenceNumber))

	dataFile, err := os.Open(filepath.Join(".", "testdata", "v13", "1", ecsDataFile))
	assert.Nil(t, err, "Error opening test data")
	defer dataFile.Close()

	tempOpen := open
	open = func(name string) (oswrapper.File, error) {
		return dataFile, nil
	}
	defer func() {
		open = tempOpen
	}()

	// TODO figure out why gomock does not like registry.READ as the mode
	mockRegistry.EXPECT().OpenKey(ecsDataFileRootKey, ecsDataFileKeyPath, gomock.Any()).Return(mockKey, nil)
	mockKey.EXPECT().GetStringValue(ecsDataFileValueName).Return(`C:\data.json`, uint32(0), nil)
	mockKey.EXPECT().Close()
	err = manager.Load()
	assert.Nil(t, err, "Error loading state")
	assert.Equal(t, "test-statefile", cluster, "Wrong cluster")
	assert.Equal(t, int64(0), sequenceNumber, "v13 should give a sequence number of 0")
	assert.Equal(t, "arn:aws:ecs:us-west-2:1234567890:container-instance/test-statefile/b9c2c229d5ce4f52851f3e9e6d4db894", containerInstanceArn)
	assert.Equal(t, "i-1234567890", savedInstanceID)
}

func TestStateManagerSaveCreateFileError(t *testing.T) {
	mockRegistry, mockKey, _, manager := setup(t)

	// TODO figure out why gomock does not like registry.READ as the mode
	gomock.InOrder(
		mockRegistry.EXPECT().OpenKey(ecsDataFileRootKey, ecsDataFileKeyPath, gomock.Any()).Return(mockKey, nil),
		mockKey.EXPECT().GetStringValue(ecsDataFileValueName).Return(`C:\data.json`, uint32(0), nil),
		mockKey.EXPECT().Close(),
	)
	defer mockTempFileError()()

	err := manager.Save()
	assert.Equal(t, testError, err, "expected error creating file")
}

func TestStateManagerSaveSyncFileError(t *testing.T) {
	mockRegistry, mockKey, mockFile, manager := setup(t)

	mockFile.(*mocks.MockFile).NameImpl = mockNameImpl
	mockFile.(*mock_oswrapper.MockFile).SyncImpl = func() error {
		return errors.New("test error")
	}
	defer mockTempFileError()()

	// TODO figure out why gomock does not like registry.READ as the mode
	gomock.InOrder(
		mockRegistry.EXPECT().OpenKey(ecsDataFileRootKey, ecsDataFileKeyPath, gomock.Any()).Return(mockKey, nil),
		mockKey.EXPECT().GetStringValue(ecsDataFileValueName).Return(`C:\data.json`, uint32(0), nil),
		mockKey.EXPECT().Close(),
	)
	err := manager.Save()
	assert.Equal(t, testError, err, "expected error creating file")
}

func TestStateManagerSave(t *testing.T) {
	mockRegistry, mockKey, mockFile, manager := setup(t)

	otempFile := tempFile
	tempFile = func(dir, pattern string) (oswrapper.File, error) {
		return mockFile, nil
	}
	remove = func(name string) error {
		return nil
	}
	mockFile.(*mocks.MockFile).NameImpl = mockNameImpl
	mockFile.(*mock_oswrapper.MockFile).SyncImpl = mockSyncImpl
	defer func() {
		tempFile = otempFile
		remove = os.Remove
	}()

	// TODO figure out why gomock does not like registry.READ as the mode
	gomock.InOrder(
		mockRegistry.EXPECT().OpenKey(ecsDataFileRootKey, ecsDataFileKeyPath, gomock.Any()).Return(mockKey, nil),
		mockKey.EXPECT().GetStringValue(ecsDataFileValueName).Return(`C:\old.json`, uint32(0), nil),
		mockKey.EXPECT().Close(),
		mockRegistry.EXPECT().CreateKey(ecsDataFileRootKey, ecsDataFileKeyPath, gomock.Any()).Return(mockKey, false, nil),
		mockKey.EXPECT().SetStringValue(ecsDataFileValueName, `C:\new.json`),
		mockKey.EXPECT().Close(),
	)
	err := manager.Save()
	assert.Nil(t, err)
}

func TestStateManagerNoOldStateRemoval(t *testing.T) {
	mockRegistry, mockKey, mockFile, manager := setup(t)

	otempFile := tempFile
	tempFile = func(dir, pattern string) (oswrapper.File, error) {
		return mockFile, nil
	}
	mockFile.(*mock_oswrapper.MockFile).NameImpl = mockNameImpl
	mockFile.(*mock_oswrapper.MockFile).SyncImpl = mockSyncImpl

	defer func() {
		tempFile = otempFile
	}()

	gomock.InOrder(
		mockRegistry.EXPECT().OpenKey(ecsDataFileRootKey, ecsDataFileKeyPath, gomock.Any()).Return(mockKey, nil),
		mockKey.EXPECT().GetStringValue(ecsDataFileValueName).Return(``, uint32(0), nil),
		mockKey.EXPECT().Close(),
		mockRegistry.EXPECT().CreateKey(ecsDataFileRootKey, ecsDataFileKeyPath, gomock.Any()).Return(mockKey, false, nil),
		mockKey.EXPECT().SetStringValue(ecsDataFileValueName, `C:\new.json`),
		mockKey.EXPECT().Close(),
	)
	err := manager.Save()
	assert.Nil(t, err)
}

// TODO TestStateManagerSave + errors
