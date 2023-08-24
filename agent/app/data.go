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

package app

import (
	"strconv"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	dm "github.com/aws/amazon-ecs-agent/agent/engine/daemonmanager"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/engine/execcmd"
	"github.com/aws/amazon-ecs-agent/agent/engine/serviceconnect"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/eventstream"

	"github.com/pkg/errors"
)

const (
	objNotFoundErrMsg = "not found"
)

// savedData holds all the data that we might save in db.
type savedData struct {
	taskEngine               engine.TaskEngine
	agentVersion             string
	availabilityZone         string
	cluster                  string
	containerInstanceARN     string
	ec2InstanceID            string
	latestTaskManifestSeqNum int64
}

// loadData loads data from previous checkpoint file, if any, with backward compatibility preserved. It first tries to
// load from boltdb, and if it doesn't get anything, it tries to load from state file and then save data it loaded to
// boltdb. Behavior of three cases are considered:
//
//  1. Agent starts from fresh instance (no previous state):
//     (1) Try to load from boltdb, get nothing;
//     (2) Try to load from state file, get nothing;
//     (3) Return empty data.
//
//  2. Agent starts with previous state stored in boltdb:
//     (1) Try to load from boltdb, get the data;
//     (2) Return loaded data.
//
//  3. Agent starts with previous state stored in state file (i.e. it was just upgraded from an old agent that uses state file):
//     (1) Try to load from boltdb, get nothing;
//     (2) Try to load from state file, get something;
//     (3) Save loaded data to boltdb;
//     (4) Return loaded data.
func (agent *ecsAgent) loadData(containerChangeEventStream *eventstream.EventStream,
	credentialsManager credentials.Manager,
	state dockerstate.TaskEngineState,
	imageManager engine.ImageManager,
	hostResources map[string]*ecs.Resource,
	execCmdMgr execcmd.Manager,
	serviceConnectManager serviceconnect.Manager,
	daemonManagers map[string]dm.DaemonManager) (*savedData, error) {

	s := &savedData{
		taskEngine: engine.NewTaskEngine(agent.cfg, agent.dockerClient, credentialsManager,
			containerChangeEventStream, imageManager, hostResources, state,
			agent.metadataManager, agent.resourceFields, execCmdMgr,
			serviceConnectManager, daemonManagers),
	}
	s.taskEngine.SetDataClient(agent.dataClient)

	err := agent.loadDataFromBoltDB(s)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load previous data from BoltDB")
	}

	// The fact that agent version is present in the boltdb database is served as an indicator that
	// we has started using boltdb to store data. In the migration case (case 3 mentioned in method comment),
	// we migrate the data to boltdb before saving the version, so if the version is saved, the data has been
	// migrated to boltdb.
	if s.agentVersion != "" {
		return s, nil
	}

	err = agent.loadDataFromStateFile(s)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load previous data from state file")
	}

	err = agent.saveDataToBoltDB(s)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to save loaded data to BoltDB")
	}

	return s, nil
}

func (agent *ecsAgent) loadDataFromBoltDB(s *savedData) error {
	err := s.taskEngine.LoadState()
	if err != nil {
		return errors.Wrap(err, "failed to load task engine state")
	}

	s.agentVersion, err = agent.loadMetadata(data.AgentVersionKey)
	if err != nil {
		return err
	}
	s.availabilityZone, err = agent.loadMetadata(data.AvailabilityZoneKey)
	if err != nil {
		return err
	}
	s.cluster, err = agent.loadMetadata(data.ClusterNameKey)
	if err != nil {
		return err
	}
	s.containerInstanceARN, err = agent.loadMetadata(data.ContainerInstanceARNKey)
	if err != nil {
		return err
	}
	s.ec2InstanceID, err = agent.loadMetadata(data.EC2InstanceIDKey)
	if err != nil {
		return err
	}
	seqNumStr, err := agent.loadMetadata(data.TaskManifestSeqNumKey)
	if err != nil {
		return err
	}
	if seqNumStr != "" {
		s.latestTaskManifestSeqNum, err = strconv.ParseInt(seqNumStr, 10, 64)
		if err != nil {
			return errors.Wrapf(err, "failed to convert saved task manifest sequence number to int64: %s", seqNumStr)
		}
	}
	return nil
}

func (agent *ecsAgent) loadDataFromStateFile(s *savedData) error {
	stateManager, err := agent.newStateManager(s.taskEngine, &s.cluster, &s.containerInstanceARN,
		&s.ec2InstanceID, &s.availabilityZone, &s.latestTaskManifestSeqNum)
	if err != nil {
		return err
	}

	err = stateManager.Load()
	if err != nil {
		return err
	}
	return nil
}

func (agent *ecsAgent) saveDataToBoltDB(s *savedData) error {
	if err := s.taskEngine.SaveState(); err != nil {
		return err
	}
	if s.availabilityZone != "" {
		if err := agent.dataClient.SaveMetadata(data.AvailabilityZoneKey, s.availabilityZone); err != nil {
			return err
		}
	}
	if s.cluster != "" {
		if err := agent.dataClient.SaveMetadata(data.ClusterNameKey, s.cluster); err != nil {
			return err
		}
	}
	if s.containerInstanceARN != "" {
		if err := agent.dataClient.SaveMetadata(data.ContainerInstanceARNKey, s.containerInstanceARN); err != nil {
			return err
		}
	}
	if s.ec2InstanceID != "" {
		if err := agent.dataClient.SaveMetadata(data.EC2InstanceIDKey, s.ec2InstanceID); err != nil {
			return err
		}
	}
	if err := agent.dataClient.SaveMetadata(data.TaskManifestSeqNumKey,
		strconv.FormatInt(s.latestTaskManifestSeqNum, 10)); err != nil {
		return err
	}

	return nil
}

func (agent *ecsAgent) loadMetadata(key string) (string, error) {
	val, err := agent.dataClient.GetMetadata(key)
	if err != nil {
		if strings.Contains(err.Error(), objNotFoundErrMsg) {
			// Happen when starting with empty db, not actual error
			return "", nil
		}
		return "", errors.Wrapf(err, "failed to get value for metadata '%s'", key)
	}
	return val, nil
}
