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

package ecr

import (
	"testing"
)

func TestClientCachingSameClient(t *testing.T) {
	region := "us-west-2"
	endpoint := ""
	factory := &ecrFactory{
		clients: make(map[cacheKey]ECRSDK),
	}

	sdk1 := factory.GetClient(region, endpoint)
	sdk2 := factory.GetClient(region, endpoint)

	if sdk1 == nil || sdk2 == nil {
		t.Error("Should not be nil")
	}
	if sdk1 != sdk2 {
		t.Errorf("Should be the same, but was %v and %v", sdk1, sdk2)
	}
}

func TestClientCachingDifferentClients(t *testing.T) {
	region1 := "us-west-2"
	endpoint1 := ""
	region2 := "us-east-1"
	endpoint2 := endpoint1
	region3 := region1
	endpoint3 := "different.endpoint"
	factory := &ecrFactory{
		clients: make(map[cacheKey]ECRSDK),
	}

	sdk1 := factory.GetClient(region1, endpoint1)
	sdk2 := factory.GetClient(region2, endpoint2)
	sdk3 := factory.GetClient(region3, endpoint3)

	if sdk1 == nil || sdk2 == nil || sdk3 == nil {
		t.Error("Should not be nil")
	}
	if sdk1 == sdk2 || sdk2 == sdk3 || sdk1 == sdk3 {
		t.Errorf("Should be the different, but was %v, %v, and %v", sdk1, sdk2, sdk3)
	}
}
