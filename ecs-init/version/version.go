// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

// Package version contains constants to indicate the current version of the
// ecs-init. The values of the symbols defined here are expected to be
// provided at build time via ldflags, e.g. "-ldflags '-X version.Version 1.2.3'"

package version

// Version is the version of the ecs-init
const Version string

// GitDirty indicates the cleanliness of the git repo when this ecs-init was built
const GitDirty string

// GitShortHash is the short hash of this ecs-init build
const GitShortHash string
