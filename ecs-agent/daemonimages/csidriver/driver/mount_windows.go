//go:build windows
// +build windows

// this file has been modified from its original found in:
// https://github.com/kubernetes-sigs/aws-ebs-csi-driver

/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package driver

import "errors"

func (m NodeMounter) GetDeviceNameFromMount(mountPath string) (string, int, error) {
	// TODO
	return "", 0, errors.New("not supported")
}

func (m NodeMounter) IsCorruptedMnt(err error) bool {
	// TODO
	return false
}

func (m *NodeMounter) MakeDir(path string) error {
	// TODO
	return errors.New("not supported")
}

func (m *NodeMounter) MakeFile(path string) error {
	// TODO
	return errors.New("not supported")
}

func (m *NodeMounter) NeedResize(devicePath string, deviceMountPath string) (bool, error) {
	// TODO
	return false, errors.New("not supported")
}

func (m *NodeMounter) NewResizeFs() (Resizefs, error) {
	// TODO
	return nil, errors.New("not supported")
}

func (m *NodeMounter) PathExists(path string) (bool, error) {
	// TODO
	return false, errors.New("not supported")
}

func (m *NodeMounter) Unpublish(path string) error {
	// TODO
	return errors.New("not supported")
}

func (m *NodeMounter) Unstage(path string) error {
	// TODO
	return errors.New("not supported")
}
