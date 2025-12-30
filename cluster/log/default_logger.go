package log

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
)

var defaultLogger *Logger // Singleton instance of the logger.
var mu sync.Mutex         // Mutex for ensuring thread safety when initializing the logger.

// use this in place of any non-slog io.Writer in submodules to run the output through our logger's format
type WrappedWriter struct {
	Tag string
}

func (w *WrappedWriter) Write(p []byte) (n int, err error) {
	if defaultLogger != nil {
		defaultLogger.Logger.Info(string(p), "tag", w.Tag)
	}
	return len(p), nil
}

// Init initializes the logger with the provided options.
func Init(opt *Options) {
	mu.Lock()
	defer mu.Unlock()
	defaultLogger = New(opt)
}

// Default returns the default logger instance.
func Default() *slog.Logger {
	return defaultLogger.Logger
}

// Writer returns the writer associated with the logger.
func Writer() io.Writer {
	if defaultLogger != nil {
		return defaultLogger.writer
	}
	return nil
}

// Info logs an informational message with optional arguments.
func Info(msg string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Info(msg, args...)
	}
}

// Warn logs a warning message with optional arguments.
func Warn(msg string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Warn(msg, args...)
	}
}

// Error logs an error message with optional arguments.
func Error(msg string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Error(msg, args...)
	}
}

// Fatal logs a fatal error message with optional arguments and exits the program.
func Fatal(msg string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Error(msg, args...)
		os.Exit(1)
	}
}

// Debug logs a debug message with optional arguments.
func Debug(msg string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Debug(msg, args...)
	}
}

// Log logs a message with the specified log level and optional arguments.
func Log(level slog.Level, msg string, args ...interface{}) {
	if defaultLogger != nil {
		slevel := slog.Level(level)
		defaultLogger.Log(context.TODO(), slevel, msg, args...)
	}
}
