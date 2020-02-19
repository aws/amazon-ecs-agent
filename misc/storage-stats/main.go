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

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"syscall"
	"time"
)

func check(e error) {
	if e != nil {
		fmt.Printf("error: %v\n", e)
	}
}

func writeBytes(byteCount int64, toFile *os.File) error {
	//populate content with random bytes
	writeBytes := make([]byte, byteCount)
	rand.Read(writeBytes)
	// write and flush to disk to force block write
	bytesWritten, err := toFile.Write(writeBytes)
	if err != nil {
		return err
	}
	err = toFile.Sync()
	if err != nil {
		return err
	}
	fmt.Printf("wrote %d bytes\n", bytesWritten)
	return nil
}

func readBytes(readFilePath string) error {
	// O_DIRECT flag skips the filesystem cache
	f, err := os.OpenFile(readFilePath, os.O_RDONLY|syscall.O_DIRECT, 0777)
	if err != nil {
		return err
	}
	bytes, err := ioutil.ReadAll(f)
	if err != nil {
		return err
	}
	fmt.Printf("read %d bytes\n", len(bytes))
	return nil
}

func main() {
	sleepInterval := flag.Int("sleep", 1000, "length of sleep interval")
	byteCount := flag.Int64("bytecount", 1024, "size in bytes to be written per interval")
	flag.Parse()
	for {
		// create a temp file to write to then read from
		tmpFile, err := ioutil.TempFile(os.TempDir(), "blockwrite-")
		defer func() {
			err = tmpFile.Close()
			check(err)
			err = os.Remove(tmpFile.Name())
			check(err)
		}()
		// Storage stats are cumulative.
		// We do incremental reads/writes with sleep to create
		// a predictable increase over time.
		writeBytes(*byteCount, tmpFile)
		readBytes(tmpFile.Name())
		time.Sleep(time.Duration(int32(*sleepInterval)) * time.Millisecond)
	}
}
