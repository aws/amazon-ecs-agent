package types

import (
	"fmt"
	"strings"
)

// Protection type to set on the task
type taskProtectionType string

const (
	TaskProtectionTypeScaleIn  taskProtectionType = "SCALE_IN"
	TaskProtectionTypeDisabled taskProtectionType = "DISABLED"
	// TODO Check if DISABLED_PERMANENTLY protection type is to be supported
)

// Converts a string to taskProtectionType
func taskProtectionTypeFromString(s string) (taskProtectionType, error) {
	switch p := strings.ToUpper(s); p {
	case string(TaskProtectionTypeScaleIn):
		return TaskProtectionTypeScaleIn, nil
	case string(TaskProtectionTypeDisabled):
		return TaskProtectionTypeDisabled, nil
	default:
		return "", fmt.Errorf("unknown task protection type: %v", s)
	}
}

// Protection for a Task
type taskProtection struct {
	protectionType           taskProtectionType
	protectionTimeoutMinutes *int
}

// Creates a taskProtection value after validating the arguments
func NewTaskProtection(protectionType string, timeoutMinutes *int) (*taskProtection, error) {
	ptype, err := taskProtectionTypeFromString(protectionType)
	if err != nil {
		return nil, fmt.Errorf("protection type is invalid: %w", err)
	}

	if timeoutMinutes != nil && *timeoutMinutes <= 0 {
		return nil, fmt.Errorf("protection timeout must be greater than zero")
	}

	return &taskProtection{protectionType: ptype, protectionTimeoutMinutes: timeoutMinutes}, nil
}

func (taskProtection *taskProtection) GetProtectionType() taskProtectionType {
	return taskProtection.protectionType
}

func (taskProtection *taskProtection) GetProtectionTimeoutMinutes() *int {
	return taskProtection.protectionTimeoutMinutes
}
