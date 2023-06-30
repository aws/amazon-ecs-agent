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

package main

import (
	"flag"

	"k8s.io/klog/v2"

	"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/driver"
)

func main() {
	fs := flag.NewFlagSet("csi-driver", flag.ExitOnError)
	klog.InitFlags(fs)
	srvOptions, err := GetServerOptions(fs)
	if err != nil {
		klog.ErrorS(err, "Failed to get the server options")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	klog.V(4).InfoS("Server Options are provided", "ServerOptions", srvOptions)

	drv, err := driver.NewDriver(
		driver.WithEndpoint(srvOptions.Endpoint),
	)
	if err != nil {
		klog.ErrorS(err, "Failed to create driver")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	if err := drv.Run(); err != nil {
		klog.ErrorS(err, "Failed to run driver")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}
}
