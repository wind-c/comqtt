// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package zap

import (
	"github.com/wind-c/comqtt/v2/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
	"time"
)

const (
	console = iota
	json
)

var logger *zap.Logger
var cfg config.Log

func Init(c config.Log) *zap.Logger {
	cfg = c
	level := zapcore.Level(cfg.Level)
	if !cfg.Enable {
		level = zapcore.Level(7)
	}
	encoder := getEncoder(cfg.Format)

	infoLevel := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl < zapcore.WarnLevel && lvl >= level
	})

	warnLevel := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.WarnLevel && lvl >= level
	})

	infoWriter := getLogWriter(cfg.InfoFile)
	errorWriter := getLogWriter(cfg.ErrorFile)

	core := zapcore.NewTee(
		zapcore.NewCore(encoder, infoWriter, infoLevel),
		zapcore.NewCore(encoder, errorWriter, warnLevel),
	)

	ops := make([]zap.Option, 0)
	ops = append(ops, zap.AddCaller()) //zap.AddCaller() 才会显示打日志点的文件名和行数
	ops = append(ops, zap.AddCallerSkip(1))
	if cfg.Env == 0 {
		ops = append(ops, zap.Development())
		ops = append(ops, zap.AddStacktrace(zapcore.WarnLevel)) //warn以上输出调用堆栈
	}
	if cfg.NodeName != "" {
		ops = append(ops, zap.Fields(zap.String("nn", cfg.NodeName)))
	}
	logger = zap.New(core, ops...)
	zap.ReplaceGlobals(logger)

	return logger
}

func getEncoder(tp int) zapcore.Encoder {
	conf := zapcore.EncoderConfig{
		MessageKey:   "m",
		LevelKey:     "l",
		TimeKey:      "t",
		CallerKey:    "f",
		EncodeLevel:  zapcore.CapitalLevelEncoder,
		EncodeCaller: zapcore.ShortCallerEncoder,
		EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(t.Format("2006-01-02 15:04:05"))
		},
		EncodeDuration: func(d time.Duration, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendInt64(int64(d) / 1000000)
		},
	}

	encoder := zapcore.NewConsoleEncoder(conf)
	if tp == json {
		encoder = zapcore.NewJSONEncoder(conf)
	}

	return encoder
}

func getLogWriter(filename string) zapcore.WriteSyncer {
	writeSyncer := zapcore.AddSync(os.Stdout)
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

		writeSyncer = zapcore.AddSync(lumberJackLogger)
	}

	return writeSyncer
}

func Sync() {
	zap.L().Sync()
}

func getLevel(level string) zapcore.Level {
	logLevel := zap.DebugLevel
	switch level {
	case "debug":
		logLevel = zap.DebugLevel
	case "info":
		logLevel = zap.InfoLevel
	case "warn":
		logLevel = zap.WarnLevel
	case "error":
		logLevel = zap.ErrorLevel
	case "panic":
		logLevel = zap.PanicLevel
	case "fatal":
		logLevel = zap.FatalLevel
	default:
		logLevel = zap.InfoLevel
	}
	return logLevel
}

// Field isolate the reference
type Field = zap.Field

func Debug(msg string, fields ...Field) {
	zap.L().Debug(msg, fields...)
}

func Info(msg string, fields ...Field) {
	zap.L().Info(msg, fields...)
}
func Infof(template string, args ...interface{}) {
	zap.S().Infof(template, args...)
}

func Warn(msg string, fields ...Field) {
	zap.L().Warn(msg, fields...)
}
func Warnf(template string, args ...interface{}) {
	zap.S().Warnf(template, args...)
}

func Error(err error, fields ...Field) {
	zap.L().Error(err.Error(), fields...)
}
func Errorf(template string, args ...interface{}) {
	zap.S().Errorf(template, args...)
}

func Panic(msg error, fields ...Field) {
	zap.L().Panic(msg.Error(), fields...)
}
func Panicf(template string, args ...interface{}) {
	zap.S().Panicf(template, args)
}
func DPanic(msg string, fields ...Field) {
	zap.L().DPanic(msg, fields...)
}

func Fatal(msg string, fields ...Field) {
	zap.L().Fatal(msg, fields...)
}
func Fatalf(template string, args ...interface{}) {
	zap.S().Fatalf(template, args...)
}

func String(key string, val string) Field {
	return zap.String(key, val)
}
func Strings(key string, val []string) Field {
	return zap.Strings(key, val)
}
func ByteString(key string, val []byte) Field {
	return zap.ByteString(key, val)
}
func Binary(key string, val []byte) Field {
	return zap.Binary(key, val)
}
func Int(key string, val int) Field {
	return zap.Int(key, val)
}
func Int8(key string, val int8) Field {
	return zap.Int8(key, val)
}
func Int16(key string, val int16) Field {
	return zap.Int16(key, val)
}
func Int32(key string, val int32) Field {
	return zap.Int32(key, val)
}
func Int64(key string, val int64) Field {
	return zap.Int64(key, val)
}
func Uint(key string, val uint) Field {
	return zap.Uint(key, val)
}
func Uint8(key string, val uint8) Field {
	return zap.Uint8(key, val)
}
func Uint16(key string, val uint16) Field {
	return zap.Uint16(key, val)
}
func Uint32(key string, val uint32) Field {
	return zap.Uint32(key, val)
}
func Uint64(key string, val uint64) Field {
	return zap.Uint64(key, val)
}
func Bool(key string, val bool) Field {
	return zap.Bool(key, val)
}
func Duration(key string, val time.Duration) Field {
	return zap.Duration(key, val)
}
func Time(key string, val time.Time) Field {
	return zap.Time(key, val)
}
func Float32(key string, val float32) Field {
	return zap.Float32(key, val)
}
func Float64(key string, val float64) Field {
	return zap.Float64(key, val)
}
func Uintptr(key string, val uintptr) Field {
	return zap.Uintptr(key, val)
}
func Any(key string, value interface{}) Field {
	return zap.Any(key, value)
}
