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

package types

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/require"
)

func TestNetworkBlackholePortAddSourceToFilterIfNotAlready(t *testing.T) {
	t.Run("nil SourcesToFilter is initialized", func(t *testing.T) {
		var req NetworkBlackholePortRequest = NetworkBlackholePortRequest{}
		req.AddSourceToFilterIfNotAlready("1.2.3.4")
		require.Equal(t, aws.StringValueSlice(req.SourcesToFilter), []string{"1.2.3.4"})
	})
	t.Run("Source can be added", func(t *testing.T) {
		var req NetworkBlackholePortRequest = NetworkBlackholePortRequest{
			SourcesToFilter: aws.StringSlice([]string{"8.8.8.8"}),
		}
		req.AddSourceToFilterIfNotAlready("1.2.3.4")
		require.Equal(t, aws.StringValueSlice(req.SourcesToFilter), []string{"8.8.8.8", "1.2.3.4"})
	})
	t.Run("Duplicate source is not added", func(t *testing.T) {
		var req NetworkBlackholePortRequest = NetworkBlackholePortRequest{
			SourcesToFilter: aws.StringSlice([]string{"8.8.8.8", "1.2.3.4"}),
		}
		req.AddSourceToFilterIfNotAlready("1.2.3.4")
		require.Equal(t, aws.StringValueSlice(req.SourcesToFilter), []string{"8.8.8.8", "1.2.3.4"})
	})
}

// Tests for validateNetworkFaultRequestSource function that parses IPv4 and IPv4 CIDR blocks.
func TestValidateNetworkFaultRequestSources(t *testing.T) {
	tcs := []struct {
		Name          string
		Input         string
		ShouldSucceed bool
	}{
		{"IPv4", "1.2.3.4", true},
		{"IPv4 CIDR", "1.2.3.4/10", true},
		{"IPv6", "2001:db8::68", false},
		{"IPv6 CIDR", "::1/128", false},
		{"invalid input", "invalid", false},
		{"empty input", "", false},
	}
	for _, tc := range tcs {
		t.Run(tc.Name, func(t *testing.T) {
			err := validateNetworkFaultRequestSource(tc.Input, "input")
			if tc.ShouldSucceed {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, fmt.Sprintf("invalid value %s for parameter input", tc.Input))
			}
		})
	}
}
