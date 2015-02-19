// Package ttime implements a testable alternative to the Go "time" package.
package ttime

import "time"

// Time represents an implementation for this package's methods
type Time interface {
	Now() time.Time
	Sleep(d time.Duration)
}

// DefaultTime is a Time that behaves normally
type DefaultTime struct{}

var _time Time = &DefaultTime{}

// Now returns the current time
func (*DefaultTime) Now() time.Time {
	return time.Now()
}

// Sleep sleeps for the given duration
func (*DefaultTime) Sleep(d time.Duration) {
	time.Sleep(d)
}

// SetTime configures what 'Time' implementation to use for each of the
// package-level methods.
func SetTime(t Time) {
	_time = t
}

// Now returns the implementation's current time
func Now() time.Time {
	return _time.Now()
}

// Sleep calls the implementation's Sleep method
func Sleep(d time.Duration) {
	_time.Sleep(d)
}

// Since returns the time different from Now and the given time t
func Since(t time.Time) time.Duration {
	return _time.Now().Sub(t)
}
