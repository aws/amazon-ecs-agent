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

package api

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/service"
	"github.com/aws/aws-sdk-go/aws/service/serviceinfo"
)

func TestExtendedRetryMaxDelayHandler(t *testing.T) {
	maxDelayRetries := uint(2)
	maxExtraRetries := uint(10)
	retryCounts := []uint{}
	// inject fake awsAfterRetryHandler to just collect retry counts
	awsAfterRetryHandler = func(r *request.Request) {
		retryCounts = append(retryCounts, r.RetryCount)
		r.Error = nil
	}

	extendedRetryMaxDelayHandler := extendedRetryMaxDelayHandlerFactory(maxExtraRetries)

	request := &request.Request{
		Retryer: service.DefaultRetryer{&service.Service{
			DefaultMaxRetries: maxDelayRetries,
			ServiceInfo: serviceinfo.ServiceInfo{
				Config: &aws.Config{
					MaxRetries: aws.Int(-1),
				},
			},
		}},
	}
	var count uint
	for count = 0; request.Error == nil; count++ {
		request.Error = errors.New("")
		extendedRetryMaxDelayHandler.Fn(request)
	}

	if count != (maxDelayRetries + maxExtraRetries + 1) {
		t.Errorf("Should have been called %d times but was %d", maxDelayRetries+maxExtraRetries+1, count)
	}

	for i := uint(0); i < maxDelayRetries; i++ {
		if retryCounts[i] != i {
			t.Errorf("Expected retry count for attempt %d to be %d, but was %d", i, i, retryCounts[i])
		}
	}

	for i := maxDelayRetries; i < count-1; i++ {
		if retryCounts[i] != maxDelayRetries-1 {
			t.Errorf("Expected retry count for attempt %d to be %d, but was %d", i, maxDelayRetries-1, retryCounts[i])
		}
	}

	if request.RetryCount != maxDelayRetries+maxExtraRetries {
		t.Errorf("Expected retry count for attempt %d to be %d, but was %d", count-1, maxDelayRetries+maxExtraRetries, request.RetryCount)
	}
}
