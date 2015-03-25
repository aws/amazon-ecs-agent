// Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
)

//go:generate mockgen.sh $GOPACKAGE $GOFILE

type downloader interface {
	IsAgentCached() bool
	IsAgentLatest() bool
	LoadCachedAgent() (io.ReadCloser, error)
	DownloadAgent() error
}

type dockerClient interface {
	IsAgentImageLoaded() (bool, error)
	LoadImage(image io.Reader) error
	RemoveExistingAgentContainer() error
	StartAgent() (int, error)
	StopAgent() error
}
