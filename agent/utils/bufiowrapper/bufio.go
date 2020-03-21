// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package bufiowrapper

import (
	"bufio"

	"io"
)

// Bufio wraps method from bufio package for testing
type Bufio interface {
	NewScanner(io.Reader) Scanner
}

// Scanner wraps methods for bufio.Scanner type
type Scanner interface {
	Scan() bool
	Text() string
	Err() error
}

type _bufio struct {
}

func NewBufio() Bufio {
	return &_bufio{}
}

func (*_bufio) NewScanner(reader io.Reader) Scanner {
	return bufio.NewScanner(reader)
}
