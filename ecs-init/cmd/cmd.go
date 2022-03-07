// Copyright 2015-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package cmd

//go:generate mockgen.sh sysctl $GOFILE ../exec/sysctl
//go:generate mockgen.sh iptables $GOFILE ../exec/iptables

// Cmd defines common methods from exec.Cmd that are used to run external
// commands
type Cmd interface {
	CombinedOutput() ([]byte, error)
	Output() ([]byte, error)
}
