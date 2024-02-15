package doctor

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/cihub/seelog"
	"time"
)

type customHealthcheck struct {
	commonHealthcheck
	Command string `json:"command,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

func ParseCustomHealthChecks() []doctor.Healthcheck {
	//TODO: parse the list of health check
	customHealthchecks := []doctor.Healthcheck{
		NewCustomHealthcheck(),
	}

	return customHealthchecks
}

func NewCustomHealthcheck() *customHealthcheck {
	//TODO: Parse single health check here
	name := "SSM_CHECK"
	command := "sudo systemctl is-active amazon-ssm-agent"
	timeout := 30

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

func (chc *customHealthcheck) RunCheck() doctor.HealthcheckStatus {
	//
	res := runCustomHealthCheckCmd(chc.Command, chc.Timeout)
	resultStatus := doctor.HealthcheckStatusOk
	if res != "0" {
		seelog.Infof("%s Docker Ping failed with error", "[CustomHealthCheck]")
		resultStatus = doctor.HealthcheckStatusImpaired
	}
	chc.SetHealthcheckStatus(resultStatus)
	return resultStatus
}

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

func (chc *customHealthcheck) GetHealthcheckType() string {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.HealthcheckType
}

func (chc *customHealthcheck) GetHealthcheckStatus() doctor.HealthcheckStatus {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.Status
}

func (chc *customHealthcheck) GetHealthcheckTime() time.Time {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.TimeStamp
}

func (chc *customHealthcheck) GetStatusChangeTime() time.Time {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.StatusChangeTime
}

func (chc *customHealthcheck) GetLastHealthcheckStatus() doctor.HealthcheckStatus {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.LastStatus
}

func (chc *customHealthcheck) GetLastHealthcheckTime() time.Time {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.LastTimeStamp
}
