package logging

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
)

var (
	logFile *os.File
	logger  *log.Logger
	debug   bool
)

// Init initializes the logging system. It sets up a log file and a
// multi-writer to output to both the file and any provided extra writers.
func Init(logFilePath string, extraWriters ...io.Writer) error {
	var err error
	logDir := filepath.Dir(logFilePath)
	if err = os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}

	writers := []io.Writer{logFile}
	writers = append(writers, extraWriters...)
	multiWriter := io.MultiWriter(writers...)

	logger = log.New(multiWriter, "", log.LstdFlags)
	logger.Println("Logging initialized.")
	return nil
}

func SetDebug(enable bool) {
	debug = enable
}

// ChannelWriter is an io.Writer that sends log messages to a channel.
// It supports graceful shutdown to prevent race conditions.
type ChannelWriter struct {
	LogChannel chan<- []byte
	ctx        context.Context
}

// NewChannelWriter creates a new writer that sends log messages to a channel.
func NewChannelWriter(ctx context.Context, ch chan<- []byte) *ChannelWriter {
	return &ChannelWriter{
		LogChannel: ch,
		ctx:        ctx,
	}
}

// Write implements the io.Writer interface. It is race-free on shutdown.
func (cw *ChannelWriter) Write(p []byte) (n int, err error) {
	// Check if shutdown has been initiated.
	select {
	case <-cw.ctx.Done():
		// Context is cancelled, do not write.
		return len(p), nil
	default:
		// Continue
	}

	msg := make([]byte, len(p))
	copy(msg, p)
	select {
	case cw.LogChannel <- msg:
	default:
		// Channel is full, drop log message to UI to prevent blocking.
	}
	return len(p), nil
}

// CaptureWriter is an io.Writer that captures the first log line.
type CaptureWriter struct {
	Buffer *bytes.Buffer
}

// Write implements io.Writer, capturing data to its buffer.
func (cw *CaptureWriter) Write(p []byte) (n int, err error) {
	if cw.Buffer == nil {
		cw.Buffer = &bytes.Buffer{}
	}
	return cw.Buffer.Write(p)
}

// Info logs an informational message.
func Info(v ...interface{}) {
	if logger == nil {
		return
	}
	logger.Println(v...)
}

// Infof logs a formatted informational message.
func Infof(format string, v ...interface{}) {
	if logger == nil {
		return
	}
	logger.Printf(format, v...)
}

// Warn logs a warning message.
func Warn(v ...interface{}) {
	if logger == nil {
		return
	}
	logger.Println(append([]interface{}{"WARN:"}, v...)...)
}

// Warnf logs a formatted warning message.
func Warnf(format string, v ...interface{}) {
	if logger == nil {
		return
	}
	logger.Printf("WARN: "+format, v...)
}

// Error logs an error message.
func Error(v ...interface{}) {
	if logger == nil {
		return
	}
	logger.Println(append([]interface{}{"ERROR:"}, v...)...)
}

// Errorf logs a formatted error message.
func Errorf(format string, v ...interface{}) {
	if logger == nil {
		return
	}
	logger.Printf("ERROR: "+format, v...)
}

// Warn logs a warning message.
func Debug(v ...interface{}) {
	if logger == nil || !debug {
		return
	}
	logger.Println(append([]interface{}{"WARN:"}, v...)...)
}

// Warnf logs a formatted warning message.
func Debugf(format string, v ...interface{}) {
	if logger == nil || !debug {
		return
	}
	logger.Printf("DEBUG: "+format, v...)
}

// Close gracefully closes the log file handle.
func Close() {
	if logFile != nil {
		if logger != nil {
			logger.Println("Closing log file.")
		}
		logFile.Close()
	}
}
