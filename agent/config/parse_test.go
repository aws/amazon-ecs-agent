package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseContainerInstanceTags(t *testing.T) {
	// empty
	t.Setenv("ECS_CONTAINER_INSTANCE_TAGS", "")
	var expected, actual map[string]string
	expectedErrs := []error{}
	actualErrs := []error{}
	actual, actualErrs = parseContainerInstanceTags(actualErrs)
	assert.Equal(t, expected, actual)
	assert.Equal(t, expectedErrs, actualErrs)
	// with valid values
	t.Setenv("ECS_CONTAINER_INSTANCE_TAGS", `{"foo":"bar","baz":"bin","num":"7"}`)
	expected = map[string]string{"baz": "bin", "foo": "bar", "num": "7"}
	expectedErrs = []error{}
	actual, actualErrs = parseContainerInstanceTags(actualErrs)
	assert.Equal(t, expected, actual)
	assert.Equal(t, expectedErrs, actualErrs)
	// with invalid values
	t.Setenv("ECS_CONTAINER_INSTANCE_TAGS", `{"foo":"bar","baz":"bin,"num":"7"}`) // missing "
	var expectedInvalid map[string]string
	expectedErrs = []error{fmt.Errorf("Invalid format for ECS_CONTAINER_INSTANCE_TAGS. Expected a json hash: invalid character 'n' after object key:value pair")}
	actual, actualErrs = parseContainerInstanceTags(actualErrs)
	assert.Equal(t, expectedInvalid, actual)
	assert.Equal(t, expectedErrs, actualErrs)
}
