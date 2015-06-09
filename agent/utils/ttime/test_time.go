package ttime

import (
	"sync"
	"time"
)

// TestTime implements a time that is able to be moved forward easily
type TestTime struct {
	// IsLudicrousSpeed indicates whether sleeps will all succeed instantly
	IsLudicrousSpeed bool

	warped time.Duration
	slept  time.Duration

	// Whenever we warp around or toggle LudicrousSpeed, broadcast to any waiting sleeps so they can recalculate remaining time
	timeChange *sync.Cond
}

// NewTestTime returns a default TestTime. It will not be LudicrousSpeed and
// will behave like a normal time
func NewTestTime() *TestTime {
	return &TestTime{
		timeChange: sync.NewCond(&sync.Mutex{}),
	}
}

// Warp moves the mock time forwards by the given duration.
func (t *TestTime) Warp(d time.Duration) {
	t.warped += d
	t.timeChange.Broadcast()
}

// LudicrousSpeed can be called to toggle LudicrousSpeed (default off). When
// LudicrousSpeed is engaged, all calls to "sleep" will succeed instantly.
func (t *TestTime) LudicrousSpeed(b bool) {
	t.IsLudicrousSpeed = b
	t.timeChange.Broadcast()
}

// Now returns the current time, including any time-warping that has occured.
func (t *TestTime) Now() time.Time {
	return time.Now().Add(t.warped)
}

// After returns a channel which is written to after the given duration, taking
// into account time-warping
func (t *TestTime) After(d time.Duration) <-chan time.Time {
	done := make(chan time.Time, 1)
	go func() {
		t.Sleep(d)
		done <- t.Now()
	}()
	return done
}

// AfterFunc returns a timer and calls a function after a given time
// taking into account time-warping
func (t *TestTime) AfterFunc(d time.Duration, f func()) *time.Timer {
	timer := time.AfterFunc(d, f)
	go func() {
		t.Sleep(d)
		timer.Reset(0)
	}()
	return timer
}

// Sleep sleeps the given duration in mock-time; that is to say that Warps will
// reduce the amount of time slept and LudicrousSpeed will cause instant
// success.
func (t *TestTime) Sleep(d time.Duration) {
	defer func() {
		t.slept += d
		t.timeChange.Broadcast()
	}()

	t.timeChange.L.Lock()
	defer t.timeChange.L.Unlock()

	// Calculate the 'real' end time so previously applied Warps work as
	// expected. Add in the 'slept' time to ensure sleeps accumulate time
	endTime := time.Now().Add(t.slept).Add(d)
	for remainingTime := endTime.Sub(t.Now()); remainingTime > 0; remainingTime = endTime.Sub(t.Now()) {
		if t.IsLudicrousSpeed {
			t.Warp(remainingTime)
			break
		}
		go func(remaining time.Duration) {
			time.Sleep(remainingTime)
			t.timeChange.Broadcast()
		}(remainingTime)

		t.timeChange.Wait()
	}
}
