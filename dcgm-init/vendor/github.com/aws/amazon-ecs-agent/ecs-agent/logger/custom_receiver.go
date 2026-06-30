package logger

import (
	"fmt"

	"github.com/cihub/seelog"
)

// CustomReceiver defines the interface for custom logging implementations
type CustomReceiver interface {
	GetOutputFormat() string
	Trace(message string)
	Debug(message string)
	Info(message string)
	Warn(message string)
	Error(message string)
	Critical(message string)
	Flush() error
	Close() error
}

// internal wrapper that implements seelog.CustomReceiver
type customReceiverWrapper struct {
	receiver CustomReceiver
}

func (w *customReceiverWrapper) ReceiveMessage(message string, level seelog.LogLevel, context seelog.LogContextInterface) error {

	switch level {
	case seelog.TraceLvl:
		w.receiver.Trace(message)
	case seelog.DebugLvl:
		w.receiver.Debug(message)
	case seelog.InfoLvl:
		w.receiver.Info(message)
	case seelog.WarnLvl:
		w.receiver.Warn(message)
	case seelog.ErrorLvl:
		w.receiver.Error(message)
	case seelog.CriticalLvl:
		w.receiver.Critical(message)
	default:
		fmt.Printf("Unhandled level: %v", level)
	}
	return nil
}

func (w *customReceiverWrapper) AfterParse(initArgs seelog.CustomReceiverInitArgs) error {
	return nil
}

func (w *customReceiverWrapper) Flush() {
	err := w.receiver.Flush()
	if err != nil {
		fmt.Printf("Couldn't flush the logger due to: %v", err)
	}
}

func (w *customReceiverWrapper) Close() error {
	return w.receiver.Close()
}
