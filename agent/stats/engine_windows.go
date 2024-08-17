//go:build windows
// +build windows

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

package stats

import "time"

const (
	// publishMetricsTimeout is the duration that we wait for metrics/health info to be
	// pushed to the TCS channels. In theory, this timeout should never be hit since
	// the TCS handler should be continually reading from the channels and pushing to
	// TCS, but when we lose connection to TCS, these channels back up. In case this
	// happens, we need to have a timeout to prevent statsEngine channels from blocking.
	// The times on Linux and Windows vary from one another and that is why we have
	// them separated out in their respective modules.
	publishMetricsTimeout = 6 * time.Second

	// getVolumeMetricsTimeout is the time that we want for the metrics to be fetched from
	// the local disk. The times on Linux and Windows vary from one another and that is
	// why we have them separated out in their respective modules.
	getVolumeMetricsTimeout = 5 * time.Second
)
