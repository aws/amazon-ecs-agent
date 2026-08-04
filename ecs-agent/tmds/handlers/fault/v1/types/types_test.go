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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"
)

func TestNetworkBlackholePortAddSourceToFilterIfNotAlready(t *testing.T) {
	t.Run("nil SourcesToFilter is initialized", func(t *testing.T) {
		var req NetworkBlackholePortRequest = NetworkBlackholePortRequest{}
		req.AddSourceToFilterIfNotAlready("1.2.3.4")
		require.Equal(t, aws.ToStringSlice(req.SourcesToFilter), []string{"1.2.3.4"})
	})
	t.Run("Source can be added", func(t *testing.T) {
		var req NetworkBlackholePortRequest = NetworkBlackholePortRequest{
			SourcesToFilter: aws.StringSlice([]string{"8.8.8.8"}),
		}
		req.AddSourceToFilterIfNotAlready("1.2.3.4")
		require.Equal(t, aws.ToStringSlice(req.SourcesToFilter), []string{"8.8.8.8", "1.2.3.4"})
	})
	t.Run("Duplicate source is not added", func(t *testing.T) {
		var req NetworkBlackholePortRequest = NetworkBlackholePortRequest{
			SourcesToFilter: aws.StringSlice([]string{"8.8.8.8", "1.2.3.4"}),
		}
		req.AddSourceToFilterIfNotAlready("1.2.3.4")
		require.Equal(t, aws.ToStringSlice(req.SourcesToFilter), []string{"8.8.8.8", "1.2.3.4"})
	})
}

func TestRequireIPInRequestSources(t *testing.T) {
	tcs := []struct {
		Name          string
		Input         string
		ShouldSucceed bool
	}{
		{"Valid IPv4", "1.2.3.4", true},
		{"Valid IPv4 CIDR", "1.2.3.4/10", true},
		{"Valid IPv6", "2001:db8::1", true},
		{"Valid full IPv6", "2001:0db8:0000:0000:0000:0000:0000:0001", true},
		{"Valid IPv6 CIDR", "::1/128", true},
		{"Invalid input", "invalid", false},
		{"Invalid IPv6", "2001:db8::1::1", false},
		{"IPv4 with invalid CIDR", "192.168.1.0/", false},
		{"IPv6 with invalid CIDR", "2001:db8::/129", false},
		{"Empty input", "", false},
	}
	for _, tc := range tcs {
		t.Run(tc.Name, func(t *testing.T) {
			err := requireIPInRequestSource(tc.Input, "input")
			if tc.ShouldSucceed {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, fmt.Sprintf("invalid value %s for parameter input", tc.Input))
			}
		})
	}
}

func TestNetworkFaultAddSourcesRequestValidateRequest(t *testing.T) {
	ip := func(s string) *string { return aws.String(s) }

	// many16 builds a slice of 16 distinct IPv4 addresses for cap tests.
	many16 := make([]*string, 16)
	for i := 0; i < 16; i++ {
		many16[i] = aws.String(fmt.Sprintf("10.0.0.%d", i+1))
	}
	many17 := append([]*string{}, many16...)
	many17 = append(many17, aws.String("10.0.0.17"))

	tcs := []struct {
		name    string
		request NetworkFaultAddSourcesRequest
		wantErr string // empty means no error expected
	}{
		{
			name:    "both lists empty",
			request: NetworkFaultAddSourcesRequest{},
			wantErr: AddSourcesEmptyError,
		},
		{
			name: "Sources only, valid IPv4",
			request: NetworkFaultAddSourcesRequest{
				Sources: []*string{ip("1.2.3.4")},
			},
		},
		{
			name: "SourcesToFilter only, valid IPv4 CIDR",
			request: NetworkFaultAddSourcesRequest{
				SourcesToFilter: []*string{ip("10.0.0.0/24")},
			},
		},
		{
			name: "both lists, disjoint, valid",
			request: NetworkFaultAddSourcesRequest{
				Sources:         []*string{ip("1.2.3.4"), ip("5.6.7.8")},
				SourcesToFilter: []*string{ip("10.0.0.1")},
			},
		},
		{
			name: "total exactly at cap",
			request: NetworkFaultAddSourcesRequest{
				Sources: many16,
			},
		},
		{
			name: "total exceeds cap",
			request: NetworkFaultAddSourcesRequest{
				Sources: many17,
			},
			wantErr: fmt.Sprintf(AddSourcesTooManyIPsError, 17, AddSourcesMaxIPs),
		},
		{
			name: "IPv6 in Sources accepted",
			request: NetworkFaultAddSourcesRequest{
				Sources: []*string{ip("2001:db8::1")},
			},
		},
		{
			name: "IPv6 CIDR in SourcesToFilter accepted",
			request: NetworkFaultAddSourcesRequest{
				SourcesToFilter: []*string{ip("::1/128")},
			},
		},
		{
			name: "dual-stack mix across both lists accepted",
			request: NetworkFaultAddSourcesRequest{
				Sources:         []*string{ip("1.2.3.4"), ip("2001:db8::1")},
				SourcesToFilter: []*string{ip("10.0.0.0/24"), ip("2001:db8:1::/48")},
			},
		},
		{
			name: "non-IP garbage in Sources rejected",
			request: NetworkFaultAddSourcesRequest{
				Sources: []*string{ip("not-an-ip")},
			},
			wantErr: fmt.Sprintf(InvalidValueError, "not-an-ip", "Sources"),
		},
		{
			name: "Sources and SourcesToFilter overlap",
			request: NetworkFaultAddSourcesRequest{
				Sources:         []*string{ip("1.2.3.4")},
				SourcesToFilter: []*string{ip("1.2.3.4")},
			},
			wantErr: fmt.Sprintf(AddSourcesOverlapError, "1.2.3.4"),
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.request.ValidateRequest()
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.wantErr)
			}
		})
	}
}

func TestNetworkFaultAddSourcesRequestToString(t *testing.T) {
	req := NetworkFaultAddSourcesRequest{
		Sources:         []*string{aws.String("1.2.3.4")},
		SourcesToFilter: []*string{aws.String("5.6.7.8")},
	}
	require.Equal(t, `{"Sources":["1.2.3.4"],"SourcesToFilter":["5.6.7.8"]}`, req.ToString())
}
