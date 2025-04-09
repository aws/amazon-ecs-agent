// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//    http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package fsx

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/fsx"
	"github.com/pkg/errors"
)

// GetFileSystemDNSNames returns a map of filesystem ids and corresponding dns names
// Example: key := fs-12345678, value :=  amznfsxujfqr2nj.test.com
func GetFileSystemDNSNames(fileSystemIds []string, client FSxClient) (map[string]string, error) {
	out, err := describeFileSystems(fileSystemIds, client)
	if err != nil {
		return nil, err
	}

	fileSystemDNSMap := make(map[string]string)
	for _, filesystem := range out.FileSystems {
		fileSystemDNSMap[aws.ToString(filesystem.FileSystemId)] = aws.ToString(filesystem.DNSName)
	}

	return fileSystemDNSMap, nil
}

// describeFileSystems makes the api call to the AWS FSx service to retrieve filesystems info
func describeFileSystems(fileSystemIds []string, client FSxClient) (*fsx.DescribeFileSystemsOutput, error) {
	in := &fsx.DescribeFileSystemsInput{
		FileSystemIds: fileSystemIds,
	}

	out, err := client.DescribeFileSystems(context.TODO(), in)
	if err != nil {
		return nil, errors.Wrapf(err, "fsx describing filesystem(s) from the service for %v", fileSystemIds)
	}

	return out, nil
}
