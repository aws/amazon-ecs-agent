package doctor

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	log "github.com/cihub/seelog"
)

const customHealthcheckFile = "/etc/ecs/healthcheck.json"

type customHealthcheck struct {
	// HealthcheckType is the reported healthcheck type
	HealthcheckType string `json:"Name,omitempty"`
	// Command is the custom healthcheck command
	Command string `json:"Command,omitempty"`
	// Timeout is the timeout for the custom healthcheck command
	Timeout int `json:"Timeout,omitempty"`
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
	lock          sync.RWMutex
}

// NewCustomHealthchecks returns a list of custom healthchecks.
// It parses the healthcheck configuration from a healthcheck.json file.
func NewCustomHealthchecks() []customHealthcheck {
	var customHealthcheckList []customHealthcheck
	_, err := os.Stat(customHealthcheckFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Infof("Custom healthcheck file not found: %v", customHealthcheckFile)
		} else {
			log.Errorf("error stat file, err: %v", err)
		}
		return customHealthcheckList
	}
	data, _ := os.ReadFile(customHealthcheckFile)
	err = json.Unmarshal(data, &customHealthcheckList)
	if err != nil {
		log.Errorf("error unmarshalling healthcheck json file, err: %v", err)
	}
	return newCustomHealthchecks(customHealthcheckList)
}

// newCustomHealthcheck returns a list of custom healthcheck objects.
func newCustomHealthchecks(input []customHealthcheck) []customHealthcheck {
	var result []customHealthcheck
	nowTime := time.Now()
	for _, hc := range input {
		name := hc.HealthcheckType
		command := hc.Command
		timeout := hc.Timeout
		r := customHealthcheck{
			HealthcheckType:  name,
			Status:           doctor.HealthcheckStatusInitializing,
			TimeStamp:        nowTime,
			StatusChangeTime: nowTime,
			Command:          command,
			Timeout:          timeout,
		}
		result = append(result, r)
	}
	return result
}

// RunCheck runs the custom healthcheck command and returns the result.
func (chc *customHealthcheck) RunCheck() doctor.HealthcheckStatus {
	res := runCustomHealthCheckCmd(chc.Command, chc.Timeout)
	resultStatus := doctor.HealthcheckStatusOk
	if res != "0" {
		log.Infof("Custom healthcheck return non-zero exit code: %v", res)
		resultStatus = doctor.HealthcheckStatusImpaired
	}
	chc.SetHealthcheckStatus(resultStatus)
	return resultStatus
}

// SetHealthcheckStatus sets the healthcheck status.
func (chc *customHealthcheck) SetHealthcheckStatus(healthStatus doctor.HealthcheckStatus) {
	chc.lock.Lock()
	defer chc.lock.Unlock()
	nowTime := time.Now()
	// if the status has changed, update status change timestamp
	if chc.Status != healthStatus {
		chc.StatusChangeTime = nowTime
	}
	// track previous status
	chc.LastStatus = chc.Status
	chc.LastTimeStamp = chc.TimeStamp

	// update latest status
	chc.Status = healthStatus
	chc.TimeStamp = nowTime
}

// GetHealthcheckType returns the healthcheck type.
func (chc *customHealthcheck) GetHealthcheckType() string {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.HealthcheckType
}

// GetHealthcheckStatus returns the healthcheck status.
func (chc *customHealthcheck) GetHealthcheckStatus() doctor.HealthcheckStatus {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.Status
}

// GetHealthcheckTime returns the healthcheck time.
func (chc *customHealthcheck) GetHealthcheckTime() time.Time {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.TimeStamp
}

// GetStatusChangeTime returns the time when the status changed.
func (chc *customHealthcheck) GetStatusChangeTime() time.Time {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.StatusChangeTime
}

// GetLastHealthcheckStatus returns the last healthcheck status.
func (chc *customHealthcheck) GetLastHealthcheckStatus() doctor.HealthcheckStatus {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.LastStatus
}

// GetLastHealthcheckTime returns the last healthcheck time.
func (chc *customHealthcheck) GetLastHealthcheckTime() time.Time {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.LastTimeStamp
}
