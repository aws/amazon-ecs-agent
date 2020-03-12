// +build unit

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

package retry

import (
	"testing"
	"time"
)

func TestJitter(t *testing.T) {
	for i := 0; i < 10; i++ {
		duration := AddJitter(10*time.Second, 3*time.Second)
		if duration < 10*time.Second || duration > 13*time.Second {
			t.Error("Excessive amount of jitter", duration)
		}
	}
}
