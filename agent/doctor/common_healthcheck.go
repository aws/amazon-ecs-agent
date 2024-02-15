package doctor

import (
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
)

type commonHealthcheck struct {
	// HealthcheckType is the reported healthcheck type
	HealthcheckType string `json:"HealthcheckType,omitempty"`
	// Status is the container health status
	Status doctor.HealthcheckStatus `json:"HealthcheckStatus,omitempty"`
	// Timestamp is the timestamp when container health status changed
	TimeStamp time.Time `json:"TimeStamp,omitempty"`
	// StatusChangeTime is the latest time the health status changed
	StatusChangeTime time.Time `json:"StatusChangeTime,omitempty"`
	// LastStatus is the last container health status
	LastStatus doctor.HealthcheckStatus `json:"LastStatus,omitempty"`
	// LastTimeStamp is the timestamp of last container health status
	LastTimeStamp time.Time `json:"LastTimeStamp,omitempty"`

	lock sync.RWMutex
}
