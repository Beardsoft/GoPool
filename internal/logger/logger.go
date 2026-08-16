package logger

import "go.uber.org/zap"

var Logger = zap.Must(zap.NewProduction())

func Sync() { _ = Logger.Sync() }
