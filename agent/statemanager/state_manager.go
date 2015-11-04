// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

// Package statemanager implements simple constructs for saving and restoring
// state from disk.
// It provides the interface for a StateManager which can read/write arbitrary
// json data from/to disk.
package statemanager

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/logger"
)

// The current version of saved data. Any backwards or forwards incompatible
// changes to the data-format should increment this number and retain the
// ability to read old data versions.
// Version changes:
// 1) initial
// 2)
//   a) Add 'ACSSeqNum' top level field (backwards compatible; technically
//      forwards compatible but could cause resource constraint violations)
//   b) remove 'DEAD', 'UNKNOWN' state from ever being marshalled (backward and
//      forward compatible)
// 3) Add 'Protocol' field to 'portMappings' and 'KnownPortBindings'
// 4) Add 'DockerConfig' struct
const EcsDataVersion = 4

// Filename in the ECS_DATADIR
const ecsDataFile = "ecs_agent_data.json"

// How frequently to flush to disk
const minSaveInterval = 10 * time.Second

var log = logger.ForModule("statemanager")

// Saveable types should be able to be json serializable and deserializable
// Properly, this should have json.Marshaler/json.Unmarshaler here, but string
// and so on can be marshaled/unmarshaled sanely but don't fit those interfaces.
type Saveable interface{}

// Saver is a type that can be saved
type Saver interface {
	Save() error
	ForceSave() error
}

// Option functions are functions that may be used as part of constructing a new
// StateManager
type Option func(StateManager)

type saveableState map[string]*Saveable
type intermediateSaveableState map[string]json.RawMessage

// State is a struct of all data that should be saveable/loadable to disk. Each
// element should be json-serializable.
//
// Note, changing this to work with BinaryMarshaler or another more compact
// format would be fine, but everything already needs a json representation
// since that's our wire format and the extra space taken / IO-time is expected
// to be fairly negligable.
type state struct {
	Data saveableState

	Version int
}

type intermediateState struct {
	Data intermediateSaveableState
}

type versionOnlyState struct {
	Version int
}

// A StateManager can load and save state from disk.
// Load is not expected to return an error if there is no state to load.
type StateManager interface {
	Saver
	Load() error
}

type basicStateManager struct {
	statePath string // The path to a file in which state can be serialized

	state *state // pointers to the data we should save / load into

	saveTimesLock   sync.Mutex // guards save times
	lastSave        time.Time  //the last time a save completed
	nextPlannedSave time.Time  //the next time a save is planned

	savingLock sync.Mutex // guards marshal, write, and move
}

// NewStateManager constructs a new StateManager which saves data at the
// location specified in cfg and operates under the given options.
// The returned StateManager will not save more often than every 10 seconds and
// will not reliably return errors with Save, but will log them appropriately.
func NewStateManager(cfg *config.Config, options ...Option) (StateManager, error) {
	fi, err := os.Stat(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, errors.New("State manager DataDir must exist")
	}

	state := &state{
		Data:    make(saveableState),
		Version: EcsDataVersion,
	}
	manager := &basicStateManager{
		statePath: cfg.DataDir,
		state:     state,
	}

	for _, option := range options {
		option(manager)
	}

	return manager, nil
}

// AddSaveable is an option that adds a given saveable as one that should be saved
// under the given name. The name must be the same across uses of the
// statemanager (e.g. program invocations) for it to be serialized and
// deserialized correctly.
func AddSaveable(name string, saveable Saveable) Option {
	return (Option)(func(m StateManager) {
		manager, ok := m.(*basicStateManager)
		if !ok {
			log.Crit("Unable to add to state manager; unknown instantiation")
			return
		}
		manager.state.Data[name] = &saveable
	})
}

