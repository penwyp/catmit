package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger defines the logger interface
type Logger interface {
	Debug(msg string, fields ...zap.Field)
	Debugf(template string, args ...interface{})
	Info(msg string, fields ...zap.Field)
	Infof(template string, args ...interface{})
	Warn(msg string, fields ...zap.Field)
	Warnf(template string, args ...interface{})
	Error(msg string, fields ...zap.Field)
	Errorf(template string, args ...interface{})
}

// zapLogger wraps zap.Logger to implement our Logger interface
type zapLogger struct {
	*zap.Logger
	sugar *zap.SugaredLogger
}

// NewDefault creates a default logger
func NewDefault() Logger {
	logger, _ := New(false)
	return &zapLogger{
		Logger: logger,
		sugar:  logger.Sugar(),
	}
}

// Debug logs a debug message
func (l *zapLogger) Debugf(template string, args ...interface{}) {
	l.sugar.Debugf(template, args...)
}

// Info logs an info message
func (l *zapLogger) Infof(template string, args ...interface{}) {
	l.sugar.Infof(template, args...)
}

// Warn logs a warning message
func (l *zapLogger) Warnf(template string, args ...interface{}) {
	l.sugar.Warnf(template, args...)
}

// Error logs an error message
func (l *zapLogger) Errorf(template string, args ...interface{}) {
	l.sugar.Errorf(template, args...)
}

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