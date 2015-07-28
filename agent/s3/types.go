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
	"github.com/aws/aws-sdk-go/service/s3"
	"io"
)

// This interface is contains a subset of the methods defined by the aws-sdk-s3
// interface. This proxy interface exists to make it easier to stub out the s3
// client for testing.
type RawS3Client interface {
	// Make a GetObject request to S3
	GetObject(input *s3.GetObjectInput) (*s3.GetObjectOutput, error)

	// Make a HeadObject request to S3
	HeadObject(input *s3.HeadObjectInput) (*s3.HeadObjectOutput, error)
}

// StreamingClient wraps the aws-sdk-go s3 client to provide a streaming
// functionality.
type StreamingClient interface {
	// This method streams the content of an s3 object chunk-by-chunk. It also
	// provides an error channel so that error handling may be done via a select
	// block.
	StreamObject(string, string) (io.ReadCloser, error)
}

// Sole implementer of the StreamingClient interface.
type ApiStreamingClient struct {
	// Internal s3 client.
	s3Client RawS3Client
}

// Used to communicate between threads in the chunked reader.
type ChunkedResponse struct {
	buffer []byte
	index  int
	err    error
}

// Data structure which implements the ReadCloser interface which reads chunks
// of S3 objects using multiple threads.
type ChunkedReader struct {
	workerIn      chan *int
	workerOut     chan *ChunkedResponse
	workerDone    chan int
	bytesCache    map[int]([]byte)
	contentLength int64
	chunkSize     int64
	bytesRead     int64
	workers       int
	lookAhead     int
	open          bool
	getChunk      func(int) ([]byte, error)
}
