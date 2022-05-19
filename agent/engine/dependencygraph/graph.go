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

package dependencygraph

import (
	"fmt"
	"strings"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	// CreateCondition ensures that a container progresses to next state only when dependency container has started
	createCondition = "CREATE"
	// StartCondition ensures that a container progresses to next state only when dependency container is running
	startCondition = "START"
	// SuccessCondition ensures that a container progresses to next state only when
	// dependency container has successfully completed with exit code 0
	successCondition = "SUCCESS"
	// CompleteCondition ensures that a container progresses to next state only when dependency container has completed
	completeCondition = "COMPLETE"
	// HealthyCondition ensures that a container progresses to next state only when dependency container is healthy
	healthyCondition = "HEALTHY"
	// 0 is the standard exit code for success.
	successExitCode = 0
)

var (
	// CredentialsNotResolvedErr is the error where a container needs to wait for
	// credentials before it can process by agent
	CredentialsNotResolvedErr = &dependencyError{err: errors.New("dependency graph: container execution credentials not available")}
	// DependentContainerNotResolvedErr is the error where a dependent container isn't in expected state
	DependentContainerNotResolvedErr = &dependencyError{err: errors.New("dependency graph: dependent container not in expected state")}
	// ContainerPastDesiredStatusErr is the error where the container status is bigger than desired status
	ContainerPastDesiredStatusErr = &dependencyError{err: errors.New("container transition: container status is equal or greater than desired status")}
	// ErrContainerDependencyNotResolved is when the container's dependencies
	// on other containers are not resolved
	ErrContainerDependencyNotResolved = &dependencyError{err: errors.New("dependency graph: dependency on containers not resolved")}
	// ErrResourceDependencyNotResolved is when the container's dependencies
	// on task resources are not resolved
	ErrResourceDependencyNotResolved = &dependencyError{err: errors.New("dependency graph: dependency on resources not resolved")}
	// ResourcePastDesiredStatusErr is the error where the task resource known status is bigger than desired status
	ResourcePastDesiredStatusErr = &dependencyError{err: errors.New("task resource transition: task resource status is equal or greater than desired status")}
	// ErrContainerDependencyNotResolvedForResource is when the resource's dependencies
	// on other containers are not resolved
	ErrContainerDependencyNotResolvedForResource = &dependencyError{err: errors.New("dependency graph: resource's dependency on containers not resolved")}
)

// DependencyError represents an error of a container dependency. These errors can be either terminal or non-terminal.
// Terminal dependency errors indicate that a given dependency can never be fulfilled (e.g. a container with a SUCCESS
// dependency has stopped with an exit code other than zero).
type DependencyError interface {
	Error() string
	IsTerminal() bool
}

type dependencyError struct {
	err        error
	isTerminal bool
}

func (de *dependencyError) Error() string {
	return de.err.Error()
}

func (de *dependencyError) IsTerminal() bool {
	return de.isTerminal
}

// ValidDependencies takes a task and verifies that it is possible to allow all
// containers within it to reach the desired status by proceeding in some
// order.
func ValidDependencies(task *apitask.Task, cfg *config.Config) bool {
	unresolved := make([]*apicontainer.Container, len(task.Containers))
	resolved := make([]*apicontainer.Container, 0, len(task.Containers))

	copy(unresolved, task.Containers)

OuterLoop:
	for len(unresolved) > 0 {
		for i, tryResolve := range unresolved {
			if dependenciesCanBeResolved(tryResolve, resolved, cfg) {
				resolved = append(resolved, tryResolve)
				unresolved = append(unresolved[:i], unresolved[i+1:]...)
				// Break out of the inner loop now that we modified the slice
				// we're looping over
				continue OuterLoop
			}
		}
		log.Warnf("Could not resolve some containers: [%v] for task %v", unresolved, task)
		return false
	}

	return true
}

// DependenciesCanBeResolved verifies that it's possible to transition a `target`
// given a group of already handled containers, `by`. Essentially, it asks "is
// `target` resolved by `by`". It assumes that everything in `by` has reached
// DesiredStatus and that `target` is also trying to get there
//
// This function is used for verifying that a state should be resolvable, not
// for actually deciding what to do. `DependenciesAreResolved` should be used for
// that purpose instead.
func dependenciesCanBeResolved(target *apicontainer.Container, by []*apicontainer.Container, cfg *config.Config) bool {
	nameMap := make(map[string]*apicontainer.Container)
	for _, cont := range by {
		nameMap[cont.Name] = cont
	}

	if _, err := verifyContainerOrderingStatusResolvable(target, nameMap, cfg, containerOrderingDependenciesCanResolve); err != nil {
		return false
	}
	return verifyStatusResolvable(target, nameMap, target.SteadyStateDependencies, onSteadyStateCanResolve)
}

