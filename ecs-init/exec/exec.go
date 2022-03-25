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

package exec

import (
	osexec "os/exec"

	"github.com/aws/amazon-ecs-agent/ecs-init/cmd"
)

//go:generate mockgen.sh sysctl $GOFILE sysctl
//go:generate mockgen.sh iptables $GOFILE iptables

// Exec defines common methods from exec package that are used to run external
// commands
type Exec interface {
	LookPath(file string) (string, error)
	Command(name string, arg ...string) cmd.Cmd
}

type _exec struct{}

func NewExec() Exec {
	return &_exec{}
}

func (*_exec) LookPath(file string) (string, error) {
	return osexec.LookPath(file)
}

func (*_exec) Command(name string, arg ...string) cmd.Cmd {
	return osexec.Command(name, arg...)
}
