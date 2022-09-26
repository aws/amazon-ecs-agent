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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/cihub/seelog"
)

const (
	// PluginStatePath is the directory path to the plugin state information
	// TODO: get this value from an env var
	PluginStatePath = "/var/lib/ecs/data/"
	// PluginStateFile contains the state information of the plugin
	PluginStateFile = "ecs_volume_plugin.json"
	// PluginStateFileAbsPath is the absolute path of the plugin state file
	PluginStateFileAbsPath = "/var/lib/ecs/data/ecs_volume_plugin.json"
)

// StateManager manages the state of the volumes information
type StateManager struct {
	VolState *VolumeState
	lock     sync.Mutex
}

// VolumeState contains the list of managed volumes
type VolumeState struct {
	Volumes map[string]*VolumeInfo `json:"volumes,omitempty"`
}

// VolumeInfo contains the information of managed volumes
type VolumeInfo struct {
	Type      string            `json:"type,omitempty"`
	Path      string            `json:"path,omitempty"`
	Options   map[string]string `json:"options,omitempty"`
	CreatedAt string            `json:"createdAt,omitempty"`
}

// NewStateManager initializes the state manager of volume plugin
func NewStateManager() *StateManager {
	return &StateManager{
		VolState: &VolumeState{
			Volumes: make(map[string]*VolumeInfo),
		},
	}
}

func (s *StateManager) recordVolume(volName string, vol *Volume) error {
	s.VolState.Volumes[volName] = &VolumeInfo{
		Type:      vol.Type,
		Path:      vol.Path,
		Options:   vol.Options,
		CreatedAt: vol.CreatedAt,
	}
	return s.save()
}

func (s *StateManager) removeVolume(volName string) error {
	delete(s.VolState.Volumes, volName)
	return s.save()
}

// saves volume state to the file at path
func (s *StateManager) save() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	b, err := json.MarshalIndent(s.VolState, "", "\t")
	if err != nil {
		return fmt.Errorf("marshal data failed: %v", err)
	}
	return saveStateToDisk(b)
}

var saveStateToDisk = saveState

func saveState(b []byte) error {
	// Make our temp-file on the same volume as our data-file to ensure we can
	// actually move it atomically; cross-device renaming will error out
	tmpfile, err := os.CreateTemp(PluginStatePath, "tmp_ecs_volume_plugin")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	_, err = tmpfile.Write(b)
	if err != nil {
		return fmt.Errorf("failed to write state to temp file: %v", err)
	}

	// flush temp state file to disk
	err = tmpfile.Sync()
	if err != nil {
		return fmt.Errorf("error flushing state file: %v", err)
	}

	err = os.Rename(tmpfile.Name(), filepath.Join(PluginStatePath, PluginStateFile))
	if err != nil {
		return fmt.Errorf("could not move data to state file: %v", err)
	}

	stateDir, err := os.Open(PluginStatePath)
	if err != nil {
		return fmt.Errorf("error opening state path: %v", err)
	}

	// sync directory entry of the new state file to disk
	err = stateDir.Sync()
	if err != nil {
		return fmt.Errorf("error syncing state file directory entry: %v", err)
	}
	return nil
}

var fileExists = checkFile

func checkFile(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

// loads the file at path into interface 'a'
func (s *StateManager) load(a interface{}) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	b, err := readStateFile()
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, a)
	if err != nil {
		seelog.Criticalf("Could not unmarshal existing state; corrupted data: %v. Please remove statefile at %s", err, PluginStateFileAbsPath)
		return err
	}
	return err
}

var readStateFile = readFile

func readFile() ([]byte, error) {
	return os.ReadFile(PluginStateFileAbsPath)
}
