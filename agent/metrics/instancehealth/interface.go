package instancehealth

import "github.com/aws/amazon-ecs-agent/agent/api/errors"

// InstanceHealth interface defines the behaviour of any counter
// that uses genericCounter to collect the metrics used to determine
// instance's health
// As a starting point we will use this interface to collect Docker API
// metrics. In future it can be extended to different counters.
type InstanceHealth interface {

	// This function increments the API call count
	IncrementCallCount()

	// Records the error message and increments the API's error count
	RecordError(errors.NamedError)

	//  Returns the error message and resets to empty
	GetAndResetErrorMessage() string

	// This function returns API call and error count resets their count
	// Returns false if there have been less than 10 calls recorded because
	// this represents an insufficient number of samples.
	GetAndResetCount() (callCount int64, errCount int64, ok bool)
}
