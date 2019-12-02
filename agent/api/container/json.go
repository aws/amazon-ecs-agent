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
