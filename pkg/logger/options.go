package logger

import "go.uber.org/zap"

// Option configures the underlying zap.Config before the logger is built.
type Option func(*zap.Config)

// WithLevel overrides the minimum log level.
// Valid values: "debug", "info", "warn", "error", "fatal".
func WithLevel(level string) Option {
	return func(cfg *zap.Config) {
		var l zap.AtomicLevel
		if err := l.UnmarshalText([]byte(level)); err == nil {
			cfg.Level = l
		}
	}
}

// WithOutputPaths overrides where log output is written.
// Defaults: ["stderr"] for both production and development.
func WithOutputPaths(paths ...string) Option {
	return func(cfg *zap.Config) {
		cfg.OutputPaths = paths
	}
}

// WithDisableCaller turns off caller annotation in log entries.
func WithDisableCaller() Option {
	return func(cfg *zap.Config) {
		cfg.DisableCaller = true
	}
}

// WithDisableStacktrace turns off automatic stacktraces.
func WithDisableStacktrace() Option {
	return func(cfg *zap.Config) {
		cfg.DisableStacktrace = true
	}
}
