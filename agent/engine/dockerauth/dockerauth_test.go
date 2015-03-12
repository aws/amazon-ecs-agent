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

package dockerauth

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/credentialprovider"
	"github.com/aws/amazon-ecs-agent/agent/config"
)

type authTestPair struct {
	Image         string
	ExpectedUser  string
	ExpectedPass  string
	ExpectedEmail string
}

// Intentionally don't test any real images to avoid overlap with the user's
// '.dockercfg' file since this uses a a normal DockerKeyring
var expectedPairs = []authTestPair{
	authTestPair{"example.tld/my/image", "user", "swordfish", "user2@example.com"},
	authTestPair{"example.tld/user2/image", "test", "dragon", "test@test.test"},
	authTestPair{"registry.tld/image", "", "", ""},
}

var secretAuth string = base64.StdEncoding.EncodeToString([]byte("user:swordfish"))
var dragonAuth string = base64.StdEncoding.EncodeToString([]byte("test:dragon"))

func TestDockerCfgAuth(t *testing.T) {
	authData := []byte(strings.Join(append([]string{`{`},
		`"example.tld/user2":{"auth":"`+dragonAuth+`","email":"test@test.test"},`,
		`"example.tld":{"auth":"`+secretAuth+`","email":"user2@example.com"}`,
		`}`), ""))
	// Avoid accidentally loading the test-runner's .dockercfg
	credentialprovider.SetPreferredDockercfgPath("/dev/null")
	SetConfig(&config.Config{EngineAuthType: "dockercfg", EngineAuthData: authData})

	for ndx, pair := range expectedPairs {
		authConfig := GetAuthconfig(pair.Image)
		if authConfig.Email != pair.ExpectedEmail || authConfig.Username != pair.ExpectedUser || authConfig.Password != pair.ExpectedPass {
			t.Errorf("Expectation failure: #%v. Got %v, wanted %v", ndx, authConfig, pair)
		}
	}
}
