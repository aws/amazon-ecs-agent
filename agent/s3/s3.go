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

package s3

import (
	"context"
	"io"
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
)

const (
	s3ARNRegex = `arn:([^:]+):s3:::([^/]+)/(.+)`
)

// DownloadFile downloads a file from s3 and writes it with the writer.
func DownloadFile(bucket, key string, timeout time.Duration, w io.WriterAt, client S3ManagerClient) error {
	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, err := client.DownloadWithContext(ctx, w, input)
	return err
}

// ParseS3ARN parses an s3 ARN.
func ParseS3ARN(s3ARN string) (bucket string, key string, err error) {
	exp := regexp.MustCompile(s3ARNRegex)
	match := exp.FindStringSubmatch(s3ARN)
	if len(match) != 4 {
		return "", "", errors.Errorf("invalid s3 arn: %s", s3ARN)
	}
	return match[2], match[3], nil
}

func GetObject(bucket string, key string, client S3Client) (string, error) {
	requestInput := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	result, err := client.GetObject(requestInput)
	if err != nil {
		return "", err
	}

	defer result.Body.Close()
	resultBody, err := io.ReadAll(result.Body)
	if err != nil {
		return "", err
	}
	credSpecData := string(resultBody)

	return credSpecData, nil
}
