package doctor

import (
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	log "github.com/cihub/seelog"
)

type customHealthcheck struct {
	commonHealthcheck
	Command string `json:"command,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

// NewCustomHealthchecks returns a list of custom healthchecks.
// It parses the healthcheck configuration from a healthcheck.json file.
func NewCustomHealthchecks() []doctor.Healthcheck {
	// TODO: parse the list of health check
	customHealthchecks := []doctor.Healthcheck{
		newCustomHealthcheck(),
	}
	return customHealthchecks
}

// newCustomHealthcheck returns a custom healthcheck object.
func newCustomHealthcheck() *customHealthcheck {
	// TODO: Parse single health check here
	name := "SSM_CHECK"
	command := "sudo systemctl is-active amazon-ssm-agent"
	timeout := 10

	nowTime := time.Now()
	return &customHealthcheck{
		commonHealthcheck: commonHealthcheck{
			HealthcheckType:  name,
			Status:           doctor.HealthcheckStatusInitializing,
			TimeStamp:        nowTime,
			StatusChangeTime: nowTime,
		},
		Command: command,
		Timeout: timeout,
	}
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
