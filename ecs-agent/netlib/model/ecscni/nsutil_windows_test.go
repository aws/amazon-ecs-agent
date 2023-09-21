//go:build windows && unit
// +build windows,unit

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
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestNewDelNetNS(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	nsUtil := NewNetNSUtil()
	testNetNsID := nsUtil.NewNetNSID()

	delErr := nsUtil.DelNetNS(testNetNsID)
	require.Error(t, delErr, "before NewNetNS, DelNetNS expects error")

	newErr := nsUtil.NewNetNS(testNetNsID)
	require.NoError(t, newErr, "expect no error from NewNetNS")

	delErr = nsUtil.DelNetNS(testNetNsID)
	require.NoError(t, delErr, "expect no error from DelNetNS")

	delErr = nsUtil.DelNetNS(testNetNsID)
	require.Error(t, delErr, "after DelNetNS, another DelNetNS expects error")
}

func TestNSExists(t *testing.T) {
	nsUtil := NewNetNSUtil()
	testNetNsID := nsUtil.NewNetNSID()

	existPreNew, err := nsUtil.NSExists(testNetNsID)
	require.False(t, existPreNew)
	require.NoError(t, err)

	newErr := nsUtil.NewNetNS(testNetNsID)
	require.NoError(t, newErr)
	existAfterNew, err := nsUtil.NSExists(testNetNsID)
	require.True(t, existAfterNew)
	require.NoError(t, err)

	delErr := nsUtil.DelNetNS(testNetNsID)
	require.NoError(t, delErr)
	existAfterDel, err := nsUtil.NSExists(testNetNsID)
	require.False(t, existAfterDel)
	require.NoError(t, err)
}
