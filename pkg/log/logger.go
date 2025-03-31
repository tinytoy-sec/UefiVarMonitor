package log

import (
	"log"
	"os"
)

type Logger interface {
	Warnf(format string, args ...interface{})

	Errorf(format string, args ...interface{})

	Fatalf(format string, args ...interface{})
}

var DefaultLogger Logger

func init() {
	DefaultLogger = logWrapper{Logger: log.New(os.Stderr, "", log.LstdFlags)}
}

type logWrapper struct {
	Logger *log.Logger
}

func (logger logWrapper) Warnf(format string, args ...interface{}) {
	logger.Logger.Printf("[WARN] "+format, args...)
}

func (logger logWrapper) Errorf(format string, args ...interface{}) {
	logger.Logger.Printf("[ERROR] "+format, args...)
}

func (logger logWrapper) Fatalf(format string, args ...interface{}) {
	logger.Logger.Fatalf("[FATAL] "+format, args...)
}

func Warnf(format string, args ...interface{}) {
	DefaultLogger.Warnf(format, args...)
}

func Errorf(format string, args ...interface{}) {
	DefaultLogger.Errorf(format, args...)
}

func Fatalf(format string, args ...interface{}) {
	DefaultLogger.Fatalf(format, args...)
}
