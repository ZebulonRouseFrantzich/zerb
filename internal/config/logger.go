package config

// Logger provides structured logging for config operations.
// This interface allows users to plug in their own logging implementation.
type Logger interface {
	// Debug logs debug-level messages with optional key-value pairs.
	Debug(msg string, keysAndValues ...interface{})

	// Info logs info-level messages with optional key-value pairs.
	Info(msg string, keysAndValues ...interface{})

	// Warn logs warning-level messages with optional key-value pairs.
	Warn(msg string, keysAndValues ...interface{})

	// Error logs error-level messages with optional key-value pairs.
	Error(msg string, keysAndValues ...interface{})
}

// noopLogger is a Logger implementation that does nothing.
// This is the default logger used when none is provided.
type noopLogger struct{}

func (n *noopLogger) Debug(msg string, keysAndValues ...interface{}) {}
func (n *noopLogger) Info(msg string, keysAndValues ...interface{})  {}
func (n *noopLogger) Warn(msg string, keysAndValues ...interface{})  {}
func (n *noopLogger) Error(msg string, keysAndValues ...interface{}) {}

// defaultLogger returns the default no-op logger.
func defaultLogger() Logger {
	return &noopLogger{}
}
