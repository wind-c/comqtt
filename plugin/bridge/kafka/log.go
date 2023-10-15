package kafka

import (
	"fmt"
	"log/slog"
)

type kafkaLogger struct {
	logger *slog.Logger
	prefix string
}

func newKafkaLogger(logger *slog.Logger) *kafkaLogger {
	return &kafkaLogger{
		logger: logger,
		prefix: "kafka: ",
	}
}

func (k kafkaLogger) Printf(format string, v ...interface{}) {
	k.logger.Info(fmt.Sprintf(k.prefix+format, v...))
}
