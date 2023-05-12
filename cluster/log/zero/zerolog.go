// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package zero

import (
	"github.com/rs/zerolog"
	"github.com/wind-c/comqtt/v2/config"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"os"
	"time"
)

const (
	FormatConsole = iota
	FormatJson
)

var logger *zerolog.Logger
var cfg config.Log
var lws []*levelWriter

func Init(c config.Log) *zerolog.Logger {
	cfg = c
	if !cfg.Enable {
		zerolog.SetGlobalLevel(zerolog.Disabled)
	}

	// init zerolog format
	zerolog.TimeFieldFormat = "2006-01-02 15:04:05.000"
	zerolog.DurationFieldInteger = true
	zerolog.TimestampFieldName = "timestamp"
	zerolog.DurationFieldUnit = time.Millisecond
	zerolog.TimestampFieldName = "t"
	zerolog.LevelFieldName = "l"
	zerolog.MessageFieldName = "m"
	writers := zerolog.MultiLevelWriter()
	if cfg.Format == FormatJson {
		if w := getLogWriter(cfg.InfoFile); w != nil {
			lw := newLevelWriter(w, zerolog.Level(cfg.Level), zerolog.WarnLevel)
			lws = append(lws, lw)
			writers = zerolog.MultiLevelWriter(lw)
		}
		if w := getLogWriter(cfg.ErrorFile); w != nil {
			lw := newLevelWriter(w, zerolog.ErrorLevel, zerolog.PanicLevel)
			lws = append(lws, lw)
			writers = zerolog.MultiLevelWriter(writers, lw)
		}
		if w := getLogWriter(cfg.ThirdpartyFile); w != nil {
			lw := newLevelWriter(w, zerolog.NoLevel, zerolog.NoLevel)
			lws = append(lws, lw)
			writers = zerolog.MultiLevelWriter(writers, lw)
		}
	} else {
		writers = zerolog.MultiLevelWriter(stdErrWriter())
	}

	lg := zerolog.New(writers).With().Timestamp().Logger()
	if cfg.Caller {
		lg = zerolog.New(writers).With().Timestamp().Caller().Logger()
	}
	logger = &lg
	return logger
}

func Logger() *zerolog.Logger {
	return logger
}

func Close() {
	for _, w := range lws {
		w.Close()
	}
}

func getLogWriter(filename string) io.Writer {
	if filename != "" {
		lumberJackLogger := &lumberjack.Logger{
			Filename:   filename, // ⽇志⽂件路径
			MaxSize:    100,      // 1M=1024KB=1024000byte
			MaxBackups: 10,       // 最多保留10个备份
			MaxAge:     30,       // days
			Compress:   true,     // 是否压缩 disabled by default
		}
		if cfg.MaxSize != 0 {
			lumberJackLogger.MaxSize = cfg.MaxSize
		}
		if cfg.MaxBackups != 0 {
			lumberJackLogger.MaxBackups = cfg.MaxBackups
		}
		if cfg.MaxAge != 0 {
			lumberJackLogger.MaxAge = cfg.MaxAge
		}
		if !cfg.Compress {
			lumberJackLogger.Compress = false
		}

		return lumberJackLogger
	}

	return nil
}

type levelWriter struct {
	writer   io.Writer
	minLevel zerolog.Level
	maxLevel zerolog.Level
}

func newLevelWriter(w io.Writer, minLevel zerolog.Level, maxLevel zerolog.Level) *levelWriter {
	return &levelWriter{
		writer:   w,
		minLevel: minLevel,
		maxLevel: maxLevel,
	}
}

func (lw *levelWriter) Close() error {
	if c, ok := lw.writer.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (lw *levelWriter) WriteLevel(level zerolog.Level, p []byte) (n int, err error) {
	if level >= lw.minLevel && level <= lw.maxLevel {
		return lw.Write(p)
	}
	return len(p), nil
}

func (lw *levelWriter) Write(p []byte) (n int, err error) {
	return lw.writer.Write(p)
}

func stdOutWriter() zerolog.LevelWriter {
	cw := zerolog.NewConsoleWriter()
	cw.NoColor = false
	return newLevelWriter(cw, zerolog.DebugLevel, zerolog.WarnLevel)
}

func stdErrWriter() zerolog.LevelWriter {
	cw := zerolog.NewConsoleWriter()
	cw.Out = os.Stderr
	cw.NoColor = false
	return newLevelWriter(cw, zerolog.DebugLevel, zerolog.Disabled)
}

func Trace() *zerolog.Event {
	return logger.Trace()
}
func Debug() *zerolog.Event {
	return logger.Debug()
}
func Info() *zerolog.Event {
	return logger.Info()
}
func Warn() *zerolog.Event {
	return logger.Warn()
}
func Error() *zerolog.Event {
	return logger.Error()
}
func Fatal() *zerolog.Event {
	return logger.Fatal()
}
func Panic() *zerolog.Event {
	return logger.Panic()
}
