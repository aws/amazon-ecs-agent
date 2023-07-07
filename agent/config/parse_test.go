package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseReservedPorts(t *testing.T) {
	envVar := "ECS_RESERVED_PORTS"
	// unset value
	v := parseReservedPorts(envVar)
	assert.Empty(t, v)
	// invalid value
	t.Setenv(envVar, "1,2,3")
	v = parseReservedPorts(envVar)
	assert.Empty(t, v)
	// valid value
	t.Setenv(envVar, "[1,2,3]")
	v = parseReservedPorts(envVar)
	assert.Equal(t, []uint16{1, 2, 3}, v)
	// number too large
	t.Setenv(envVar, "[1,2,3,99999999999]")
	v = parseReservedPorts(envVar)
	assert.Equal(t, []uint16{1, 2, 3, 0}, v)
}

func TestParseVolumePluginCapabilities(t *testing.T) {
	// unset value
	t.Setenv("ECS_VOLUME_PLUGIN_CAPABILITIES", "")
	v := parseVolumePluginCapabilities()
	assert.Empty(t, v)
	// invalid value
	t.Setenv("ECS_VOLUME_PLUGIN_CAPABILITIES", "1,2,3")
	v = parseVolumePluginCapabilities()
	assert.Empty(t, v)
	// valid value
	t.Setenv("ECS_VOLUME_PLUGIN_CAPABILITIES", "[1,2,3]")
	v = parseVolumePluginCapabilities()
	assert.Equal(t, []string{"", "", ""}, v)
	// valid values
	t.Setenv("ECS_VOLUME_PLUGIN_CAPABILITIES", `["cap1","cap2"]`)
	v = parseVolumePluginCapabilities()
	assert.Equal(t, []string{"cap1", "cap2"}, v)
}

func TestParseNumImagesToDeletePerCycle(t *testing.T) {
	// unset value
	t.Setenv("ECS_NUM_IMAGES_DELETE_PER_CYCLE", "")
	v := parseNumImagesToDeletePerCycle()
	assert.Zero(t, v)
	// invalid value
	t.Setenv("ECS_NUM_IMAGES_DELETE_PER_CYCLE", "1")
	v = parseNumImagesToDeletePerCycle()
	assert.Equal(t, 1, v)
	// valid value
	t.Setenv("ECS_NUM_IMAGES_DELETE_PER_CYCLE", "2000000")
	v = parseNumImagesToDeletePerCycle()
	assert.Equal(t, 2000000, v)
	// valid values
	t.Setenv("ECS_NUM_IMAGES_DELETE_PER_CYCLE", "foobar")
	v = parseNumImagesToDeletePerCycle()
	assert.Zero(t, v)
}

func TestParseNumNonECSContainersToDeletePerCycle(t *testing.T) {
	// unset value
	t.Setenv("NONECS_NUM_CONTAINERS_DELETE_PER_CYCLE", "")
	v := parseNumNonECSContainersToDeletePerCycle()
	assert.Zero(t, v)
	// invalid value
	t.Setenv("NONECS_NUM_CONTAINERS_DELETE_PER_CYCLE", "1")
	v = parseNumNonECSContainersToDeletePerCycle()
	assert.Equal(t, 1, v)
	// valid value
	t.Setenv("NONECS_NUM_CONTAINERS_DELETE_PER_CYCLE", "2000000")
	v = parseNumNonECSContainersToDeletePerCycle()
	assert.Equal(t, 2000000, v)
	// valid values
	t.Setenv("NONECS_NUM_CONTAINERS_DELETE_PER_CYCLE", "foobar")
	v = parseNumNonECSContainersToDeletePerCycle()
	assert.Zero(t, v)
}

func TestParseBooleanDefaultFalseConfig(t *testing.T) {
	t.Setenv("ECS_PARSE_BOOLEAN_DEFAULT_FALSE", "")
	v := parseBooleanDefaultFalseConfig("ECS_PARSE_BOOLEAN_DEFAULT_FALSE")
	assert.False(t, v.Enabled())
	t.Setenv("ECS_PARSE_BOOLEAN_DEFAULT_FALSE", "true")
	v = parseBooleanDefaultFalseConfig("ECS_PARSE_BOOLEAN_DEFAULT_FALSE")
	assert.True(t, v.Enabled())
	t.Setenv("ECS_PARSE_BOOLEAN_DEFAULT_FALSE", "false")
	v = parseBooleanDefaultFalseConfig("ECS_PARSE_BOOLEAN_DEFAULT_FALSE")
	assert.False(t, v.Enabled())
	t.Setenv("ECS_PARSE_BOOLEAN_DEFAULT_FALSE", "foobar")
	v = parseBooleanDefaultFalseConfig("ECS_PARSE_BOOLEAN_DEFAULT_FALSE")
	assert.False(t, v.Enabled())
}

func TestParseBooleanDefaultTrueConfig(t *testing.T) {
	t.Setenv("ECS_PARSE_BOOLEAN_DEFAULT_TRUE", "")
	v := parseBooleanDefaultTrueConfig("ECS_PARSE_BOOLEAN_DEFAULT_TRUE")
	assert.True(t, v.Enabled())
	t.Setenv("ECS_PARSE_BOOLEAN_DEFAULT_TRUE", "true")
	v = parseBooleanDefaultTrueConfig("ECS_PARSE_BOOLEAN_DEFAULT_TRUE")
	assert.True(t, v.Enabled())
	t.Setenv("ECS_PARSE_BOOLEAN_DEFAULT_TRUE", "false")
	v = parseBooleanDefaultTrueConfig("ECS_PARSE_BOOLEAN_DEFAULT_TRUE")
	assert.False(t, v.Enabled())
	t.Setenv("ECS_PARSE_BOOLEAN_DEFAULT_TRUE", "foobar")
	v = parseBooleanDefaultTrueConfig("ECS_PARSE_BOOLEAN_DEFAULT_TRUE")
	assert.True(t, v.Enabled())
}

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
