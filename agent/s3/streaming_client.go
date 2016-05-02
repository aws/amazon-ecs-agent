// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package s3

import (
	"errors"
	"fmt"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/service/s3"
	"io"
	"time"
)

const (
	// Max number of chunks to keep in the output channel at a time.
	BUFFER_SIZE = 4

	// Number of times to retry S3.HeadObject/GetObject.
	RETRIES = 5

	// Initial sleep time when an S3 operation fails.
	BACKOFF_INIT = time.Second

	// Maximum amount of time to sleep between retries.
	BACKOFF_MAX = time.Second * 8

	// Multiplier applied to the sleep time when an S3 operation fails more than
	// once.
	BACKOFF_MULTIPLIER = 2

	// Preferred chunk size to pull from S3 at a time.
	PREFERRED_CHUNK_SIZE = 8 * 1024 * 1024

	// Preferred number of workers pulling S3 chunks.
	PREFERRED_WORKERS = 4

	// Maximum allowable number of stored chunks.
	PREFERRED_LOOK_AHEAD = 8

	// Maximum size of a chunk
	MAX_CHUNK_SIZE = 5 * 1024 * 1024 * 1024
)

// Create a new StreamingClient
func NewStreamingClient(s3Client RawS3Client) *ApiStreamingClient {
	return &ApiStreamingClient{s3Client: s3Client}
}

// Stream the bytes and errors from an S3 object.
func (client *ApiStreamingClient) StreamObject(bucket string, key string) (io.ReadCloser, error) {
	chunkSize := int64(PREFERRED_CHUNK_SIZE)
	workers := PREFERRED_WORKERS
	lookAhead := PREFERRED_LOOK_AHEAD
	return client.streamObject(bucket, key, chunkSize, workers, lookAhead)
}

func (client *ApiStreamingClient) streamObject(bucket string, key string, chunkSize int64, workers int, lookAhead int) (io.ReadCloser, error) {
	contentLength, err := client.contentLength(bucket, key)

	if err != nil {
		return nil, err
	}

	chunkCount := int(contentLength / chunkSize)

	if (contentLength % chunkSize) != 0 {
		chunkCount += 1
	}

	if chunkSize > contentLength {
		chunkSize = contentLength
	}

	if chunkSize > MAX_CHUNK_SIZE {
		chunkSize = MAX_CHUNK_SIZE
	}

	if workers > chunkCount {
		workers = chunkCount
	}

	if lookAhead > chunkCount {
		lookAhead = chunkCount
	}

	getChunk := func(index int) ([]byte, error) {
		begin := int64(index) * chunkSize
		end := begin + chunkSize - 1

		if (begin < 0) || (begin >= contentLength) {
			return nil, errors.New(fmt.Sprintf("Index out of bounds: %i", index))
		}

		if end >= contentLength {
			end = contentLength - 1
		}

		return client.getObjectChunk(bucket, key, begin, end)
	}

	reader := NewChunkedReader(contentLength, chunkSize, workers, lookAhead, getChunk)
	err = reader.Init()

	return reader, err
}

// Retreive a chunk of an S3 object.
func (client *ApiStreamingClient) getObjectChunk(bucket string, key string, begin int64, end int64) (buffer []byte, err error) {
	backoff := utils.NewSimpleBackoff(BACKOFF_INIT, BACKOFF_MAX, 0.0, BACKOFF_MULTIPLIER)
	rng := fmt.Sprintf("bytes=%d-%d", begin, end)
	length := (end - begin) + 1
	bytes := make([]byte, length)
	input := &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
		Range:  &rng,
	}

	err = utils.RetryNWithBackoff(backoff, RETRIES, func() error {
		getObjectOutput, s3Err := client.s3Client.GetObject(input)

		if s3Err != nil {
			return s3Err
		} else if getObjectOutput.Body == nil {
			return errors.New("S3.GetObject returned no body")
		}

		read, ioErr := io.ReadFull(getObjectOutput.Body, bytes)

		if ioErr != nil {
			return ioErr
		}

		if int64(read) != length {
			return errors.New(fmt.Sprintf("Expected %i bytes, got %i", length, read))
		}

		buffer = bytes

		return nil
	})

	return
}

// Retreive the content length of an S3 object.
func (client *ApiStreamingClient) contentLength(bucket string, key string) (contentLength int64, err error) {
	backoff := utils.NewSimpleBackoff(BACKOFF_INIT, BACKOFF_MAX, 0.0, BACKOFF_MULTIPLIER)
	err = utils.RetryNWithBackoff(backoff, RETRIES, func() error {
		headObjectOutput, s3Err := client.s3Client.HeadObject(&s3.HeadObjectInput{
			Bucket: &bucket,
			Key:    &key,
		})

		if s3Err != nil {
			return s3Err
		} else if (headObjectOutput.ContentLength == nil) || (*headObjectOutput.ContentLength == 0) {
			return errors.New("S3.HeadObject returned no content length")
		} else {
			contentLength = *headObjectOutput.ContentLength
			return nil
		}
	})

	return
}
