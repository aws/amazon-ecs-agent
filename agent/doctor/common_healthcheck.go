package doctor

import (
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
)

type commonHealthcheckConfig struct {
	// HealthcheckName is the reported healthcheck name
	HealthcheckName string `json:"HealthcheckName,omitempty"`
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
	// lock is a reader/writer mutual exclusion lock
	lock sync.RWMutex
}
