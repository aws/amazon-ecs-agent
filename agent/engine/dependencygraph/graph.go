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

package dependencygraph

import (
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/logger"
)

var log = logger.ForModule("dependencygraph")

// Because a container may depend on another container being created
// (volumes-from) or running (links) it makes sense to abstract it out
// to each container having dependencies on another container being in any
// perticular state set. For now, these are resolved here and support only
// volume/link (created/run)

// ValidDependencies takes a task and verifies that it is possible to allow all
// containers within it to reach the desired status by proceeding in some order
func ValidDependencies(task *api.Task) bool {
	unresolved := make([]*api.Container, len(task.Containers))
	resolved := make([]*api.Container, 0, len(task.Containers))

	copy(unresolved, task.Containers)

OuterLoop:
	for len(unresolved) > 0 {
		for i, tryResolve := range unresolved {
			if dependenciesCanBeResolved(tryResolve, resolved) {
				resolved = append(resolved, tryResolve)
				unresolved = append(unresolved[:i], unresolved[i+1:]...)
				// Break out of the inner loop now that we modified the slice
				// we're looping over
				continue OuterLoop
			}
		}
		log.Warn("Could not resolve some containers", "task", task, "unresolved", unresolved)
		return false
	}

	return true
}

func linksToContainerNames(links []string) []string {
	names := make([]string, 0, len(links))
	for _, link := range links {
		name := strings.Split(link, ":")[0]
		names = append(names, name)
	}
	return names
}

// DependenciesCanBeResolved verifies that it's possible to start a `target`
// given a group of already handled containers, `by`. Essentially, it asks "is
// `target` resolved by `by`". It assumes that everything in `by` has reached
// DesiredStatus and that `target` is also trying to get there
//
// This function is used for verifying that a state should be resolveable, not
// for actually deciding what to do. `DependenciesAreResolved` should be used for
// that purpose instead.
func dependenciesCanBeResolved(target *api.Container, by []*api.Container) bool {
	nameMap := make(map[string]*api.Container)
	for _, cont := range by {
		nameMap[cont.Name] = cont
	}
	neededVolumeContainers := make([]string, len(target.VolumesFrom))
	for i, volume := range target.VolumesFrom {
		neededVolumeContainers[i] = volume.SourceContainer
	}

	return verifyStatusResolveable(target, nameMap, neededVolumeContainers, volumeCanResolve) &&
		verifyStatusResolveable(target, nameMap, linksToContainerNames(target.Links), linkCanResolve)
}

// DependenciesAreResolved validates that the `target` container can be started
// given the current known state of the containers in `by`. If this function
// returns true, `target` should be technically able to launch with on issues
func DependenciesAreResolved(target *api.Container, by []*api.Container) bool {
	nameMap := make(map[string]*api.Container)
	for _, cont := range by {
		nameMap[cont.Name] = cont
	}
	neededVolumeContainers := make([]string, len(target.VolumesFrom))
	for i, volume := range target.VolumesFrom {
		neededVolumeContainers[i] = volume.SourceContainer
	}

	return verifyStatusResolveable(target, nameMap, neededVolumeContainers, volumeIsResolved) &&
		verifyStatusResolveable(target, nameMap, linksToContainerNames(target.Links), linkIsResolved) &&
		verifyStatusResolveable(target, nameMap, target.RunDependencies, onRunIsResolved)
}

// verifyStatusResolveable validates that `target` can be resolved given that
// target depends on `dependencies` (which are container names) and there are
// `existingContainers` (map from name to container). The `resolves` function
// passed should return true if the named container is resolved.
func verifyStatusResolveable(target *api.Container, existingContainers map[string]*api.Container, dependencies []string, resolves func(*api.Container, *api.Container) bool) bool {
	targetGoal := target.DesiredStatus
	if targetGoal != api.ContainerRunning && targetGoal != api.ContainerCreated {
		// A container can always stop, die, or reach whatever other statre it
		// wants regardless of what dependencies it has
		return true
	}

	for _, dependency := range dependencies {
		maybeResolves, exists := existingContainers[dependency]
		if !exists {
			return false
		}
		if !resolves(target, maybeResolves) {
			return false
		}
	}
	return true
}

func linkCanResolve(target *api.Container, link *api.Container) bool {
	if target.DesiredStatus == api.ContainerCreated {
		return link.DesiredStatus == api.ContainerCreated || link.DesiredStatus == api.ContainerRunning
	} else if target.DesiredStatus == api.ContainerRunning {
		return link.DesiredStatus == api.ContainerRunning
	}
	log.Error("Unexpected desired status", "target", target)
	return false
}

func linkIsResolved(target *api.Container, link *api.Container) bool {
	if target.DesiredStatus == api.ContainerCreated {
		return link.KnownStatus == api.ContainerCreated || link.KnownStatus == api.ContainerRunning
	} else if target.DesiredStatus == api.ContainerRunning {
		return link.KnownStatus == api.ContainerRunning
	}
	log.Error("Unexpected desired status", "target", target)
	return false
}

func volumeCanResolve(target *api.Container, volume *api.Container) bool {
	if target.DesiredStatus == api.ContainerCreated || target.DesiredStatus == api.ContainerRunning {
		return volume.DesiredStatus == api.ContainerCreated ||
			volume.DesiredStatus == api.ContainerRunning ||
			volume.DesiredStatus == api.ContainerStopped
	}

	log.Error("Unexpected desired status", "target", target)
	return false
}

func volumeIsResolved(target *api.Container, volume *api.Container) bool {
	if target.DesiredStatus == api.ContainerCreated || target.DesiredStatus == api.ContainerRunning {
		return volume.KnownStatus == api.ContainerCreated ||
			volume.KnownStatus == api.ContainerRunning ||
			volume.KnownStatus == api.ContainerStopped
	}

	log.Error("Unexpected desired status", "target", target)
	return false
}

// onRunIsResolved defines a relationship where a target cannot be created until
// 'run' has reached a running state.
func onRunIsResolved(target *api.Container, run *api.Container) bool {
	if target.DesiredStatus >= api.ContainerCreated {
		return run.KnownStatus >= api.ContainerRunning
	}
	return false
}
