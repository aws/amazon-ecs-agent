package ttime

import (
	"sync"
	"time"

	log "github.com/cihub/seelog"
	"golang.org/x/net/context"
)

// TestTime implements a time that is able to be moved forward easily
type TestTime struct {
	// IsLudicrousSpeed indicates whether sleeps will all succeed instantly
	IsLudicrousSpeed bool

	warped time.Duration
	slept  time.Duration

	// Whenever we warp around or toggle LudicrousSpeed, broadcast to any waiting sleeps so they can recalculate remaining time
	timeChange *sync.Cond

	ctx    context.Context
	cancel func()
}

// NewTestTime returns a default TestTime. It will not be LudicrousSpeed and
// will behave like a normal time
func NewTestTime() *TestTime {
	ctx, cancel := context.WithCancel(context.Background())
	return &TestTime{
		timeChange: sync.NewCond(&sync.Mutex{}),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Warp moves the mock time forwards by the given duration.
func (t *TestTime) Warp(d time.Duration) {
	t.warped += d
	log.Criticalf("WARPED TIME: %v", d)
	t.timeChange.Broadcast()
}

// Cancel cancels all pending sleeps
func (t *TestTime) Cancel() {
	t.cancel()
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

type testTimer struct {
	active    bool
	expiresAt time.Time
	duration  time.Duration
	f         func()
	testTime  *TestTime
	sync.Mutex
}

func (timer *testTimer) Reset(d time.Duration) bool {
	timer.Lock()
	defer timer.Unlock()

	active := timer.active
	timer.active = true
	timer.expiresAt = timer.testTime.Now().Add(d)
	timer.duration = d
	go timer.run()
	return active
}

func (timer *testTimer) Stop() bool {
	timer.Lock()
	defer timer.Unlock()

	active := timer.active
	timer.active = false
	return active
}

func (timer *testTimer) run() {
	timer.testTime.Sleep(timer.duration)
	timer.Lock()
	defer timer.Unlock()
	if timer.active && timer.testTime.Now().Sub(timer.expiresAt) >= 0 {
		timer.active = false
		timer.f()
	}
}

// AfterFunc returns a timer and calls a function after a given time
// taking into account time-warping
func (t *TestTime) AfterFunc(d time.Duration, f func()) Timer {
	timer := &testTimer{
		active:    true,
		expiresAt: t.Now().Add(d),
		f:         f,
		testTime:  t,
		duration:  d,
	}
	go timer.run()
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
	timer := time.NewTimer(endTime.Sub(t.Now()))
	done := make(chan bool, 1)
	for remainingTime := endTime.Sub(t.Now()); remainingTime > 0; remainingTime = endTime.Sub(t.Now()) {
		select {
		case <-t.ctx.Done():
			return
		default:
		}
		if t.IsLudicrousSpeed {
			t.Warp(remainingTime)
			break
		}
		timer.Reset(remainingTime)
		go func(remaining time.Duration) {
			select {
			case <-timer.C:
				t.timeChange.Broadcast()
			case <-done:
				return
			case <-t.ctx.Done():
				return
			}
		}(remainingTime)

		t.timeChange.Wait()
	}
	timer.Stop()
	done <- true
}
