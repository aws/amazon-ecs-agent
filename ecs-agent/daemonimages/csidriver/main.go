// this file has been modified from its original found in:
// https://github.com/kubernetes-sigs/aws-ebs-csi-driver

/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"os"

	"k8s.io/klog/v2"

	"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/driver"
	"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/health"
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

	if srvOptions.HealthCheck {
		// Perform health check and exit
		err := health.CheckHealth(srvOptions.Endpoint)
		if err != nil {
			klog.Errorf("Health check failed: %s", err.Error())
			os.Exit(1)
		}
		os.Exit(0)
	}

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
