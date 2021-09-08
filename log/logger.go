package log

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)


// Logger instance
var Logger *zap.Logger

// InitLog init log setting
func InitLog(debug bool) {
	//we currently only log into stdout/stderr
	newGenericLogger(debug)

	Logger.Info("logger construction succeeded")
}

func newGenericLogger(debug bool) {
	var err error
	var cfg zap.Config

	// create config
	if debug {
		cfg = zap.NewDevelopmentConfig()
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
		cfg.Development = true
		cfg.OutputPaths = []string{"stdout"}
		cfg.ErrorOutputPaths = []string{"stderr"}
		encoderCfg := zap.NewProductionEncoderConfig()
		encoderCfg.TimeKey = "ts"
		encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		cfg.EncoderConfig = encoderCfg
	} else {
		cfg = zap.NewProductionConfig()
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
		cfg.Development = false
		cfg.OutputPaths = []string{"stdout"}
		cfg.ErrorOutputPaths = []string{"stderr"}
		encoderCfg := zap.NewProductionEncoderConfig()
		encoderCfg.TimeKey = "ts"
		encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		cfg.EncoderConfig = encoderCfg
	}

	// init some defined fields to log
	cfg.InitialFields = map[string]interface{}{
		//TODO: Add useful field here.
	}

	// create logger
	Logger, err = cfg.Build()

	if err != nil {
		panic(err)
	}
}
