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
	"io"
)

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
