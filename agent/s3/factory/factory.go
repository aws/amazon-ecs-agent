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
	"time"

	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/httpclient"
	s3client "github.com/aws/amazon-ecs-agent/agent/s3"

	"github.com/aws/aws-sdk-go/aws"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

const (
	bucketLocationDefault = "us-east-1"
	roundtripTimeout      = 5 * time.Second
)

type S3ClientCreator interface {
	NewS3ClientForBucket(bucket, region string, creds credentials.IAMRoleCredentials) (s3client.S3Client, error)
}

func NewS3ClientCreator() S3ClientCreator {
	return &s3ClientCreator{}
}

type s3ClientCreator struct{}

// NewS3Client returns a new S3 client based on the region of the bucket.
func (*s3ClientCreator) NewS3ClientForBucket(bucket, region string,
	creds credentials.IAMRoleCredentials) (s3client.S3Client, error) {
	cfg := aws.NewConfig().
		WithHTTPClient(httpclient.New(roundtripTimeout, false)).
		WithCredentials(
			awscreds.NewStaticCredentials(creds.AccessKeyID, creds.SecretAccessKey,
				creds.SessionToken)).WithRegion(region)
	sess := session.Must(session.NewSession(cfg))

	svc := s3.New(sess)
	bucketRegion, err := getRegionFromBucket(svc, bucket)
	if err != nil {
		return nil, err
	}

	sessWithRegion := session.Must(session.NewSession(cfg.WithRegion(bucketRegion)))
	return s3manager.NewDownloaderWithClient(s3.New(sessWithRegion)), nil
}

func getRegionFromBucket(svc *s3.S3, bucket string) (string, error) {
	input := &s3.GetBucketLocationInput{
		Bucket: aws.String(bucket),
	}
	result, err := svc.GetBucketLocation(input)
	if err != nil {
		return "", err
	}
	if result.LocationConstraint == nil { // GetBucketLocation returns nil for bucket in us-east-1.
		return bucketLocationDefault, nil
	}

	return aws.StringValue(result.LocationConstraint), nil
}
