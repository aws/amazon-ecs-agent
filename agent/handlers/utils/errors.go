package utils

// TransientError represents a transient error when accessing task metadata endpoint
type TransientError struct {
	Msg string
}

func (e *TransientError) Error() string {
	return e.Msg
}

// IsTransient returns true if the error is transient
func IsTransient(err error) bool {
	_, ok := err.(*TransientError)
	return ok
}
