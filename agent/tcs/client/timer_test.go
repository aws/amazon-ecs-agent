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

import (
	"testing"
	"time"
)

func incr(p interface{}) error {
	iptr := p.(*int)
	*iptr = *iptr + 1
	return nil
}

func TestTimer(t *testing.T) {
	tkr := newTimer(1*time.Millisecond, incr)
	i := 0
	go tkr.start(&i)
	time.Sleep(5 * time.Millisecond)
	tkr.stop()
	time.Sleep(2 * time.Millisecond)
	if i == 0 || i > 5 {
		t.Error("Incorrect result from tick function: ", i)
	}
}
