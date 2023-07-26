package kafka

import "github.com/rs/zerolog"

type kafkaLogger struct {
	logger *zerolog.Logger
	prefix string
}

func newKafkaLogger(logger *zerolog.Logger) *kafkaLogger {
	return &kafkaLogger{
		logger: logger,
		prefix: "kafka: ",
	}
}

func (k kafkaLogger) Printf(format string, v ...interface{}) {
	k.logger.Printf(k.prefix+format, v...)
}
