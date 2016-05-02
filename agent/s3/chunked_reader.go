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
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"io"
	"time"
)

// Create a new ChunkedReader
func NewChunkedReader(contentLength int64, chunkSize int64, workers int, lookAhead int, getChunk func(int) ([]byte, error)) *ChunkedReader {
	return &ChunkedReader{
		workerOut:     nil,
		workerDone:    nil,
		workerIn:      nil,
		bytesCache:    make(map[int]([]byte)),
		contentLength: contentLength,
		chunkSize:     chunkSize,
		bytesRead:     0,
		workers:       workers,
		lookAhead:     lookAhead,
		open:          true,
		getChunk:      getChunk,
	}
}

// Start the worker threads.
func (self *ChunkedReader) Init() error {
	if self.workerOut != nil {
		return nil
	} else if !self.open {
		return errors.New("Cannot init a closed reader")
	}

	chunks := int(self.contentLength / self.chunkSize)
	if (self.contentLength % self.chunkSize) != 0 {
		chunks += 1
	}

	self.workerOut = make(chan *ChunkedResponse, chunks)
	self.workerDone = make(chan int, chunks)
	self.workerIn = make(chan *int, chunks)

	for id := 0; id < self.workers; id++ {
		go self.work(id)
	}

	for i := 0; i < chunks; i++ {
		index := i
		self.workerIn <- &index
	}

	return nil
}

// Read input into the given byte slice.
func (self *ChunkedReader) Read(buffer []byte) (int, error) {
	if !self.open {
		return 0, io.EOF
	}

	var err error

	index := self.bytesRead / self.chunkSize
	key := int(index)
	offset := index * self.chunkSize

	for self.bytesCache[key] == nil {
		response := <-self.workerOut
		if response.err != nil {
			self.Close()
			return 0, response.err
		}
		self.bytesCache[response.index] = response.buffer
	}

	end := int64(len(buffer)) + self.bytesRead - 1

	if end >= self.contentLength {
		end = self.contentLength - 1
	} else if end >= ((index + 1) * self.chunkSize) {
		end = ((index + 1) * self.chunkSize) - 1
	}

	copy(buffer, self.bytesCache[key][self.bytesRead-offset:end-offset+1])

	bytesRead := (end - self.bytesRead) + 1
	self.bytesRead += bytesRead

	if (self.bytesRead / self.chunkSize) > index {
		delete(self.bytesCache, key)
	}

	if self.bytesRead >= self.contentLength {
		self.Close()
		err = io.EOF
	}

	return int(bytesRead), err
}

// Close the ChunkedReader, draining out its worker pool.
func (self *ChunkedReader) Close() error {
	if !self.open {
		return nil
	}

	self.open = false

	go func() {
		done := 0
		close(self.workerIn)
		defer close(self.workerOut)
		defer close(self.workerDone)

		for {
			select {
			case <-self.workerOut:
			case <-self.workerDone:
				done++
				if done == self.workers {
					self.bytesCache = nil
					return
				}
			}
		}
	}()

	return nil
}

func (self *ChunkedReader) work(id int) {
	for {
		index, open := <-self.workerIn
		if index != nil {
			for (int(self.bytesRead/self.chunkSize) + 1 + self.lookAhead) < *index {
				ttime.Sleep(time.Millisecond * 100)
			}

			buffer, err := self.getChunk(*index)
			self.workerOut <- &ChunkedResponse{
				buffer: buffer,
				index:  *index,
				err:    err,
			}
		}
		if !open {
			break
		}
	}

	self.workerDone <- id
}
