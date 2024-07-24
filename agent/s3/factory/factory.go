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
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/aws-sdk-go/aws"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

const (
	roundtripTimeout = 5 * time.Second
)

type S3ClientCreator interface {
	NewS3ManagerClient(bucket, region string, creds credentials.IAMRoleCredentials) (s3client.S3ManagerClient, error)
	NewS3Client(bucket, region string, creds credentials.IAMRoleCredentials) (s3client.S3Client, error)
}

// NewS3ClientCreator provides 2 implementations
// NewS3ManagerClient implements methods from aws-sdk-go/service/s3manager.
// NewS3Client implements methods from aws-sdk-go/service/s3.
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

func createAWSConfig(region string, creds credentials.IAMRoleCredentials, useFIPSEndpoint bool) *aws.Config {
	cfg := aws.NewConfig().
		WithHTTPClient(httpclient.New(roundtripTimeout, false, agentversion.String(), config.OSType)).
		WithCredentials(
			awscreds.NewStaticCredentials(creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken)).
		WithRegion(region)
	if useFIPSEndpoint {
		logger.Debug("FIPS mode detected, using FIPS-compliant S3 endpoint in supported regions")
		cfg.UseFIPSEndpoint = endpoints.FIPSEndpointStateEnabled
	}
	return cfg
}

// NewS3ManagerClient returns a new S3 client based on the region of the bucket.
func (*s3ClientCreator) NewS3ManagerClient(bucket, region string, creds credentials.IAMRoleCredentials) (s3client.S3ManagerClient, error) {
	// Create an initial AWS session to get the bucket region
	cfg := createAWSConfig(region, creds, false)
	sess := session.Must(session.NewSession(cfg))
	svc := s3.New(sess)
	bucketRegion, err := getRegionFromBucket(svc, bucket)
	if err != nil {
		return nil, err
	}
	// Determine if we should use FIPS endpoints based on the bucket region
	useFIPSEndpoint := config.IsFIPSEnabled() && isS3FIPSCompliantRegion(bucketRegion)
	cfg = createAWSConfig(bucketRegion, creds, useFIPSEndpoint)
	sessWithRegion := session.Must(session.NewSession(cfg))
	return s3manager.NewDownloaderWithClient(s3.New(sessWithRegion)), nil
}

// NewS3Client returns a new S3 client to support S3 operations which are not provided by s3manager.
func (*s3ClientCreator) NewS3Client(bucket, region string, creds credentials.IAMRoleCredentials) (s3client.S3Client, error) {
	// Create an initial AWS session to get the bucket region
	cfg := createAWSConfig(region, creds, false)
	sess := session.Must(session.NewSession(cfg))
	svc := s3.New(sess)
	bucketRegion, err := getRegionFromBucket(svc, bucket)
	if err != nil {
		return nil, err
	}
	// Determine if we should use FIPS endpoints based on the bucket region
	useFIPSEndpoint := config.IsFIPSEnabled() && isS3FIPSCompliantRegion(bucketRegion)
	cfg = createAWSConfig(bucketRegion, creds, useFIPSEndpoint)
	sessWithRegion := session.Must(session.NewSession(cfg))
	return s3.New(sessWithRegion), nil
}
func getRegionFromBucket(svc *s3.S3, bucket string) (string, error) {
	ctx := context.Background()
	opts := []request.Option{}
	if config.IsFIPSEnabled() {
		logger.Debug("FIPS mode detected, using virtual-host–style URLs for bucket location")
		opts = append(opts, func(r *request.Request) {
			r.Config.S3ForcePathStyle = aws.Bool(false)
		})
	}
	region, err := s3manager.GetBucketRegionWithClient(ctx, svc, bucket, opts...)
	if err != nil {
		return "", err
	}
	return region, nil
}
