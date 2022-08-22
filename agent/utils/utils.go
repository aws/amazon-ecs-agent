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
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/awserr"

	"github.com/pkg/errors"
	"golang.org/x/net/http/httpproxy"
)

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

func IntPtr(i int) *int {
	return &i
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

func StrSliceContains(strs []string, s string) bool {
	for _, a := range strs {
		if a == s {
			return true
		}
	}
	return false
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

// GetRequestFailureStatusCode returns the status code from a
// RequestFailure error, or 0 if the error is not of that type
func GetRequestFailureStatusCode(err error) int {
	var statusCode int
	if reqErr, ok := err.(awserr.RequestFailure); ok {
		statusCode = reqErr.StatusCode()
	}
	return statusCode
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

// SearchStrInDir searches the files in directory for specific content
func SearchStrInDir(dir, filePrefix, content string) error {
	logfiles, err := ioutil.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("Error reading the directory, err %v", err)
	}

	var desiredFile string
	found := false

	for _, file := range logfiles {
		if strings.HasPrefix(file.Name(), filePrefix) {
			desiredFile = file.Name()
			if ZeroOrNil(desiredFile) {
				return fmt.Errorf("File with prefix: %v does not exist", filePrefix)
			}

			data, err := ioutil.ReadFile(filepath.Join(dir, desiredFile))
			if err != nil {
				return fmt.Errorf("Failed to read file, err: %v", err)
			}

			if strings.Contains(string(data), content) {
				found = true
				break
			}
		}
	}

	if !found {
		return fmt.Errorf("Could not find the content: %v in the file: %v", content, desiredFile)
	}

	return nil
}

// GetTaskID retrieves the task ID from task ARN.
func GetTaskID(taskARN string) (string, error) {
	_, err := arn.Parse(taskARN)
	if err != nil {
		return "", errors.Errorf("failed to get task id: task arn format invalid: %s", taskARN)
	}
	fields := strings.Split(taskARN, "/")
	if len(fields) < 2 {
		return "", errors.Errorf("failed to get task id: task arn format invalid: %s", taskARN)
	}
	return fields[len(fields)-1], nil
}

// GetENIAttachmentId retrieves the attachment ID from eni attachment ARN.
func GetENIAttachmentId(eniAttachmentArn string) (string, error) {
	_, err := arn.Parse(eniAttachmentArn)
	if err != nil {
		return "", errors.Errorf("failed to get eni attachment id: eni attachment arn format invalid: %s", eniAttachmentArn)
	}
	fields := strings.Split(eniAttachmentArn, "/")
	if len(fields) < 2 {
		return "", errors.Errorf("failed to get eni attachment id: eni attachment arn invalid: %s", eniAttachmentArn)
	}
	return fields[len(fields)-1], nil
}

// Proxy is an uncached version of http.ProxyFromEnvironment.
func Proxy(req *http.Request) (*url.URL, error) {
	return httpproxy.FromEnvironment().ProxyFunc()(req.URL)
}
