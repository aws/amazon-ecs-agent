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

// Package imds provides an IMDS credential scanner that retrieves task
// credentials from IMDS.
package imds

import (
	"context"

	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
)

// Scanner fetches task credentials from IMDS iam-ecs-* namespaces.
type Scanner interface {
	// Scan discovers iam-ecs-* namespaces, reads their info files, and fetches
	// credentials only for task IDs present in the provided set. Credentials
	// for unknown task IDs are ignored.
	Scan(ctx context.Context, taskIDs map[string]bool) ([]TaskCredential, error)
}

// scanner implements the Scanner interface.
type scanner struct {
	ec2Client ec2.EC2MetadataClient
}

// NewScanner creates a new IMDS credential scanner.
func NewScanner(ec2Client ec2.EC2MetadataClient) Scanner {
	return &scanner{
		ec2Client: ec2Client,
	}
}

// Scan is a stub to satisfy the Scanner interface.
// TODO: implementation
func (s *scanner) Scan(ctx context.Context, taskIDs map[string]bool) ([]TaskCredential, error) {
	return nil, nil
}
