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
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/pkg/errors"
)

// GetSecretFromSSM makes the api call to the AWS SSM parameter store to
// retrieve secrets value in batches
func GetSecretsFromSSM(names []string, client SSMClient) (map[string]string, error) {
	return getParameters(names, client, true)
}

// GetParametersFromSSM makes the api call to the AWS SSM parameter store to
// retrieve parameter value in batches
func GetParametersFromSSM(names []string, client SSMClient) (map[string]string, error) {
	return getParameters(names, client, false)
}

func getParameters(names []string, client SSMClient, withDecryption bool) (map[string]string, error) {
	in := &ssm.GetParametersInput{
		Names:          names,
		WithDecryption: aws.Bool(withDecryption),
	}

	out, err := client.GetParameters(context.TODO(), in)
	if err != nil {
		return nil, err
	}

	return extractSSMValues(out)
}

func extractSSMValues(out *ssm.GetParametersOutput) (map[string]string, error) {
	if out == nil {
		return nil, errors.New(
			"empty response")
	}

	if len(out.InvalidParameters) != 0 {
		return nil, fmt.Errorf(
			"invalid parameters: %s", strings.Join(out.InvalidParameters, ","))
	}

	parameterValues := make(map[string]string)
	for _, parameter := range out.Parameters {
		parameterValues[aws.ToString(parameter.Name)] = aws.ToString(parameter.Value)
	}

	return parameterValues, nil
}
