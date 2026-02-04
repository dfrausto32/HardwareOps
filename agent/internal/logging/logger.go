package logging

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

func ParseLevel(val string) Level {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "debug":
		return Debug
	case "warn", "warning":
		return Warn
	case "error":
		return Error
	default:
		return Info
	}
}

type Logger struct {
	level     Level
	component string
	deviceID  string
	std       *log.Logger
	exporter  *Exporter
}

func New(level Level, component, deviceID string, exporter *Exporter) *Logger {
	return &Logger{
		level:     level,
		component: component,
		deviceID:  deviceID,
		std:       log.New(os.Stdout, "", log.LstdFlags),
		exporter:  exporter,
	}
}

func (l *Logger) SetDeviceID(deviceID string) {
	l.deviceID = deviceID
}

func (l *Logger) Debugf(format string, args ...any) { l.logf(Debug, format, args...) }
func (l *Logger) Infof(format string, args ...any)  { l.logf(Info, format, args...) }
func (l *Logger) Warnf(format string, args ...any)  { l.logf(Warn, format, args...) }
func (l *Logger) Errorf(format string, args ...any) { l.logf(Error, format, args...) }

func (l *Logger) logf(level Level, format string, args ...any) {
	if level < l.level {
		return
	}
	msg := fmt.Sprintf(format, args...)
	levelStr := level.String()
	prefix := fmt.Sprintf("%s component=%s", levelStr, l.component)
	if l.deviceID != "" {
		prefix += " device=" + l.deviceID
	}
	l.std.Printf("%s %s", prefix, msg)

	if l.exporter != nil {
		l.exporter.Send(LogEvent{
			Timestamp: time.Now().UTC(),
			Level:     levelStr,
			Component: l.component,
			DeviceID:  l.deviceID,
			Message:   msg,
		})
	}
}

func (l Level) String() string {
	switch l {
	case Debug:
		return "DEBUG"
	case Warn:
		return "WARN"
	case Error:
		return "ERROR"
	default:
		return "INFO"
	}
}
