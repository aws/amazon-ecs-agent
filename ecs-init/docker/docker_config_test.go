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

package docker

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetNsenterBinds(t *testing.T) {
	t.Run("nsenter not found", func(t *testing.T) {
		binds := getNsenterBinds(
			func(s string) (os.FileInfo, error) { return nil, errors.New("not found") })
		assert.Empty(t, binds)
	})

	t.Run("nsenter is found", func(t *testing.T) {
		binds := getNsenterBinds(
			func(s string) (os.FileInfo, error) { return nil, nil })
		require.Len(t, binds, 1)
		assert.Equal(t, "/usr/bin/nsenter:/usr/bin/nsenter", binds[0])
	})
}

func TestGetModinfoBinds(t *testing.T) {
	t.Run("modinfo not found", func(t *testing.T) {
		binds := getModInfoBinds(
			func(s string) (os.FileInfo, error) { return nil, errors.New("not found") })
		assert.Empty(t, binds)
	})
	t.Run("modinfo is found", func(t *testing.T) {
		binds := getModInfoBinds(
			func(s string) (os.FileInfo, error) { return nil, nil })
		require.Len(t, binds, 1)
		assert.Equal(t, "/sbin/modinfo:/sbin/modinfo", binds[0])
	})
}
