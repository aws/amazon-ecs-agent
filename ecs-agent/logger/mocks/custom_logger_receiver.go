package mock_seelog

import (
	"sync"

	"github.com/cihub/seelog"
)

type CustomLoggerReceiver struct {
	OutputFormat        string
	Mu                  sync.Mutex
	TraceCalled         bool
	LastTraceMessage    string
	DebugCalled         bool
	LastDebugMessage    string
	InfoCalled          bool
	LastInfoMessage     string
	WarnCalled          bool
	LastWarnMessage     string
	ErrorCalled         bool
	LastErrorMessage    string
	CriticalCalled      bool
	LastCriticalMessage string
}

func (m *CustomLoggerReceiver) GetOutputFormat() string {
	return m.OutputFormat
}

func (m *CustomLoggerReceiver) Trace(message string) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.TraceCalled = true
	m.LastTraceMessage = message
}

func (m *CustomLoggerReceiver) Debug(message string) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.DebugCalled = true
	m.LastDebugMessage = message
}

func (m *CustomLoggerReceiver) Info(message string) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.InfoCalled = true
	m.LastInfoMessage = message
}

func (m *CustomLoggerReceiver) Warn(message string) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.WarnCalled = true
	m.LastWarnMessage = message
}

func (m *CustomLoggerReceiver) Error(message string) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.ErrorCalled = true
	m.LastErrorMessage = message
}

func (m *CustomLoggerReceiver) Critical(message string) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.CriticalCalled = true
	m.LastCriticalMessage = message
}

func (m *CustomLoggerReceiver) Flush() error {
	return nil
}
func (m *CustomLoggerReceiver) AfterParse(args interface{}) error {
	return nil
}

func (m *CustomLoggerReceiver) Close() error {
	return nil
}

func (m *CustomLoggerReceiver) ReceiveMessage(message string, level seelog.LogLevel,
	context seelog.LogContextInterface) error {

	switch level {
	case seelog.DebugLvl:
		m.Debug(message)
	case seelog.InfoLvl:
		m.Info(message)
	case seelog.WarnLvl:
		m.Warn(message)
	case seelog.ErrorLvl:
		m.Error(message)
	case seelog.CriticalLvl:
		m.Error(message)
	}
	return nil
}
