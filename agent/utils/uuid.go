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

package utils

import (
	"github.com/pborman/uuid"
)

// UUIDProvider wraps 'uuid' methods for testing
type UUIDProvider interface {
	New() string
}

// dynamicUUIDProvider is a uuid provider used in run time
type dynamicUUIDProvider struct {
}

// NewDynamicUUIDProvider returns a new dynamicUUIDProvider
func NewDynamicUUIDProvider() UUIDProvider {
	return &dynamicUUIDProvider{}
}

// New returns a new random uuid
func (*dynamicUUIDProvider) New() string {
	return uuid.New()
}

// staticUUIDProvider is a uuid provider used in testing
type staticUUIDProvider struct {
	staticID string
}

// NewStaticUUIDProvider returns a new staticUUIDProvider
func NewStaticUUIDProvider(staticID string) UUIDProvider {
	return &staticUUIDProvider{
		staticID: staticID,
	}
}

// New returns a static uuid
func (s *staticUUIDProvider) New() string {
	return s.staticID
}