// DependenciesAreResolved validates that the `target` container can be
// transitioned given the current known state of the containers in `by`. If
// this function returns true, `target` should be technically able to launch
// without issues.
// Transitions are between known statuses (whether the container can move to
// the next known status), not desired statuses; the desired status typically
// is either RUNNING or STOPPED.
func DependenciesAreResolved(target *apicontainer.Container,
	by []*apicontainer.Container,
	id string,
	manager credentials.Manager,
	resources []taskresource.TaskResource,
	cfg *config.Config) (*apicontainer.DependsOn, DependencyError) {
	if !executionCredentialsResolved(target, id, manager) {
		return nil, CredentialsNotResolvedErr
	}

	nameMap := make(map[string]*apicontainer.Container)
	for _, cont := range by {
		nameMap[cont.Name] = cont
	}
	neededVolumeContainers := make([]string, len(target.VolumesFrom))
	for i, volume := range target.VolumesFrom {
		neededVolumeContainers[i] = volume.SourceContainer
	}

	resourcesMap := make(map[string]taskresource.TaskResource)
	for _, resource := range resources {
		resourcesMap[resource.GetName()] = resource
	}

	if blocked, err := verifyContainerOrderingStatusResolvable(target, nameMap, cfg, containerOrderingDependenciesIsResolved); err != nil {
		return blocked, err
	}

	if !verifyStatusResolvable(target, nameMap, target.SteadyStateDependencies, onSteadyStateIsResolved) {
		return nil, DependentContainerNotResolvedErr
	}
	if err := verifyTransitionDependenciesResolved(target, nameMap, resourcesMap); err != nil {
		return nil, err
	}

	// If the target is desired terminal and isn't stopped, we should validate that it doesn't have any containers
	// that are dependent on it that need to shut down first.
	if target.DesiredTerminal() && !target.KnownTerminal() {
		if err := verifyShutdownOrder(target, nameMap); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

// TaskResourceDependenciesAreResolved validates that the `target` resource can be
// transitioned given the current known state of the containers in `by`. If
// this function returns true, `target` should be technically able to transit
// without issues.
// Transitions are between known statuses (whether the resource can move to
// the next known status), not desired statuses; the desired status typically
// is either CREATED or REMOVED.
func TaskResourceDependenciesAreResolved(target taskresource.TaskResource,
	by []*apicontainer.Container) error {

	nameMap := make(map[string]*apicontainer.Container)
	for _, cont := range by {
		nameMap[cont.Name] = cont
	}

	if !verifyContainerDependenciesResolvedForResource(target, nameMap) {
		return ErrContainerDependencyNotResolvedForResource
	}

	return nil
}

func verifyContainerDependenciesResolvedForResource(target taskresource.TaskResource, existingContainers map[string]*apicontainer.Container) bool {
	targetNext := target.NextKnownState()
	// For task resource without dependency map, containerDependencies will just be nil
	containerDependencies := target.GetContainerDependencies(targetNext)
	for _, containerDependency := range containerDependencies {
		dep, exists := existingContainers[containerDependency.ContainerName]
		if !exists {
			return false
		}
		if dep.GetKnownStatus() < containerDependency.SatisfiedStatus {
			return false
		}
	}
	return true
}

func executionCredentialsResolved(target *apicontainer.Container, id string, manager credentials.Manager) bool {
	if target.GetKnownStatus() >= apicontainerstatus.ContainerPulled ||
		!target.ShouldPullWithExecutionRole() ||
		target.GetDesiredStatus() >= apicontainerstatus.ContainerStopped {
		return true
	}

	_, ok := manager.GetTaskCredentials(id)
	return ok
}

// verifyStatusResolvable validates that `target` can be resolved given that
// target depends on `dependencies` (which are container names) and there are
// `existingContainers` (map from name to container). The `resolves` function
// passed should return true if the named container is resolved.
func verifyStatusResolvable(target *apicontainer.Container, existingContainers map[string]*apicontainer.Container,
	dependencies []string, resolves func(*apicontainer.Container, *apicontainer.Container) bool) bool {
	targetGoal := target.GetDesiredStatus()
	if targetGoal != target.GetSteadyStateStatus() && targetGoal != apicontainerstatus.ContainerCreated {
		// A container can always stop, die, or reach whatever other state it
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

// verifyContainerOrderingStatusResolvable validates that `target` can be resolved given that
// the dependsOn containers are resolved and there are `existingContainers`
// (map from name to container). The `resolves` function passed should return true if the named container is resolved.

func verifyContainerOrderingStatusResolvable(target *apicontainer.Container, existingContainers map[string]*apicontainer.Container,
	cfg *config.Config, resolves func(*apicontainer.Container, *apicontainer.Container, string, *config.Config) bool) (*apicontainer.DependsOn, DependencyError) {

	targetGoal := target.GetDesiredStatus()
	targetKnown := target.GetKnownStatus()
	if targetGoal != target.GetSteadyStateStatus() && targetGoal != apicontainerstatus.ContainerCreated {
		// A container can always stop, die, or reach whatever other state it
		// wants regardless of what dependencies it has
		return nil, nil
	}

	var blockedDependency *apicontainer.DependsOn

	targetDependencies := target.GetDependsOn()
	for _, dependency := range targetDependencies {
		dependencyContainer, ok := existingContainers[dependency.ContainerName]
		if !ok {
			return nil, &dependencyError{err: fmt.Errorf("dependency graph: container ordering dependency [%v] for target [%v] does not exist.", dependencyContainer, target), isTerminal: true}
		}

		// We want to check whether the dependency container has timed out only if target has not been created yet.
		// If the target is already created, then everything is normal and dependency can be and is resolved.
		// However, if dependency container has already stopped, then it cannot time out.
		if targetKnown < apicontainerstatus.ContainerCreated && dependencyContainer.GetKnownStatus() != apicontainerstatus.ContainerStopped {
			if hasDependencyTimedOut(dependencyContainer, dependency.Condition) {
				return nil, &dependencyError{err: fmt.Errorf("dependency graph: container ordering dependency [%v] for target [%v] has timed out.", dependencyContainer, target), isTerminal: true}
			}
		}

		// We want to fail fast if the dependency container has stopped but did not exit successfully because target container
		// can then never progress to its desired state when the dependency condition is 'SUCCESS'
		if dependency.Condition == successCondition && dependencyContainer.GetKnownStatus() == apicontainerstatus.ContainerStopped &&
			!hasDependencyStoppedSuccessfully(dependencyContainer) {
			return nil, &dependencyError{err: fmt.Errorf("dependency graph: failed to resolve container ordering dependency [%v] for target [%v] as dependency did not exit successfully.", dependencyContainer, target), isTerminal: true}
		}

		// For any of the dependency conditions - START/COMPLETE/SUCCESS/HEALTHY, if the dependency container has
		// not started and will not start in the future, this dependency can never be resolved.
		if dependencyContainer.HasNotAndWillNotStart() {
			return nil, &dependencyError{err: fmt.Errorf("dependency graph: failed to resolve container ordering dependency [%v] for target [%v] because dependency will never start", dependencyContainer, target), isTerminal: true}
		}

		if !resolves(target, dependencyContainer, dependency.Condition, cfg) {
			blockedDependency = &dependency
		}
	}
	if blockedDependency != nil {
		return blockedDependency, &dependencyError{err: fmt.Errorf("dependency graph: failed to resolve the container ordering dependency [%v] for target [%v]", blockedDependency, target)}
	}
	return nil, nil
}

func verifyTransitionDependenciesResolved(target *apicontainer.Container,
	existingContainers map[string]*apicontainer.Container,
	existingResources map[string]taskresource.TaskResource) DependencyError {

	if !verifyContainerDependenciesResolved(target, existingContainers) {
		return ErrContainerDependencyNotResolved
	}
	if !verifyResourceDependenciesResolved(target, existingResources) {
		return ErrResourceDependencyNotResolved
	}
	return nil
}

func verifyContainerDependenciesResolved(target *apicontainer.Container, existingContainers map[string]*apicontainer.Container) bool {
	targetNext := target.GetNextKnownStateProgression()
	containerDependencies := target.TransitionDependenciesMap[targetNext].ContainerDependencies
	for _, containerDependency := range containerDependencies {
		dep, exists := existingContainers[containerDependency.ContainerName]
		if !exists {
			return false
		}
		if dep.GetKnownStatus() < containerDependency.SatisfiedStatus {
			return false
		}
	}
	return true
}

func verifyResourceDependenciesResolved(target *apicontainer.Container, existingResources map[string]taskresource.TaskResource) bool {
	targetNext := target.GetNextKnownStateProgression()
	resourceDependencies := target.TransitionDependenciesMap[targetNext].ResourceDependencies
	for _, resourceDependency := range resourceDependencies {
		dep, exists := existingResources[resourceDependency.Name]
		if !exists {
			return false
		}
		if dep.GetKnownStatus() < resourceDependency.GetRequiredStatus() {
			return false
		}
	}
	return true
}

func containerOrderingDependenciesCanResolve(target *apicontainer.Container,
	dependsOnContainer *apicontainer.Container,
	dependsOnStatus string,
	cfg *config.Config) bool {

	targetDesiredStatus := target.GetDesiredStatus()
	dependsOnContainerDesiredStatus := dependsOnContainer.GetDesiredStatus()

	switch dependsOnStatus {
	case createCondition:
		return verifyContainerOrderingStatus(dependsOnContainer)

	case startCondition:
		if targetDesiredStatus == apicontainerstatus.ContainerCreated {
			// The 'target' container desires to be moved to 'Created' state.
			// Allow this only if the desired status of the dependency container is
			// 'Created' or if the linked container is in 'steady state'
			return dependsOnContainerDesiredStatus == apicontainerstatus.ContainerCreated ||
				dependsOnContainerDesiredStatus == dependsOnContainer.GetSteadyStateStatus()

		} else if targetDesiredStatus == target.GetSteadyStateStatus() {
			// The 'target' container desires to be moved to its 'steady' state.
			// Allow this only if the dependency container also desires to be in 'steady state'
			return dependsOnContainerDesiredStatus == dependsOnContainer.GetSteadyStateStatus()
		}
		return false

	case successCondition:
		var dependencyStoppedSuccessfully bool

		if dependsOnContainer.GetKnownExitCode() != nil {
			dependencyStoppedSuccessfully = dependsOnContainer.GetKnownStatus() == apicontainerstatus.ContainerStopped &&
				*dependsOnContainer.GetKnownExitCode() == successExitCode
		}
		return verifyContainerOrderingStatus(dependsOnContainer) || dependencyStoppedSuccessfully

	case completeCondition:
		return verifyContainerOrderingStatus(dependsOnContainer)

	case healthyCondition:
		return verifyContainerOrderingStatus(dependsOnContainer) && dependsOnContainer.HealthStatusShouldBeReported()

	default:
		return false
	}
}

func containerOrderingDependenciesIsResolved(target *apicontainer.Container,
	dependsOnContainer *apicontainer.Container,
	dependsOnStatus string,
	cfg *config.Config) bool {

	targetDesiredStatus := target.GetDesiredStatus()
	targetContainerKnownStatus := target.GetKnownStatus()
	dependsOnContainerKnownStatus := dependsOnContainer.GetKnownStatus()

	// The 'target' container desires to be moved to 'Created' or the 'steady' state.
	// Allow this only if the environment variable ECS_PULL_DEPENDENT_CONTAINERS_UPFRONT is enabled and
	// known status of the `target` container state has not reached to 'Pulled' state;
	if cfg.DependentContainersPullUpfront.Enabled() && targetContainerKnownStatus < apicontainerstatus.ContainerPulled {
		return true
	}

	switch dependsOnStatus {
	case createCondition:
		// The 'target' container desires to be moved to 'Created' or the 'steady' state.
		// Allow this only if the known status of the dependency container state is already started
		// i.e it's state is any of 'Created', 'steady state' or 'Stopped'
		return dependsOnContainerKnownStatus >= apicontainerstatus.ContainerCreated

	case startCondition:
		if targetDesiredStatus == apicontainerstatus.ContainerCreated {
			// The 'target' container desires to be moved to 'Created' state.
			// Allow this only if the known status of the linked container is
			// 'Created' or if the dependency container is in 'steady state'
			return dependsOnContainerKnownStatus == apicontainerstatus.ContainerCreated || dependsOnContainer.IsKnownSteadyState()
		} else if targetDesiredStatus == target.GetSteadyStateStatus() {
			// The 'target' container desires to be moved to its 'steady' state.
			// Allow this only if the dependency container is in 'steady state' as well
			return dependsOnContainer.IsKnownSteadyState()
		}
		return false

	case successCondition:
		// The 'target' container desires to be moved to 'Created' or the 'steady' state.
		// Allow this only if the known status of the dependency container state is stopped with an exit code of 0
		if dependsOnContainer.GetKnownExitCode() != nil {
			return dependsOnContainerKnownStatus == apicontainerstatus.ContainerStopped &&
				*dependsOnContainer.GetKnownExitCode() == successExitCode
		}
		return false

	case completeCondition:
		// The 'target' container desires to be moved to 'Created' or the 'steady' state.
		// Allow this only if the known status of the dependency container state is stopped with any exit code
		return dependsOnContainerKnownStatus == apicontainerstatus.ContainerStopped && dependsOnContainer.GetKnownExitCode() != nil

	case healthyCondition:
		return dependsOnContainer.HealthStatusShouldBeReported() &&
			dependsOnContainer.GetHealthStatus().Status == apicontainerstatus.ContainerHealthy

	default:
		return false
	}
}

func hasDependencyTimedOut(dependOnContainer *apicontainer.Container, dependencyCondition string) bool {
	if dependOnContainer.GetStartedAt().IsZero() || dependOnContainer.GetStartTimeout() <= 0 {
		return false
	}
	switch dependencyCondition {
	case successCondition, completeCondition, healthyCondition:
		return time.Now().After(dependOnContainer.GetStartedAt().Add(dependOnContainer.GetStartTimeout()))
	default:
		return false
	}
}

func hasDependencyStoppedSuccessfully(dependency *apicontainer.Container) bool {
	p := dependency.GetKnownExitCode()
	return p != nil && *p == 0
}

func verifyContainerOrderingStatus(dependsOnContainer *apicontainer.Container) bool {
	dependsOnContainerDesiredStatus := dependsOnContainer.GetDesiredStatus()
	// The 'target' container desires to be moved to 'Created' or the 'steady' state.
	// Allow this only if the dependency container also desires to be started
	// i.e it's status is any of 'Created', 'steady state' or 'Stopped'
	return dependsOnContainerDesiredStatus == apicontainerstatus.ContainerCreated ||
		dependsOnContainerDesiredStatus == apicontainerstatus.ContainerStopped ||
		dependsOnContainerDesiredStatus == dependsOnContainer.GetSteadyStateStatus()
}

func verifyShutdownOrder(target *apicontainer.Container, existingContainers map[string]*apicontainer.Container) DependencyError {
	// We considered adding this to the task state, but this will be at most 45 loops,
	// so we err'd on the side of having less state.
	missingShutdownDependencies := []string{}

	for _, existingContainer := range existingContainers {
		dependencies := existingContainer.GetDependsOn()
		for _, dependency := range dependencies {
			// If another container declares a dependency on our target, we will want to verify that the container is
			// stopped.
			if dependency.ContainerName == target.Name {
				if !existingContainer.KnownTerminal() {
					missingShutdownDependencies = append(missingShutdownDependencies, existingContainer.Name)
				}
			}
		}
	}

	if len(missingShutdownDependencies) == 0 {
		return nil
	}

	return &dependencyError{err: fmt.Errorf("dependency graph: target %s needs other containers stopped before it can stop: [%s]",
		target.Name, strings.Join(missingShutdownDependencies, "], ["))}
}

func onSteadyStateCanResolve(target *apicontainer.Container, run *apicontainer.Container) bool {
	return target.GetDesiredStatus() >= apicontainerstatus.ContainerCreated &&
		run.GetDesiredStatus() >= run.GetSteadyStateStatus()
}

// onSteadyStateIsResolved defines a relationship where a target cannot be
// created until 'dependency' has reached the steady state. Transitions include pulling.
func onSteadyStateIsResolved(target *apicontainer.Container, run *apicontainer.Container) bool {
	return target.GetDesiredStatus() >= apicontainerstatus.ContainerCreated &&
		run.GetKnownStatus() >= run.GetSteadyStateStatus()
}
