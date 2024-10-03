//go:build linux && sudo_unit
// +build linux,sudo_unit

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

package ecscni

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	cnins "github.com/containernetworking/plugins/pkg/ns"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testNetNsPath    = "/var/run/netns/nsNane"
	testNetNsName    = "nsName"
	testNameServer   = "nameServer"
	testSearchDomain = "searchDomain"
)

func TestNewDelNetNS(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nsUtil := NewNetNSUtil()

	delErr := nsUtil.DelNetNS(testNetNsPath)
	require.Error(t, delErr, "before NewNetNS, DelNetNS expects error")

	newErr := nsUtil.NewNetNS(testNetNsPath)
	require.NoError(t, newErr, "expect no error from NewNetNS")
	currentNS, err := cnins.GetCurrentNS()
	require.NoError(t, err, "expect no error when get current NS")
	assert.Equal(t, testNetNsPath, currentNS.Path(), "expect currentNS has path same with test NS")

	delErr = nsUtil.DelNetNS(testNetNsPath)
	require.NoError(t, delErr, "expect no error from DelNetNS")

	delErr = nsUtil.DelNetNS(testNetNsPath)
	require.Error(t, delErr, "after DelNetNS, another DelNetNS expects error")
}

func TestGetNetNSPath(t *testing.T) {
	nsUtil := NewNetNSUtil()
	path := nsUtil.GetNetNSPath(testNetNsName)
	assert.Equal(t, testNetNsPath, path)
}

func TestGetNetNSName(t *testing.T) {
	nsUtil := NewNetNSUtil()
	name := nsUtil.GetNetNSName(testNetNsPath)
	assert.Equal(t, testNetNsName, name)
}

func TestNSExists(t *testing.T) {
	nsUtil := NewNetNSUtil()
	existPreNew, err := nsUtil.NSExists(testNetNsPath)
	require.False(t, existPreNew)
	require.NoError(t, err)

	newErr := nsUtil.NewNetNS(testNetNsPath)
	require.NoError(t, newErr)
	existAfterNew, err := nsUtil.NSExists(testNetNsPath)
	require.True(t, existAfterNew)
	require.NoError(t, err)

	delErr := nsUtil.DelNetNS(testNetNsPath)
	require.NoError(t, delErr)
	existAfterDel, err := nsUtil.NSExists(testNetNsPath)
	require.False(t, existAfterDel)
	require.NoError(t, err)
}

func TestExecInNSPath(t *testing.T) {
	nsUtil := NewNetNSUtil()
	err := nsUtil.ExecInNSPath(testNetNsPath, func(ns cnins.NetNS) error {
		return nil
	})
	require.Nil(t, err)
}

func TestBuildResolvConfig(t *testing.T) {
	nsUtil := NewNetNSUtil()
	rst := nsUtil.BuildResolvConfig([]string{testNameServer}, []string{testSearchDomain})
	require.Equal(t, fmt.Sprintf("nameserver %s\nsearch %s", testNameServer, testSearchDomain), rst)
}

// TestNewNSDuplicate verifies that creating two net ns will not fail.
// It should ignore errors related to the default netns directory already existing.
func TestNewNSDuplicate(t *testing.T) {
	// Create a new instance of netnsutil.
	ns := NewNetNSUtil()

	// Create a temporary directory for testing.
	tempDir, err := os.MkdirTemp("", tempNSPathPattern)
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Default directory should not exist and check we can create with no issues.
	defaultPath := filepath.Join(tempDir, "defaultns")
	err = ns.NewNetNS(defaultPath)
	assert.NoError(t, err)

	// Newly created defaultNS should now exist.
	_, err = os.Stat(defaultPath)
	assert.NoError(t, err)

	// Default directory already exists.
	err = ns.NewNetNS(defaultPath)
	assert.NoError(t, err)

	// Newly created defaultNS should now exist.
	_, err = os.Stat(defaultPath)
	assert.NoError(t, err)
}
