// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package task

// Association is a definition of a device (or other potential things) that is
// associated with a task.
type Association struct {
	// Containers are the container names that can use this association.
	Containers []string `json:"containers"`
	// Content describes this association.
	Content EncodedString `json:"content"`
	// Name is a unique identifier of the association within a task.
	Name string `json:"name"`
	// Type specifies the type of this association.
	Type string `json:"type"`
}

// EncodedString is used to describe an association, the consumer of the association
// data should be responsible to decode it. We don't model it in Agent, or check the
// value of the encoded string, we just pass whatever we get from ACS, because
// we don't want Agent to be coupled with a specific association.
type EncodedString struct {
	// Encoding is the encoding type of the value.
	Encoding string `json:"encoding"`
	// Value is the content of the encoded string.
	Value string `json:"value"`
}
