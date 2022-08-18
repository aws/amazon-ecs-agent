package types

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/stretchr/testify/assert"
)

// Helper function for testing taskProtectionTypeFromString
func testTaskProtectionTypeFromString(t *testing.T, input string, expected taskProtectionType) {
	output, err := taskProtectionTypeFromString(input)
	assert.NoError(t, err)
	assert.Equal(t, expected, output)
}

func TestTaskProtectionTypeFromStringScaleIn(t *testing.T) {
	testTaskProtectionTypeFromString(t, "SCALE_IN", TaskProtectionTypeScaleIn)
}

func TestTaskProtectionTypeFromStringScaleInLower(t *testing.T) {
	testTaskProtectionTypeFromString(t, "scale_in", TaskProtectionTypeScaleIn)
}

func TestTaskProtectionTypeFromStringDisabled(t *testing.T) {
	testTaskProtectionTypeFromString(t, "DISABLED", TaskProtectionTypeDisabled)
}

func TestTaskProtectionTypeFromStringDisabledLower(t *testing.T) {
	testTaskProtectionTypeFromString(t, "disabled", TaskProtectionTypeDisabled)
}

func TestTaskProtectionTypeFromStringError(t *testing.T) {
	_, err := taskProtectionTypeFromString("unknown")
	assert.EqualError(t, err, "unknown task protection type: unknown")
}

// Helper function for testing newTaskProtection
func testNewTaskProtectionError(t *testing.T, protectionTypeStr string, timeoutMinutes *int, expectedErr string) {
	protection, err := NewTaskProtection(protectionTypeStr, timeoutMinutes)
	assert.EqualError(t, err, expectedErr)
	assert.Nil(t, protection)
}

// Tests newTaskProtection when protection type is invalid
func TestNewTaskProtectionTypeInvalid(t *testing.T) {
	testNewTaskProtectionError(t, "bad", nil,
		"protection type is invalid: unknown task protection type: bad")
}

// Tests newTaskProtection when timeout is invalid
func TestNewTaskProtectionTimeoutInvalid(t *testing.T) {
	testNewTaskProtectionError(t, string(TaskProtectionTypeScaleIn), utils.IntPtr(-3),
		"protection timeout must be greater than zero")
}

// Tests that newTaskProtection can accept nil timeout
func TestNewTaskProtectionNilTimeout(t *testing.T) {
	protection, err := NewTaskProtection(string(TaskProtectionTypeScaleIn), nil)
	assert.NoError(t, err)
	assert.Equal(t, protection,
		&taskProtection{protectionType: TaskProtectionTypeScaleIn, protectionTimeoutMinutes: nil})
}

// Tests newTaskProtection happy path
func TestNewTaskProtectionHappy(t *testing.T) {
	protection, err := NewTaskProtection(string(TaskProtectionTypeScaleIn), utils.IntPtr(5))
	assert.NoError(t, err)
	assert.Equal(t, protection,
		&taskProtection{protectionType: TaskProtectionTypeScaleIn, protectionTimeoutMinutes: utils.IntPtr(5)})
}
