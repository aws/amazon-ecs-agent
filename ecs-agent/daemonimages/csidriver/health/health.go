// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//      http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

// This package contains utilities related to the health of the CSI Driver.
package health

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/util"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/klog/v2"
)

const (
	defaultResponseTimeout = 5 * time.Second
)

// Checks the health of an already running CSI Driver Server by querying for
// Node Service capabilities.
func CheckHealth(endpoint string) error {
	// Parse the endpoint
	scheme, endpoint, err := util.ParseEndpointNoRemove(endpoint)
	if err != nil {
		return fmt.Errorf("failed to parse endpoint: %w", err)
	}

	// Connect to the server
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return net.Dial(scheme, addr)
	}
	conn, err := grpc.Dial(
		endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(dialer),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to the server: %w", err)
	}
	defer conn.Close()

	// Call the server to fetch node capabilities
	client := csi.NewNodeClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), defaultResponseTimeout)
	defer cancel()
	res, err := client.NodeGetCapabilities(ctx, &csi.NodeGetCapabilitiesRequest{})
	if err != nil {
		return fmt.Errorf("failed to get node capabilities: %w", err)
	}
	klog.Infof("Node capabilities: %s", res.String())

	return nil
}
