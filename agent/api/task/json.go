package task

import (
	"encoding/json"
)

// In order to override the MarshalJSON method while still using Go's marshalling logic inside, an alias type
// is needed to avoid infinite recursion.
type jTask Task

// MarshalJSON wraps Go's marshalling logic with a necessary read lock.
func (t *Task) MarshalJSON() ([]byte, error) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return json.Marshal((*jTask)(t))
}
