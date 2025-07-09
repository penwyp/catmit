package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates a new zap logger configured for console output
// with line numbers and no timestamps.
func New(debug bool) (*zap.Logger, error) {
	level := zapcore.InfoLevel
	if debug {
		level = zapcore.DebugLevel
	}

	encoderConfig := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		CallerKey:      "caller",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder, // Colored level names
		EncodeCaller:   zapcore.ShortCallerEncoder,       // Show file:line
		// TimeKey is omitted to remove timestamps
	}

	config := zap.Config{
		Level:            zap.NewAtomicLevelAt(level),
		Development:      debug,
		Encoding:         "console", // Console format instead of JSON
		EncoderConfig:    encoderConfig,
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
		DisableCaller:    false, // Enable caller info for line numbers
		DisableStacktrace: !debug, // Only show stack traces in debug mode
	}

	logger, err := config.Build()
	if err != nil {
		return nil, err
	}

	return logger, nil
}