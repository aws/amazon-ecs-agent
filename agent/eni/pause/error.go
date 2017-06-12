// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package pause

// UnsupportedPlatformError indicates an error when loading pause container
// image on an unsupported OS platform
type UnsupportedPlatformError struct {
	error
}

// UnsupportedPlatform returns true if the error is of UnsupportedPlatformError
// type
func UnsupportedPlatform(err error) bool {
	_, ok := err.(UnsupportedPlatformError)
	return ok
}
