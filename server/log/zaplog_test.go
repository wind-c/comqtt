package log

import (
	"errors"
	"github.com/wind-c/comqtt/server/config"
	"go.uber.org/zap"
	"os"
	"testing"
)

func Test_setupLogger(t *testing.T) {
	logger := Init(config.Log{
		Enable: true,
		Env: 1,
		Level: 1,
		InfoFile: "./logs/co-info.log",
		ErrorFile: "./logs/co-error.log",
	})
	logger.Info("in main args:", zap.String("args", "os.Args"))
	logger.Sugar().Infof("in main args:%v", os.Args)
	logger.Error("error ", zap.Error(errors.New("test")))
	logger.Sugar().Errorf("error %v", "error")
	logger.Warn("warn ", zap.String("kk","warn 123"))
	logger.Sugar().Warnf("warn %v", "warn 123")
	logger.Sugar().Infof("env is %v", 123)
	logger.Sugar().Infof("ip=%v, port=%v, env=%v", "127.0.0.1", 8080, "prod")
}
