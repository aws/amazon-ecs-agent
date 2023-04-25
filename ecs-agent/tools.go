//go:build tools
// +build tools

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

package tools

// Some packages are required by tools we use but are not used explicitly in the code.
// Import those packages so that they are copied to the vendor directory by go mod.
import (
	_ "github.com/golang/mock/mockgen/model"
)
