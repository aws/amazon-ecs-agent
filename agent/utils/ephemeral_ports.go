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

package utils

import (
	"fmt"
	"math/rand"
	"time"
)

// From https://www.kernel.org/doc/html/latest//networking/ip-sysctl.html#ip-variables
const (
	EphemeralPortMin         = 32768
	EphemeralPortMax         = 60999
	maxPortSelectionAttempts = 100
)

var (
	// Injection point for UTs
	randIntFunc = rand.Intn
)

// GenerateEphemeralPortNumbers generates a list of n unique port numbers in the 32768-60999 range. The resulting port
// number list is guaranteed to not include any port number present in "reserved" parameter.
func GenerateEphemeralPortNumbers(n int, reserved []uint16) ([]uint16, error) {
	toExcludeSet := map[uint16]struct{}{}
	for _, e := range reserved {
		toExcludeSet[e] = struct{}{}
	}
	rand.Seed(time.Now().UnixNano())

	var result []uint16
	var portSelectionAttempts int
	for len(result) < n {
		// The intention of maxPortSelectionAttempts is to avoid a super highly unlikely case where we
		// keep getting ports that collide, thus creating an infinite loop.
		if portSelectionAttempts > maxPortSelectionAttempts {
			return nil, fmt.Errorf("maximum number of attempts to generate unique ports reached")
		}
		port := uint16(randIntFunc(EphemeralPortMax-EphemeralPortMin+1) + EphemeralPortMin)
		if _, ok := toExcludeSet[port]; ok {
			portSelectionAttempts++
			continue
		}
		toExcludeSet[port] = struct{}{}
		result = append(result, port)
	}
	return result, nil
}
