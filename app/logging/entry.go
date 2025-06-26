package logging

import "time"

// LogLevel defines the severity of a log entry.
type LogLevel int

// Enum for log levels. The order is important for filtering.
const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String returns the string representation of a LogLevel.
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// LogEntry represents a single, structured log message.
type LogEntry struct {
	Timestamp time.Time
	Level     LogLevel
	Message   string
}
