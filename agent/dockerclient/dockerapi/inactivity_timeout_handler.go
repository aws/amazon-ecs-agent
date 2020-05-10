// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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

package dockerapi

import (
	"io"
	"sync/atomic"
	"time"
)

type proxyReader struct {
	io.ReadCloser
	calls uint64
}

func (p *proxyReader) callCount() uint64 {
	return atomic.LoadUint64(&p.calls)
}

func (p *proxyReader) Read(data []byte) (int, error) {
	atomic.AddUint64(&p.calls, 1)
	return p.ReadCloser.Read(data)
}

// When pulling an image, the docker api will pull and then subsequently unzip the downloaded artifacts. Docker does
// not separate the "pull" from the "unpack" step. What this means is that this timeout doesn't 'tick' while unpacking
// the downloaded files. This only causes noticeable impact with large files, but we should investigate improving this.
func handleInactivityTimeout(reader io.ReadCloser, timeout time.Duration, cancelRequest func(), canceled *uint32) (io.ReadCloser, chan<- struct{}) {
	done := make(chan struct{})
	pReader := &proxyReader{ReadCloser: reader}
	go checkInactivityTimeout(pReader, timeout, cancelRequest, canceled, done)
	return pReader, done
}

// checkInactivityTimeout checks whether there's new read activity in the proxyReader as recorded by its call count, in every interval
// as specified by the timeout argument. It returns either when there's no new read activity in an interval (in that case the
// 'canceled' flag is marked to 1 to indicate that), or when it's canceled (via the 'done' channel).
func checkInactivityTimeout(pr *proxyReader, timeout time.Duration, cancelRequest func(), canceled *uint32, done chan struct{}) {
	var lastCallCount uint64
	finished := false
	for !finished {
		lastCallCount, finished = checkReadActivityOnce(pr, timeout, cancelRequest, canceled, done, lastCallCount)
	}
}

func checkReadActivityOnce(pr *proxyReader, timeout time.Duration, cancelRequest func(), canceled *uint32, done chan struct{}, lastCallCount uint64) (uint64, bool) {
	select {
	case <-time.After(timeout):
	case <-done:
		return lastCallCount, true
	}
	curCallCount := pr.callCount()
	if curCallCount == lastCallCount {
		atomic.AddUint32(canceled, 1)
		cancelRequest()
		return lastCallCount, true
	}
	return curCallCount, false
}
