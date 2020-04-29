package instancehealth

import (
	"strings"
	"sync"
	"sync/atomic"

	"github.com/aws/amazon-ecs-agent/agent/api/errors"
)

// genericCounter records call and error counts of API calls
type genericMetric struct {
	calls        int64
	errors       int64
	errorMessage string
	mu           sync.Mutex
}

const MinimumSampleCount int64 = 10

var DockerMetric *genericMetric

func init() {
	DockerMetric = &genericMetric{}
}

func (gc *genericMetric) IncrementCallCount() {
	atomic.AddInt64(&gc.calls, 1)
}

func (gc *genericMetric) RecordError(errorMessage errors.NamedError) {
	atomic.AddInt64(&gc.errors, 1)
	newMsg := errorMessage.ErrorName() + ": " + errorMessage.Error() + " -- "
	gc.mu.Lock()
	if !strings.Contains(gc.errorMessage, newMsg) {
		gc.errorMessage = gc.errorMessage + newMsg
	}
	gc.mu.Unlock()
}

func (gc *genericMetric) GetAndResetErrorMessage() string {
	gc.mu.Lock()
	msg := gc.errorMessage
	gc.errorMessage = ""
	gc.mu.Unlock()
	return msg
}

func (gc *genericMetric) GetAndResetCount() (callCount int64, errCount int64, ok bool) {
	if atomic.LoadInt64(&gc.calls) < MinimumSampleCount {
		return 0, 0, false
	}
	callCount = atomic.SwapInt64(&gc.calls, 0)
	errCount = atomic.SwapInt64(&gc.errors, 0)
	return callCount, errCount, true
}
