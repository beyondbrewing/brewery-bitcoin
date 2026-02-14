package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// zapLogger adapts zap.SugaredLogger to the Logger interface.
type zapLogger struct {
	sugar *zap.SugaredLogger
}

// Compile-time interface compliance check.
var _ Logger = (*zapLogger)(nil)

// NewProduction creates a production-grade Logger:
//   - JSON output to stderr
//   - Info level and above
//   - Caller information included
//   - Sampling enabled to avoid log storms
//   - Timestamps in ISO 8601 format
func NewProduction(opts ...Option) (Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder

	for _, opt := range opts {
		opt(&cfg)
	}

	z, err := cfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		return nil, err
	}
	return &zapLogger{sugar: z.Sugar()}, nil
}

// NewDevelopment creates a development-friendly Logger:
//   - Console (human-readable) output to stderr
//   - Debug level and above
//   - Caller information included
//   - Stacktrace on Warn and above
func NewDevelopment(opts ...Option) (Logger, error) {
	cfg := zap.NewDevelopmentConfig()
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	cfg.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder

	for _, opt := range opts {
		opt(&cfg)
	}

	z, err := cfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		return nil, err
	}
	return &zapLogger{sugar: z.Sugar()}, nil
}

// MustProduction is like NewProduction but panics on error.
func MustProduction(opts ...Option) Logger {
	l, err := NewProduction(opts...)
	if err != nil {
		panic("logger: failed to create production logger: " + err.Error())
	}
	return l
}

// MustDevelopment is like NewDevelopment but panics on error.
func MustDevelopment(opts ...Option) Logger {
	l, err := NewDevelopment(opts...)
	if err != nil {
		panic("logger: failed to create development logger: " + err.Error())
	}
	return l
}

func (l *zapLogger) Debug(msg string, keysAndValues ...any) {
	l.sugar.Debugw(msg, keysAndValues...)
}

func (l *zapLogger) Info(msg string, keysAndValues ...any) {
	l.sugar.Infow(msg, keysAndValues...)
}

func (l *zapLogger) Warn(msg string, keysAndValues ...any) {
	l.sugar.Warnw(msg, keysAndValues...)
}

func (l *zapLogger) Error(msg string, keysAndValues ...any) {
	l.sugar.Errorw(msg, keysAndValues...)
}

func (l *zapLogger) Fatal(msg string, keysAndValues ...any) {
	l.sugar.Fatalw(msg, keysAndValues...)
}

func (l *zapLogger) With(keysAndValues ...any) Logger {
	return &zapLogger{sugar: l.sugar.With(keysAndValues...)}
}

// Sync flushes any buffered log entries. Applications should call this
// before exiting. This is intentionally NOT on the Logger interface
// because it is an implementation detail of zap.
func (l *zapLogger) Sync() error {
	return l.sugar.Sync()
}
