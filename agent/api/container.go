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

package api

const DOCKER_MINIMUM_MEMORY = 4 * 1024 * 1024 // 4MB

// Overriden returns
func (c *Container) Overridden() *Container {
	result := *c

	// We only support Command overrides at the moment
	if result.Overrides.Command != nil {
		result.Command = *c.Overrides.Command
	}

	return &result
}

func (c *Container) KnownTerminal() bool {
	return c.GetKnownStatus().Terminal()
}

func (c *Container) DesiredTerminal() bool {
	return c.GetDesiredStatus().Terminal()
}

func (c *Container) GetKnownStatus() ContainerStatus {
	c.knownStatusLock.RLock()
	defer c.knownStatusLock.RUnlock()

	return c.KnownStatus
}

func (c *Container) SetKnownStatus(status ContainerStatus) {
	c.knownStatusLock.Lock()
	defer c.knownStatusLock.Unlock()

	c.KnownStatus = status
}

func (c *Container) GetDesiredStatus() ContainerStatus {
	c.desiredStatusLock.RLock()
	defer c.desiredStatusLock.RUnlock()

	return c.DesiredStatus
}

func (c *Container) SetDesiredStatus(status ContainerStatus) {
	c.desiredStatusLock.Lock()
	defer c.desiredStatusLock.Unlock()

	c.DesiredStatus = status
}
