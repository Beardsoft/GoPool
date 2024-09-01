package logger

import (
	"go.uber.org/zap"
)

var (
	Logger *zap.Logger
)

func InitLogger() {
	var err error
	Logger, err = zap.NewProduction() // or zap.NewDevelopment() for development
	if err != nil {
		panic(err)
	}
}

func Sync() {
	Logger.Sync() // flushes buffer, if any
}
