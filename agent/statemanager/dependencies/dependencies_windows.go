// +build windows

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

package dependencies

import (
	"golang.org/x/sys/windows/registry"
)

//go:generate mockgen -destination=mocks/mocks_windows.go -copyright_file=../../../scripts/copyright_file github.com/aws/amazon-ecs-agent/agent/statemanager/dependencies WindowsRegistry,RegistryKey

// WindowsRegistry is an interface for the package-level methods in the golang.org/x/sys/windows/registry package
type WindowsRegistry interface {
	CreateKey(k registry.Key, path string, access uint32) (newk RegistryKey, openedExisting bool, err error)
	OpenKey(k registry.Key, path string, access uint32) (RegistryKey, error)
}

// RegistryKey is an interface for the registry.Key type
type RegistryKey interface {
	GetStringValue(name string) (val string, valtype uint32, err error)
	SetStringValue(name, value string) error
	Close() error
}

type StdRegistry struct{}

func (StdRegistry) CreateKey(k registry.Key, path string, access uint32) (RegistryKey, bool, error) {
	return registry.CreateKey(k, path, access)
}

func (StdRegistry) OpenKey(k registry.Key, path string, access uint32) (RegistryKey, error) {
	return registry.OpenKey(k, path, access)
}
