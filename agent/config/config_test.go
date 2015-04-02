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

package config

import "testing"

func TestMerge(t *testing.T) {
	conf1 := &Config{Cluster: "Foo"}
	conf2 := Config{Cluster: "ignored", APIEndpoint: "Bar"}
	conf3 := Config{AWSRegion: "us-west-2"}

	conf1.Merge(conf2).Merge(conf3)

	if conf1.Cluster != "Foo" {
		t.Error("The cluster should not have been overridden")
	}
	if conf1.APIEndpoint != "Bar" {
		t.Error("The APIEndpoint should have been merged in")
	}
	if conf1.AWSRegion != "us-west-2" {
		t.Error("Incorrect region")
	}
}
