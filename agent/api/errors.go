package api

import "errors"

// Implements Error & Retriable
type StateChangeError struct {
	err       error
	Retriable bool
}

func NewStateChangeError(message string, retriable bool) *StateChangeError {
	return &StateChangeError{errors.New(message), retriable}
}

func (sce *StateChangeError) Retry() bool {
	return sce.Retriable
}

func (sce *StateChangeError) Error() string {
	return sce.err.Error()
}
