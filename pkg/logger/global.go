package logger

import "sync"

var (
	defaultLogger Logger = Nop()
	defaultMu     sync.RWMutex
)

// Default returns the global default Logger. If SetDefault has not been
// called, it returns a Nop logger.
func Default() Logger {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultLogger
}

// SetDefault replaces the global default Logger. It is safe for concurrent
// use but should typically be called once in main() during initialization.
func SetDefault(l Logger) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultLogger = l
}

// SyncDefault calls Sync on the default logger if it supports it.
// This is a convenience for main() to call in a defer statement.
func SyncDefault() {
	defaultMu.RLock()
	l := defaultLogger
	defaultMu.RUnlock()

	if zl, ok := l.(*zapLogger); ok {
		_ = zl.Sync()
	}
}

// Package-level convenience functions that delegate to Default().

func Debug(msg string, keysAndValues ...any) { Default().Debug(msg, keysAndValues...) }
func Info(msg string, keysAndValues ...any)  { Default().Info(msg, keysAndValues...) }
func Warn(msg string, keysAndValues ...any)  { Default().Warn(msg, keysAndValues...) }
func Error(msg string, keysAndValues ...any) { Default().Error(msg, keysAndValues...) }
func Fatal(msg string, keysAndValues ...any) { Default().Fatal(msg, keysAndValues...) }
func With(keysAndValues ...any) Logger       { return Default().With(keysAndValues...) }
