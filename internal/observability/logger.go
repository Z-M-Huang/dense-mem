package observability

import (
	"log/slog"
	"os"
)

// Logger is a structured logger wrapper that includes correlation and request context.
type Logger struct {
	logger *slog.Logger
}

// LogProvider is the companion interface for Logger.
// Consumers and tests depend on this abstraction rather than the concrete struct.
type LogProvider interface {
	Info(msg string, attrs ...LogAttr)
	Error(msg string, err error, attrs ...LogAttr)
	Warn(msg string, attrs ...LogAttr)
	Debug(msg string, attrs ...LogAttr)
	With(attrs ...LogAttr) LogProvider
}

// Ensure Logger implements LogProvider
var _ LogProvider = (*Logger)(nil)

// LogAttr represents a key-value pair for structured logging.
type LogAttr struct {
	Key   string
	Value interface{}
}

// String returns a string LogAttr.
func String(key, value string) LogAttr {
	return LogAttr{Key: key, Value: value}
}

// Int returns an int LogAttr.
func Int(key string, value int) LogAttr {
	return LogAttr{Key: key, Value: value}
}

// Bool returns a bool LogAttr.
func Bool(key string, value bool) LogAttr {
	return LogAttr{Key: key, Value: value}
}

// CorrelationID returns a correlation_id LogAttr.
func CorrelationID(value string) LogAttr {
	return LogAttr{Key: "correlation_id", Value: value}
}

// ClientIP returns a client_ip LogAttr.
func ClientIP(value string) LogAttr {
	return LogAttr{Key: "client_ip", Value: value}
}

// ProfileID returns a profile_id LogAttr.
func ProfileID(value string) LogAttr {
	return LogAttr{Key: "profile_id", Value: value}
}

// KeyID returns a key_id LogAttr.
func KeyID(value string) LogAttr {
	return LogAttr{Key: "key_id", Value: value}
}

// KeyPrefix returns a key_prefix LogAttr.
func KeyPrefix(value string) LogAttr {
	return LogAttr{Key: "key_prefix", Value: value}
}

// New creates a new Logger with the given log level.
func New(level slog.Level) *Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	return &Logger{
		logger: slog.New(handler),
	}
}

// NewWithHandler creates a new Logger with a custom handler.
func NewWithHandler(handler slog.Handler) *Logger {
	return &Logger{
		logger: slog.New(handler),
	}
}

// toSlogAttrs converts LogAttr slice to slog.Attr slice.
// This function filters out sensitive fields.
func toSlogAttrs(attrs []LogAttr) []any {
	result := make([]any, 0, len(attrs)*2)
	for _, attr := range attrs {
		// Never log sensitive fields
		if isSensitiveField(attr.Key) {
			continue
		}
		result = append(result, slog.Any(attr.Key, attr.Value))
	}
	return result
}

// isSensitiveField returns true if the field should never be logged.
func isSensitiveField(key string) bool {
	sensitive := map[string]bool{
		"key_hash":         true,
		"encrypted_secret": true,
		"api_key":          true,
		"raw_key":          true,
		"secret":           true,
		"password":         true,
		"token":            true,
		"vector":           true,
		"embedding":        true,
		"embeddings":       true,
	}
	return sensitive[key]
}

// Info logs an info message.
func (l *Logger) Info(msg string, attrs ...LogAttr) {
	l.logger.Info(msg, toSlogAttrs(attrs)...)
}

// Error logs an error message.
func (l *Logger) Error(msg string, err error, attrs ...LogAttr) {
	allAttrs := append([]LogAttr{{Key: "error", Value: err.Error()}}, attrs...)
	l.logger.Error(msg, toSlogAttrs(allAttrs)...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, attrs ...LogAttr) {
	l.logger.Warn(msg, toSlogAttrs(attrs)...)
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, attrs ...LogAttr) {
	l.logger.Debug(msg, toSlogAttrs(attrs)...)
}

// With returns a new LogProvider with the given attributes pre-set.
func (l *Logger) With(attrs ...LogAttr) LogProvider {
	return &Logger{
		logger: l.logger.With(toSlogAttrs(attrs)...),
	}
}
