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

// Package atomic implements higher level constructs on top of the stdlib atomic package
package atomic

import (
	"encoding/json"
	"sync/atomic"
)

// IncreasingInt64 is an int64 that can only strictly increase, and does so safely
type IncreasingInt64 struct {
	i int64
}

func NewIncreasingInt64(initial int64) *IncreasingInt64 {
	return &IncreasingInt64{initial}
}

func (ai64 *IncreasingInt64) Get() int64 {
	return ai64.i
}

func (ai64 *IncreasingInt64) Set(val int64) {
	for {
		currentValue := ai64.i
		if currentValue >= val {
			// Only can increase; no need to change
			return
		}
		if atomic.CompareAndSwapInt64(&ai64.i, currentValue, val) {
			return
		}
	}
}

func (ai64 *IncreasingInt64) MarshalJSON() ([]byte, error) {
	return json.Marshal(ai64.Get())
}

func (ai64 *IncreasingInt64) UnmarshalJSON(data []byte) error {
	var val int64
	err := json.Unmarshal(data, &val)
	if err != nil {
		return err
	}
	ai64.Set(val)
	return nil
}
