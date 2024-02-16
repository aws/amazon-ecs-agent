package doctor

import (
	"encoding/json"
	"os"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	log "github.com/cihub/seelog"
)

const customHealthcheckFile = "/etc/ecs/healthcheck.json"

// customHealthcheckFromJSON represents the healthcheck input from the healthcheck.json file
type customHealthcheckFromJSON struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Timeout int    `json:"timeout,omitempty"`
}

// CustomHealthcheckConfig represents the custom healthcheck object
type CustomHealthcheckConfig struct {
	commonHealthcheckConfig
	Timeout int
}

// NewCustomHealthchecks returns a list of custom healthchecks.
// It parses the healthcheck configuration from a healthcheck.json file.
func NewCustomHealthchecks() []*CustomHealthcheckConfig {
	var hcJSON []customHealthcheckFromJSON
	_, err := os.Stat(customHealthcheckFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Infof("Custom healthcheck file not found: %v", customHealthcheckFile)
		} else {
			log.Errorf("error stat file, err: %v", err)
		}
		return nil
	}
	data, _ := os.ReadFile(customHealthcheckFile)
	err = json.Unmarshal(data, &hcJSON)
	if err != nil {
		log.Errorf("error unmarshalling healthcheck json file, err: %v", err)
		return nil
	}

	var result []*CustomHealthcheckConfig
	for _, hc := range hcJSON {
		result = append(result, newCustomHealthcheck(hc))
	}
	return result
}

// newCustomHealthcheck returns a list of custom healthcheck objects.
func newCustomHealthcheck(input customHealthcheckFromJSON) *CustomHealthcheckConfig {
	nowTime := time.Now()
	return &CustomHealthcheckConfig{
		commonHealthcheckConfig: commonHealthcheckConfig{
			HealthcheckName:  input.Name,
			HealthcheckType:  input.Type,
			Status:           doctor.HealthcheckStatusInitializing,
			TimeStamp:        nowTime,
			StatusChangeTime: nowTime,
		},
		Timeout: input.Timeout,
	}
}

// RunCheck runs the custom healthcheck command and returns the result.
func (chc *CustomHealthcheckConfig) RunCheck() doctor.HealthcheckStatus {
	res := runCustomHealthCheckCmd(chc.HealthcheckName, chc.HealthcheckType, chc.Timeout)
	resultStatus := doctor.HealthcheckStatusOk
	if res != "0" {
		log.Infof("Custom healthcheck return non-zero exit code: %v", res)
		resultStatus = doctor.HealthcheckStatusImpaired
	}
	chc.SetHealthcheckStatus(resultStatus)
	return resultStatus
}

// SetHealthcheckStatus sets the healthcheck status.
func (chc *CustomHealthcheckConfig) SetHealthcheckStatus(healthStatus doctor.HealthcheckStatus) {
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
func (chc *CustomHealthcheckConfig) GetHealthcheckType() string {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.HealthcheckType
}

// GetHealthcheckName returns the healthcheck type.
func (chc *CustomHealthcheckConfig) GetHealthcheckName() string {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.HealthcheckName
}

// GetHealthcheckStatus returns the healthcheck status.
func (chc *CustomHealthcheckConfig) GetHealthcheckStatus() doctor.HealthcheckStatus {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.Status
}

// GetHealthcheckTime returns the healthcheck time.
func (chc *CustomHealthcheckConfig) GetHealthcheckTime() time.Time {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.TimeStamp
}

// GetStatusChangeTime returns the time when the status changed.
func (chc *CustomHealthcheckConfig) GetStatusChangeTime() time.Time {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.StatusChangeTime
}

// GetLastHealthcheckStatus returns the last healthcheck status.
func (chc *CustomHealthcheckConfig) GetLastHealthcheckStatus() doctor.HealthcheckStatus {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.LastStatus
}

// GetLastHealthcheckTime returns the last healthcheck time.
func (chc *CustomHealthcheckConfig) GetLastHealthcheckTime() time.Time {
	chc.lock.RLock()
	defer chc.lock.RUnlock()
	return chc.LastTimeStamp
}
