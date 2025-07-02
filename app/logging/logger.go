package logging

import (
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"
)

// Logger is a central logger that writes to a store and an optional io.Writer.
type Logger struct {
	mu     sync.Mutex
	store  *LogStore
	writer io.Writer
	goLog  *log.Logger
	debug  bool
}

// NewLogger creates and initializes a new Logger instance.
func NewLogger() *Logger {
	l := &Logger{
		store:  newLogStore(),
		writer: io.Discard, // Default to discarding output
	}
	l.goLog = log.New(l, "", 0) // The logger will write through our Write method
	return l
}

// Write implements the io.Writer interface. This allows the standard log package
// to write through our logger, which will then dispatch to the configured writer.
func (l *Logger) Write(p []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.writer == nil {
		return len(p), nil
	}
	return l.writer.Write(p)
}

// SetWriter sets the output destination for the logger.
func (l *Logger) SetWriter(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writer = w
}

// GetWriter returns the current output writer.
func (l *Logger) GetWriter() io.Writer {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.writer
}

// Store returns the internal LogStore.
func (l *Logger) Store() *LogStore {
	return l.store
}

// SetDebug enables or disables debug-level logging.
func (l *Logger) SetDebug(enable bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debug = enable
}

func (l *Logger) IsDebugEnabled() bool {
	return l.debug
}

// log is the internal handler for variadic logging.
func (l *Logger) log(level LogLevel, v ...interface{}) {
	if level == LevelDebug && !l.debug {
		return
	}
	// Use fmt.Sprint to handle the slice of interfaces.
	message := strings.TrimSpace(fmt.Sprintln(v...))
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
	}
	l.store.Add(entry)

	logLine := fmt.Sprintf("%s %-5s %s", entry.Timestamp.Format("15:04:05.000"), level.String(), message)
	l.goLog.Println(logLine)
}

// logf is the internal handler for formatted logging.
func (l *Logger) logf(level LogLevel, format string, v ...interface{}) {
	if level == LevelDebug && !l.debug {
		return
	}

	message := fmt.Sprintf(format, v...)
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
	}
	l.store.Add(entry)

	logLine := fmt.Sprintf("%s %-5s %s", entry.Timestamp.Format("15:04:05.000"), level.String(), message)
	l.goLog.Println(logLine)
}

// Info logs an informational message.
func (l *Logger) Info(v ...interface{}) {
	l.log(LevelInfo, v...)
}

// Infof logs a formatted informational message.
func (l *Logger) Infof(format string, v ...interface{}) {
	l.logf(LevelInfo, format, v...)
}

// Warn logs a warning message.
func (l *Logger) Warn(v ...interface{}) {
	l.log(LevelWarn, v...)
}

// Warnf logs a formatted warning message.
func (l *Logger) Warnf(format string, v ...interface{}) {
	l.logf(LevelWarn, format, v...)
}

// Error logs an error message.
func (l *Logger) Error(v ...interface{}) {
	l.log(LevelError, v...)
}

// Errorf logs a formatted error message.
func (l *Logger) Errorf(format string, v ...interface{}) {
	l.logf(LevelError, format, v...)
}

// Debug logs a debug message.
func (l *Logger) Debug(v ...interface{}) {
	l.log(LevelDebug, v...)
}

// Debugf logs a formatted debug message.
func (l *Logger) Debugf(format string, v ...interface{}) {
	l.logf(LevelDebug, format, v...)
}

// ---- Global / Default Logger ----

var defaultLogger = NewLogger()

// SetDefault replaces the default logger instance.
func SetDefault(logger *Logger) {
	if logger != nil {
		defaultLogger = logger
	}
}

func IsDebugEnabled() bool {
	return defaultLogger.debug
}

// Info logs an informational message using the default logger.
func Info(v ...interface{}) {
	defaultLogger.Info(v...)
}

// Infof logs a formatted informational message using the default logger.
func Infof(format string, v ...interface{}) {
	defaultLogger.Infof(format, v...)
}

// Warn logs a warning message using the default logger.
func Warn(v ...interface{}) {
	defaultLogger.Warn(v...)
}

// Warnf logs a formatted warning message using the default logger.
func Warnf(format string, v ...interface{}) {
	defaultLogger.Warnf(format, v...)
}

// Error logs an error message using the default logger.
func Error(v ...interface{}) {
	defaultLogger.Error(v...)
}

// Errorf logs a formatted error message using the default logger.
func Errorf(format string, v ...interface{}) {
	defaultLogger.Errorf(format, v...)
}

// Debug logs a debug message using the default logger.
func Debug(v ...interface{}) {
	defaultLogger.Debug(v...)
}

// Debugf logs a formatted debug message using the default logger.
func Debugf(format string, v ...interface{}) {
	defaultLogger.Debugf(format, v...)
}
