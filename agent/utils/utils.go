// Copyright 2014-2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
)

var log = logger.ForModule("util")

func DefaultIfBlank(str string, default_value string) string {
	if len(str) == 0 {
		return default_value
	}
	return str
}

func ZeroOrNil(obj interface{}) bool {
	value := reflect.ValueOf(obj)
	if !value.IsValid() {
		return true
	}
	if obj == nil {
		return true
	}
	switch value.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		return value.Len() == 0
	}
	zero := reflect.Zero(reflect.TypeOf(obj))
	if !value.Type().Comparable() {
		return false
	}
	if obj == zero.Interface() {
		return true
	}
	return false
}

// SlicesDeepEqual checks if slice1 and slice2 are equal, disregarding order.
func SlicesDeepEqual(slice1, slice2 interface{}) bool {
	s1 := reflect.ValueOf(slice1)
	s2 := reflect.ValueOf(slice2)

	if s1.Len() != s2.Len() {
		return false
	}
	if s1.Len() == 0 {
		return true
	}

	s2found := make([]int, s2.Len())
OuterLoop:
	for i := 0; i < s1.Len(); i++ {
		s1el := s1.Slice(i, i+1)
		for j := 0; j < s2.Len(); j++ {
			if s2found[j] == 1 {
				// We already counted this s2 element
				continue
			}
			s2el := s2.Slice(j, j+1)
			if reflect.DeepEqual(s1el.Interface(), s2el.Interface()) {
				s2found[j] = 1
				continue OuterLoop
			}
		}
		// Couldn't find something unused equal to s1
		return false
	}
	return true
}

func RandHex() string {
	randInt, _ := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	out := make([]byte, 10)
	binary.PutVarint(out, randInt.Int64())
	return hex.EncodeToString(out)
}

func Strptr(s string) *string {
	return &s
}

// Uint16SliceToStringSlice converts a slice of type uint16 to a slice of type
// *string. It uses strconv.Itoa on each element
func Uint16SliceToStringSlice(slice []uint16) []*string {
	stringSlice := make([]*string, len(slice))
	for i, el := range slice {
		str := strconv.Itoa(int(el))
		stringSlice[i] = &str
	}
	return stringSlice
}

func StrSliceEqual(s1, s2 []string) bool {
	if len(s1) != len(s2) {
		return false
	}

	for i := 0; i < len(s1); i++ {
		if s1[i] != s2[i] {
			return false
		}
	}
	return true
}

func ParseBool(str string, default_ bool) bool {
	res, err := strconv.ParseBool(strings.TrimSpace(str))
	if err != nil {
		return default_
	}
	return res
}

// IsAWSErrorCodeEqual returns true if the err implements Error
// interface of awserr and it has the same error code as
// the passed in error code.
func IsAWSErrorCodeEqual(err error, code string) bool {
	awsErr, ok := err.(awserr.Error)
	return ok && awsErr.Code() == code
}

// MapToTags converts a map to a slice of tags.
func MapToTags(tagsMap map[string]string) []*ecs.Tag {
	tags := make([]*ecs.Tag, 0)
	if tagsMap == nil {
		return tags
	}

	for key, value := range tagsMap {
		tags = append(tags, &ecs.Tag{
			Key:   aws.String(key),
			Value: aws.String(value),
		})
	}

	return tags
}
