package logger

// Logger is the application-wide logging contract. All components depend
// on this interface, never on a concrete logging library.
//
// Methods accept structured key-value pairs in alternating order:
//
//	logger.Info("peer connected", "address", addr, "latency", dur)
type Logger interface {
	Debug(msg string, keysAndValues ...any)
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
	Fatal(msg string, keysAndValues ...any)

	// With returns a child logger that always includes the given
	// key-value pairs in every subsequent log entry.
	With(keysAndValues ...any) Logger
}

// Nop returns a Logger that discards all output. Useful in tests.
func Nop() Logger {
	return &nopLogger{}
}

type nopLogger struct{}

func (n *nopLogger) Debug(string, ...any) {}
func (n *nopLogger) Info(string, ...any)  {}
func (n *nopLogger) Warn(string, ...any)  {}
func (n *nopLogger) Error(string, ...any) {}
func (n *nopLogger) Fatal(string, ...any) {}
func (n *nopLogger) With(...any) Logger   { return n }
