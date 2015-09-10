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

package ecs_credentials

import (
	"encoding/base64"
	"reflect"
	"testing"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/credentialprovider"
	"github.com/aws/amazon-ecs-agent/agent/config"
)

type testPair struct {
	input  config.Config
	output credentialprovider.DockerConfig
}

var testPairs = []testPair{
	testPair{
		config.Config{
			EngineAuthType: "docker",
			EngineAuthData: config.NewSensitiveRawMessage([]byte(`{"https://index.docker.io/v1/":{"username":"user","password":"swordfish","email":"user@email"}}`)),
		},
		credentialprovider.DockerConfig{
			"https://index.docker.io/v1/": credentialprovider.DockerConfigEntry{
				Username: "user",
				Password: "swordfish",
				Email:    "user@email",
			},
		},
	},
	testPair{
		config.Config{
			EngineAuthType: "dockercfg",
			EngineAuthData: config.NewSensitiveRawMessage([]byte(`{"my.registry.tld":{"auth":"` + base64.StdEncoding.EncodeToString([]byte("test:dragon")) + `","email":"test@email"}}`)),
		},
		credentialprovider.DockerConfig{
			"my.registry.tld": credentialprovider.DockerConfigEntry{
				Username: "test",
				Password: "dragon",
				Email:    "test@email",
			},
		},
	},
	testPair{
		config.Config{
			EngineAuthType: "dockercfg",
			EngineAuthData: config.NewSensitiveRawMessage([]byte(`{"my.registry.tld":{"auth":"` + base64.StdEncoding.EncodeToString([]byte("test:dragon")) + `","email":"test@email"},` +
				`"my-other-registry.tld":{"auth":"` + base64.StdEncoding.EncodeToString([]byte("other:creds")) + `"}}`)),
		},
		credentialprovider.DockerConfig{
			"my.registry.tld": credentialprovider.DockerConfigEntry{
				Username: "test",
				Password: "dragon",
				Email:    "test@email",
			},
			"my-other-registry.tld": credentialprovider.DockerConfigEntry{
				Username: "other",
				Password: "creds",
			},
		},
	},
}

func TestProvide(t *testing.T) {
	for ndx, pair := range testPairs {
		SetConfig(&pair.input)
		provides := ecsCredentialsInstance.Provide()
		if !reflect.DeepEqual(provides, pair.output) {
			t.Errorf("Failed #%v: Got %v, expected %v", ndx, provides, pair.output)
		}
	}
}

var failingProvides = []config.Config{
	config.Config{
		EngineAuthType: "unknown",
		EngineAuthData: config.NewSensitiveRawMessage([]byte(`{"my.registry.tld":{"auth":"` + base64.StdEncoding.EncodeToString([]byte("test:dragon")) + `","email":"test@email"}}`)),
	},
	config.Config{
		EngineAuthType: "dockercfg",
		EngineAuthData: config.NewSensitiveRawMessage([]byte(`{"my.registry.tld":{"auth":"malformedbase64data"}}`)),
	},
}

func TestFailingProvides(t *testing.T) {
	for ndx, failing := range failingProvides {
		SetConfig(&failing)
		provides := ecsCredentialsInstance.Provide()
		if !reflect.DeepEqual(provides, credentialprovider.DockerConfig{}) {
			t.Errorf("Failed #%v: Got %v, expected nothing", ndx, provides)
		}
	}

}

func TestEnabled(t *testing.T) {
	if !ecsCredentialsInstance.Enabled() {
		t.Error("Should be enabled")
	}
}
