// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package container

import (
	"encoding/json"
)

// In order to override the MarshalJSON method while still using Go's marshalling logic inside, an alias type
// is needed to avoid infinite recursion.
type jContainer Container

// MarshalJSON wraps Go's marshalling logic with a necessary read lock.
func (c *Container) MarshalJSON() ([]byte, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return json.Marshal((*jContainer)(c))
}
