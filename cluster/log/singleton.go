package log

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
)

var singleton *Logger
var mu sync.Mutex

func Init(opt *Options) {
	mu.Lock()
	defer mu.Unlock()
	singleton = New(opt)
}

func Singleton() *slog.Logger {
	return singleton.Logger
}

func Writer() io.Writer {
	if singleton != nil {
		return singleton.writer
	}
	return nil
}

func Info(msg string, args ...any) {
	if singleton != nil {
		singleton.Info(msg, args...)
	}
}

func Warn(msg string, args ...any) {
	if singleton != nil {
		singleton.Warn(msg, args...)
	}
}

func Error(msg string, args ...any) {
	if singleton != nil {
		singleton.Error(msg, args...)
	}
}

func Fatal(msg string, args ...any) {
	if singleton != nil {
		singleton.Error(msg, args...)
		os.Exit(1)
	}
}

func Debug(msg string, args ...any) {
	if singleton != nil {
		singleton.Debug(msg, args...)
	}
}

func Log(level slog.Level, msg string, args ...any) {
	if singleton != nil {
		slevel := slog.Level(level)
		singleton.Log(context.TODO(), slevel, msg, args...)
	}
}
