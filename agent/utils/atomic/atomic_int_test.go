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

package atomic

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestStrictlyIncreasing(t *testing.T) {
	increasingInt := NewIncreasingInt64(10)
	if increasingInt.Get() != 10 {
		t.Fatal("Initial value")
	}
	increasingInt.Set(1)
	if increasingInt.Get() != 10 {
		t.Fatal("Decreased")
	}
	increasingInt.Set(11)
	if increasingInt.Get() != 11 {
		t.Fatal("Set")
	}

	waitAllSet := sync.WaitGroup{}
	for j := int64(30); j > 1; j-- {
		waitAllSet.Add(2)
		go func() {
			increasingInt.Set(j)
			waitAllSet.Done()
		}()
		go func(j int64) {
			increasingInt.Set(j)
			waitAllSet.Done()
		}(j)
	}

	waitAllSet.Wait()
	if increasingInt.Get() != 30 {
		t.Fatal("Decreased", increasingInt.Get())
	}
}

func TestJSONMarshal(t *testing.T) {
	increasingInt := NewIncreasingInt64(100)
	data, err := json.Marshal(increasingInt)
	if err != nil {
		t.Error(err)
	}
	if string(data) != "100" {
		t.Error("Did not marshal as expected")
	}
}

func TestJSONUnmarshal(t *testing.T) {
	increasingInt := NewIncreasingInt64(100)
	err := json.Unmarshal([]byte("101"), &increasingInt)
	if err != nil {
		t.Error(err)
	}
	if increasingInt.Get() != 101 {
		t.Error("Unmarshal should have unmarshalled expected value")
	}
	err = json.Unmarshal([]byte("42"), &increasingInt)
	if err != nil {
		t.Error(err)
	}
	if increasingInt.Get() != 101 {
		t.Error("Unmarshal should not have increased int")
	}
}