// Save triggers a save to file, though respects a minimum save interval to wait
// between saves.
func (manager *basicStateManager) Save() error {
	manager.saveTimesLock.Lock()
	defer manager.saveTimesLock.Unlock()
	if time.Since(manager.lastSave) >= minSaveInterval {
		// we can just save
		err := manager.ForceSave()
		manager.lastSave = time.Now()
		manager.nextPlannedSave = time.Time{} // re-zero it; assume all pending desires to save are fulfilled
		return err
	} else if manager.nextPlannedSave.IsZero() {
		// No save planned yet, we should plan one.
		next := manager.lastSave.Add(minSaveInterval)
		manager.nextPlannedSave = next
		go func() {
			time.Sleep(next.Sub(time.Now()))
			manager.Save()
		}()
	}
	// else nextPlannedSave wasn't Zero so there's a save planned elsewhere that'll
	// fulfill this
	return nil
}

// ForceSave saves the given State to a file. It is an atomic operation on POSIX
// systems (by Renaming over the target file).
// This function logs errors at will and does not necessarily expect the caller
// to handle the error because there's little a caller can do in general other
// than just keep going.
// In addition, the StateManager internally buffers save requests in order to
// only save at most every STATE_SAVE_INTERVAL.
func (manager *basicStateManager) ForceSave() error {
	manager.savingLock.Lock()
	defer manager.savingLock.Unlock()
	log.Info("Saving state!")
	s := manager.state
	s.Version = EcsDataVersion

	data, err := json.Marshal(s)
	if err != nil {
		log.Error("Error saving state; could not marshal data; this is odd", "err", err)
		return err
	}
	// Make our temp-file on the same volume as our data-file to ensure we can
	// actually move it atomically; cross-device renaming will error out.
	tmpfile, err := ioutil.TempFile(manager.statePath, "tmp_ecs_agent_data")
	if err != nil {
		log.Error("Error saving state; could not create temp file to save state", "err", err)
		return err
	}
	_, err = tmpfile.Write(data)
	if err != nil {
		log.Error("Error saving state; could not write to temp file to save state", "err", err)
		return err
	}
	err = os.Rename(tmpfile.Name(), filepath.Join(manager.statePath, ecsDataFile))
	if err != nil {
		log.Error("Error saving state; could not move to data file", "err", err)
	}
	return err
}

// Load reads state off the disk from the well-known filepath and loads it into
// the passed State object.
func (manager *basicStateManager) Load() error {
	// Note that even if Save overwrites the file we're looking at here, we
	// still hold the old inode and should read the old data so no locking is
	// needed (given Linux and the ext* family of fs at least).
	s := manager.state
	log.Info("Loading state!")
	file, err := os.Open(filepath.Join(manager.statePath, ecsDataFile))
	if err != nil {
		if os.IsNotExist(err) {
			// Happens every first run; not a real error
			return nil
		}
		return err
	}
	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Error("Error reading existing state file", "err", err)
		return err
	}
	// Dry-run to make sure this is a version we can understand
	tmps := versionOnlyState{}
	err = json.Unmarshal(data, &tmps)
	if err != nil {
		log.Crit("Could not unmarshal existing state; corrupted data?", "err", err, "data", data)
		return err
	}
	if tmps.Version > EcsDataVersion {
		strversion := strconv.Itoa(tmps.Version)
		return errors.New("Unsupported data format: Version " + strversion + " not " + strconv.Itoa(EcsDataVersion))
	}
	// Now load it into the actual state. The reason we do this with the
	// intermediate state is that we *must* unmarshal directly into the
	// "saveable" pointers we were given in AddSaveable; if we unmarshal
	// directly into a map with values of pointers, those pointers are lost.
	// We *must* unmarshal this way because the existing pointers could have
	// semi-initialized data (and are actually expected to)

	var intermediate intermediateState
	err = json.Unmarshal(data, &intermediate)
	if err != nil {
		log.Debug("Could not unmarshal into intermediate")
		return err
	}

	for key, rawJSON := range intermediate.Data {
		actualPointer, ok := manager.state.Data[key]
		if !ok {
			log.Error("Loading state: potentially malformed json key of " + key)
			continue
		}
		err = json.Unmarshal(rawJSON, actualPointer)
		if err != nil {
			log.Debug("Could not unmarshal into actual")
			return err
		}
	}

	log.Debug("Loaded state!", "state", s)
	return nil
}
