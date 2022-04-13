//go:build unit
// +build unit

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//    http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package ssm

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/stretchr/testify/assert"
)

const (
	invalidParam1 = "/test/invalidParams"
	validParam1   = "/test/param1"
	validValue1   = "secret1"
)

type mockGetParameters struct {
	SSMClient
	Resp ssm.GetParametersOutput
}

func (m mockGetParameters) GetParameters(input *ssm.GetParametersInput) (*ssm.GetParametersOutput, error) {
	return &m.Resp, nil
}

func TestGetSecretsFromSSM(t *testing.T) {
	param1 := ssm.Parameter{
		Name:  aws.String(validParam1),
		Value: aws.String(validValue1),
	}

	cases := []struct {
		Name        string
		Resp        ssm.GetParametersOutput
		ShouldError bool
	}{
		{
			Name: "SuccessWithNoInvalidParameters",
			Resp: ssm.GetParametersOutput{
				InvalidParameters: []*string{},
				Parameters:        []*ssm.Parameter{&param1},
			},
			ShouldError: false,
		},
		{
			Name: "ErrorWithInvalidParameters",
			Resp: ssm.GetParametersOutput{
				InvalidParameters: []*string{aws.String(invalidParam1)},
				Parameters:        []*ssm.Parameter{&param1},
			},
			ShouldError: true,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			ssmClient := mockGetParameters{Resp: c.Resp}
			names := []string{validParam1, invalidParam1}
			_, err := GetSecretsFromSSM(names, ssmClient)

			if c.ShouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetParametersFromSSM(t *testing.T) {
	param1 := ssm.Parameter{
		Name:  aws.String(validParam1),
		Value: aws.String(validValue1),
	}

	cases := []struct {
		Name        string
		Resp        ssm.GetParametersOutput
		ShouldError bool
	}{
		{
			Name: "SuccessWithNoInvalidParameters",
			Resp: ssm.GetParametersOutput{
				InvalidParameters: []*string{},
				Parameters:        []*ssm.Parameter{&param1},
			},
			ShouldError: false,
		},
		{
			Name: "ErrorWithInvalidParameters",
			Resp: ssm.GetParametersOutput{
				InvalidParameters: []*string{aws.String(invalidParam1)},
				Parameters:        []*ssm.Parameter{&param1},
			},
			ShouldError: true,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			ssmClient := mockGetParameters{Resp: c.Resp}
			names := []string{validParam1, invalidParam1}
			_, err := GetParametersFromSSM(names, ssmClient)

			if c.ShouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
