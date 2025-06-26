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

package factory

import (
	"context"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	s3client "github.com/aws/amazon-ecs-agent/agent/s3"
	agentversion "github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/httpclient"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	s3manager "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	roundtripTimeout = 5 * time.Second
)

type S3ClientCreator interface {
	NewS3ManagerClient(bucket, region string, creds credentials.IAMRoleCredentials, ipCompatibility ipcompatibility.IPCompatibility) (s3client.S3ManagerClient, error)
	NewS3Client(bucket, region string, creds credentials.IAMRoleCredentials, ipCompatibility ipcompatibility.IPCompatibility) (s3client.S3Client, error)
}

// NewS3ClientCreator provides 2 implementations
// NewS3ManagerClient implements methods from S3 manger of the AWS SDK Go.
// NewS3Client implements methods from S3 service of the AWS SDK Go.
func NewS3ClientCreator() S3ClientCreator {
	return &s3ClientCreator{}
}

type s3ClientCreator struct{}

func isS3FIPSCompliantRegion(region string) bool {
	// Define the regions where S3 has FIPS endpoints
	// Reference: https://aws.amazon.com/compliance/fips/
	s3fipsRegions := map[string]bool{
		"us-east-1":     true,
		"us-east-2":     true,
		"us-west-1":     true,
		"us-west-2":     true,
		"us-gov-east-1": true,
		"us-gov-west-1": true,
		"ca-central-1":  true,
		"ca-west-1":     true,
	}
	return s3fipsRegions[region]
}

// createAWSConfig returns a new AWS Config object that will be used to create new S3 clients
func createAWSConfig(region string, creds credentials.IAMRoleCredentials, useFIPSEndpoint, useDualStackEndpoint bool) (aws.Config, error) {
	configOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithHTTPClient(httpclient.New(roundtripTimeout, false, agentversion.String(), config.OSType, config.GetOSFamily())),
		awsconfig.WithCredentialsProvider(
			awscreds.NewStaticCredentialsProvider(creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken),
		),
		awsconfig.WithRegion(region),
	}

	if useFIPSEndpoint {
		logger.Debug("FIPS mode detected, using FIPS-compliant S3 endpoint in supported regions")
		configOpts = append(configOpts, awsconfig.WithUseFIPSEndpoint(aws.FIPSEndpointStateEnabled))
	}

	if useDualStackEndpoint {
		logger.Debug("Configuring S3 DualStack endpoint")
		configOpts = append(configOpts, awsconfig.WithUseDualStackEndpoint(aws.DualStackEndpointStateEnabled))
	}

	return awsconfig.LoadDefaultConfig(
		context.TODO(),
		configOpts...,
	)

}

// NewS3ManagerClient returns a new S3 client based on the region of the bucket.
func (*s3ClientCreator) NewS3ManagerClient(bucket, region string, creds credentials.IAMRoleCredentials, ipCompatibility ipcompatibility.IPCompatibility) (s3client.S3ManagerClient, error) {
	// Create an initial AWS session to get the bucket region
	cfg, err := createAWSConfig(region, creds, false, ipCompatibility.IsIPv6Only())
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg)
	bucketRegion, err := getRegionFromBucket(client, bucket)
	if err != nil {
		return nil, err
	}

	// Determine if we should use FIPS endpoints based on the bucket region
	useFIPSEndpoint := config.IsFIPSEnabled() && isS3FIPSCompliantRegion(bucketRegion)
	cfg, err = createAWSConfig(bucketRegion, creds, useFIPSEndpoint, ipCompatibility.IsIPv6Only())
	if err != nil {
		return nil, err
	}
	return s3manager.NewDownloader(s3.NewFromConfig(cfg)), nil
}

// NewS3Client returns a new S3 client to support S3 operations which are not provided by s3manager.
func (*s3ClientCreator) NewS3Client(bucket, region string, creds credentials.IAMRoleCredentials, ipCompatibility ipcompatibility.IPCompatibility) (s3client.S3Client, error) {
	// Create an initial AWS session to get the bucket region
	cfg, err := createAWSConfig(region, creds, false, ipCompatibility.IsIPv6Only())
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg)
	bucketRegion, err := getRegionFromBucket(client, bucket)
	if err != nil {
		return nil, err
	}

	// Determine if we should use FIPS endpoints based on the bucket region
	useFIPSEndpoint := config.IsFIPSEnabled() && isS3FIPSCompliantRegion(bucketRegion)
	cfg, err = createAWSConfig(bucketRegion, creds, useFIPSEndpoint, ipCompatibility.IsIPv6Only())
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(cfg), nil
}
func getRegionFromBucket(svc *s3.Client, bucket string) (string, error) {
	ctx := context.Background()
	opts := []func(*s3.Options){}
	if config.IsFIPSEnabled() {
		logger.Debug("FIPS mode detected, using virtual-host–style URLs for bucket location")
		opts = append(opts, func(o *s3.Options) {
			o.UsePathStyle = false
		})
	}
	region, err := s3manager.GetBucketRegion(ctx, svc, bucket, opts...)
	if err != nil {
		return "", err
	}
	return region, nil
}
