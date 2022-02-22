package utils

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/pkg/errors"
)

const (
	arnResourceDelimiter = "/"
)

// TaskIdFromArn derives the task id from the task arn
// Reference: http://docs.aws.amazon.com/general/latest/gr/aws-arns-and-namespaces.html#arn-syntax-ecs
func TaskIdFromArn(taskArn string) (string, error) {
	// Parse taskARN
	parsedARN, err := arn.Parse(taskArn)
	if err != nil {
		return "", err
	}

	// Get task resource section
	resource := parsedARN.Resource

	if !strings.Contains(resource, arnResourceDelimiter) {
		return "", errors.New("malformed task ARN resource")
	}

	resourceSplit := strings.Split(resource, arnResourceDelimiter)

	return resourceSplit[len(resourceSplit)-1], nil
}
