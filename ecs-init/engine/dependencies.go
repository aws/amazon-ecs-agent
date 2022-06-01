// Copyright 2015-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package engine

import (
	"io"

	"github.com/aws/amazon-ecs-agent/ecs-init/cache"
)

//go:generate mockgen.sh $GOPACKAGE $GOFILE

type downloader interface {
	IsAgentCached() bool
	DownloadAgent() error
	LoadCachedAgent() (io.ReadCloser, error)
	LoadDesiredAgent() (io.ReadCloser, error)
	RecordCachedAgent() error
	AgentCacheStatus() cache.CacheStatus
}

type dockerClient interface {
	GetContainerLogTail(logWindowSize string) string
	IsAgentImageLoaded() (bool, error)
	LoadImage(image io.Reader) error
	RemoveExistingAgentContainer() error
	StartAgent() (int, error)
	StopAgent() error
	LoadEnvVars() map[string]string
}

type loopbackRouting interface {
	Enable() error
	RestoreDefault() error
}

type credentialsProxyRoute interface {
	Create() error
	Remove() error
}

type ipv6RouterAdvertisements interface {
	Disable() error
}
