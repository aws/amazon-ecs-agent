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

package uri

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
)

const (
	// FIPSKeyword is the keyword used in FIPS endpoints
	// e.g. 123456789012.dkr.ecr-fips.us-west-2.amazonaws.com
	FIPSKeyword = "-fips"
)

var (
	// ECRImagePattern matches ECR image URLs in various formats:
	// e.g. 123456789012.dkr.ecr-fips.us-west-2.amazonaws.com
	// 1234567.dkr.ecr-fips.us-iso-east-1.c2s.ic.gov/
	// 1234567.dkr.ecr.us-isob-east-1.sc2s.sgov.gov/
	// 123456789.dkr.ecr.cn-north-1.amazonaws.com.cn/
	// 98765432.dkr.starport.us-west-2.amazonaws.com/
	// 98765432.dkr.starport-fips.us-west-2.amazonaws.com/
	ECRImagePattern = regexp.MustCompile(`(^[a-zA-Z0-9][a-zA-Z0-9-_]*)\.dkr\.((ecr|starport)(` + FIPSKeyword + `)?)\.([\w\-_]+)\.([\w\.\-]+).*`)

	// ECRDualStackImagePattern matches ECR dual-stack image URLs in various formats:
	// 123456789012.dkr-ecr.us-west-2.on.aws
	// 123456789012.dkr-ecr-fips.us-west-2.on.aws
	// 123456789012.dkr-ecr.cn-north-1.on.amazonwebservices.com.cn
	// 123456789012.dkr-ecr.eusc-de-east-1.amazonwebservices.eu
	// 123456789012.dkr-ecr.us-iso-west-1.on.aws.ic.gov
	// 123456789012.dkr-ecr.us-isob-east-1.on.aws.scloud
	// 123456789012.dkr-ecr.us-isof-south-1.on.aws.hci.ic.gov
	// 123456789012.dkr-ecr.eu-isoe-west-1.on.cloud-aws.adc-e.uk
	ECRDualStackImagePattern = regexp.MustCompile(`(^[a-zA-Z0-9][a-zA-Z0-9-_]*)\.dkr-((ecr|starport)(` + FIPSKeyword + `)?)\.([\w\-_]+)\.([\w\.\-]+).*`)
)

func ParseURI(uri string, pattern *regexp.Regexp, dualstackPattern *regexp.Regexp) (bool, bool, bool) {
	matchedPattern := pattern.MatchString(uri)
	matchedDualstackPattern := dualstackPattern.MatchString(uri)
	// match either ipv4/dualstack and contains FIPSKeyword
	foundFips := (matchedPattern || matchedDualstackPattern) && strings.Contains(uri, FIPSKeyword)

	logger.Info(fmt.Sprintf("URI: %s, Matched Pattern: %t, Matched Dualstack Pattern: %t, IsFIPS: %t", uri, matchedPattern, matchedDualstackPattern, foundFips))
	return matchedPattern, matchedDualstackPattern, foundFips
}
