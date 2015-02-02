package ttime

import "time"

// TestTime implements a time that is able to be moved forward easily
type TestTime struct {
	// IsLudicrousSpeed indicates whether sleeps will all succeed instantly
	IsLudicrousSpeed bool

	warped time.Duration
	slept  time.Duration

	// Whenever we warp around or toggle behavior, send a value down this
	// channel so all current calls can check if it affects them
	timeChange chan bool
}

// NewTestTime returns a default TestTime. It will not be LudicrousSpeed and
// will behave like a normal time
func NewTestTime() *TestTime {
	return &TestTime{
		timeChange: make(chan bool),
	}
}

// Warp moves the mock time forwards by the given duration.
func (t *TestTime) Warp(d time.Duration) {
	t.warped += d
	go func() { t.timeChange <- true }()
}

// LudicrousSpeed can be called to toggle LudicrousSpeed (default off). When
// LudicrousSpeed is engaged, all calls to "sleep" will succeed instantly.
func (t *TestTime) LudicrousSpeed(b bool) {
	t.IsLudicrousSpeed = b
	go func() { t.timeChange <- true }()
}

// Now returns the current time, including any time-warping that has occured.
func (t *TestTime) Now() time.Time {
	return time.Now().Add(t.warped)
}

// Sleep sleeps the given duration in mock-time; that is to say that Warps will
// reduce the amount of time slept and LudicrousSpeed will cause instant
// success.
func (t *TestTime) Sleep(d time.Duration) {
	// Calculate the 'real' end time so previously applied Warps work as
	// expected. Add in the 'slept' time to ensure sleeps accumulate time
	endTime := time.Now().Add(t.slept).Add(d)
	done := make(chan bool)

	defer func() {
		t.slept += d
	}()

	for {
		remainingTime := endTime.Sub(t.Now())
		if t.IsLudicrousSpeed {
			t.Warp(remainingTime)
			return
		}

		go func() {
			time.Sleep(remainingTime)
			done <- true
		}()

		select {
		case <-done:
			return
		case <-t.timeChange:
		}
	}
}
