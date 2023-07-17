package networkinterface

import (
	"fmt"

	"github.com/pkg/errors"
)

// UnableToFindENIError is an error type that is used to handle cases where
// the ENI device cannot be found, even after it has been acknowledged as
// "attached" by the agent. It lets us special case this error in dispatcher
// and task director workflows.
type UnableToFindENIError struct {
	macAddress          string
	associationProtocol string
}

// NewUnableToFindENIError creates a new UnableToFindENIError object.
func NewUnableToFindENIError(macAddress, associationProtocol string) error {
	return &UnableToFindENIError{
		macAddress:          macAddress,
		associationProtocol: associationProtocol,
	}
}

func (e *UnableToFindENIError) Error() string {
	return fmt.Sprintf("unable to find device name for '%s' eni %s",
		e.associationProtocol, e.macAddress)
}

// IsUnableToFindENIError returns true if the error type is of type
// `UnableToFindENIError`.
func IsUnableToFindENIError(err error) bool {
	_, ok := errors.Cause(err).(*UnableToFindENIError)
	return ok
}
