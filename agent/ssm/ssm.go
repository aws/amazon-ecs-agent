// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
	"github.com/pkg/errors"
)

// GetSecretFromSSM makes the api call to the AWS SSM parameter store to
// retrieve secrets value in batches
func GetSecretsFromSSM(names []string, client ssmiface.SSMAPI) (map[string]string, error) {
	var secretNames []*string
	for _, name := range names {
		secretNames = append(secretNames, aws.String(name))
	}

	in := &ssm.GetParametersInput{
		Names:          secretNames,
		WithDecryption: aws.Bool(true),
	}

	out, err := client.GetParameters(in)
	if err != nil {
		return nil, errors.Wrapf(err,
			"fetching secret data from ssm parameter store")
	}

	return extractSSMValues(out)
}

func extractSSMValues(out *ssm.GetParametersOutput) (map[string]string, error) {
	if out == nil {
		return nil, errors.New(
			"fetching secret data from ssm parameter store: empty response")
	}

	if len(out.InvalidParameters) != 0 {
		errorString := ""
		for _, invalid := range out.InvalidParameters {
			errorString += aws.StringValue(invalid) + ", "
		}
		return nil, fmt.Errorf(
			"fetching secret data from ssm parameter store: invalid parameters: %s", errorString)
	}

	parameterValues := make(map[string]string)
	for _, parameter := range out.Parameters {
		parameterValues[aws.StringValue(parameter.Name)] = aws.StringValue(parameter.Value)
	}

	return parameterValues, nil
}
