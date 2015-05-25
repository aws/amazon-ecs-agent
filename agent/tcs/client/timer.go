// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package tcsclient

import "time"

// timerCallback defines the function pointer type for the callback.
type timerCallback func(interface{}) error

// timer invokes a callback function at specified intervals.
type timer struct {
	active     bool
	interval   time.Duration
	invoke     timerCallback
	stopTicker chan bool
}

// start starts the loop to periodically invoke the callback function.
func (t *timer) start(param interface{}) {
	for {
		select {
		case <-t.stopTicker:
			t.setActive(false)
			return
		default:
			tick := time.Tick(t.interval)
			t.setActive(true)
			select {
			case <-tick:
				err := t.invoke(param)
				if err != nil {
					log.Debug("timer invoke", "err", err)
				}
			}
		}
	}
}

// stop stops the timer loop.
func (t *timer) stop() {
	if t.active {
		t.stopTicker <- true
	}
}

// setActive sets the status for the timer.
func (t *timer) setActive(active bool) {
	t.active = active
}

// newTimer creates a new instance of the timer struct.
func newTimer(interval time.Duration, callback timerCallback) *timer {
	return &timer{
		active:     false,
		interval:   interval,
		invoke:     callback,
		stopTicker: make(chan bool),
	}
}
