package utils

import (
	"testing"
	"time"
)

func TestSimpleBackoff(t *testing.T) {
	sb := NewSimpleBackoff(10*time.Second, time.Minute, 0, 2)

	for i := 0; i < 2; i++ {
		duration := sb.Duration()
		if duration.Nanoseconds() != 10*time.Second.Nanoseconds() {
			t.Error("Initial duration incorrect. Got ", duration.Nanoseconds())
		}

		duration = sb.Duration()
		if duration.Nanoseconds() != 20*time.Second.Nanoseconds() {
			t.Error("Increase incorrect")
		}
		_ = sb.Duration() // 40s
		duration = sb.Duration()
		if duration.Nanoseconds() != 60*time.Second.Nanoseconds() {
			t.Error("Didn't stop at maximum")
		}
		sb.Reset()
		// loop to redo the above tests after resetting, they should be the same
	}
}

func TestJitter(t *testing.T) {
	for i := 0; i < 10; i++ {
		duration := AddJitter(10*time.Second, 3*time.Second)
		if duration < 10*time.Second || duration > 13*time.Second {
			t.Error("Excessive amount of jitter", duration)
		}
	}
}
