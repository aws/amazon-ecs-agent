package instancehealth

import (
	"fmt"
	"sync/atomic"

	"github.com/aws/amazon-ecs-agent/agent/api/errors"
)

// genericCounter records call and error counts of API calls
type genericMetric struct {
	calls        int64
	errors       int64
	errorMessage atomic.Value
}

var DockerMetric *genericMetric

func init() {
	DockerMetric = &genericMetric{}
}

func (gc *genericMetric) IncrementCallCount() {
	atomic.AddInt64(&gc.calls, 1)
}

func (gc *genericMetric) RecordError(errorMessage errors.NamedError) {
	atomic.AddInt64(&gc.errors, 1)
	gc.errorMessage.Store(errorMessage.ErrorName() + ": " + errorMessage.Error())
}

func (gc *genericMetric) GetErrorMessage() string {
	errMsg := fmt.Sprintf("%v", gc.errorMessage.Load())
	return errMsg
}

func (gc *genericMetric) GetAndResetCount() (int64, int64) {
	callCount := atomic.SwapInt64(&gc.calls, 0)
	errorCount := atomic.SwapInt64(&gc.errors, 0)
	return callCount, errorCount
}
