// Package logging implements a structured logger with context propagation,
// prefix mapping, and thread safety. It is designed to support detailed campaign
// and execution level logging for both CLI consoles and verification diagnostics.
//
// Dependency Rules:
// - This package is a leaf package. It must not import other framework packages.
package logging

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// LogLevel defines logging severity.
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger represents a thread-safe structured logging instance.
type Logger struct {
	mu     sync.Mutex
	out    io.Writer
	level  LogLevel
	fields map[string]interface{}
}

// NewLogger creates a new Logger writing to out with the given min level.
func NewLogger(out io.Writer, minLevel LogLevel) *Logger {
	if out == nil {
		out = os.Stdout
	}
	return &Logger{
		out:    out,
		level:  minLevel,
		fields: make(map[string]interface{}),
	}
}

// WithFields returns a copy of the logger with the specified fields added.
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newFields := make(map[string]interface{}, len(l.fields)+len(fields))
	for k, v := range l.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	return &Logger{
		out:    l.out,
		level:  l.level,
		fields: newFields,
	}
}

type ctxKey string

const contextLoggerKey ctxKey = "atf_logger_context"

// WithContext returns a logger derived from the context, if present.
func WithContext(ctx context.Context, fallback *Logger) *Logger {
	if val := ctx.Value(contextLoggerKey); val != nil {
		if l, ok := val.(*Logger); ok {
			return l
		}
	}
	return fallback
}

// Inject returns a new context containing this logger.
func (l *Logger) Inject(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextLoggerKey, l)
}

// Log writes a log entry with the given level and format arguments.
func (l *Logger) Log(lvl LogLevel, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if lvl < l.level {
		return
	}

	ts := time.Now().Format("2006-01-02T15:04:05.000Z07:00")
	msg := fmt.Sprintf(format, args...)

	fieldsStr := ""
	if len(l.fields) > 0 {
		fieldsStr = " |"
		for k, v := range l.fields {
			fieldsStr += fmt.Sprintf(" %s=%v", k, v)
		}
	}

	fmt.Fprintf(l.out, "%s [%s] %s%s\n", ts, lvl.String(), msg, fieldsStr)
}

// Debug logs a debug-level message.
func (l *Logger) Debug(format string, args ...interface{}) {
	l.Log(LevelDebug, format, args...)
}

// Info logs an info-level message.
func (l *Logger) Info(format string, args ...interface{}) {
	l.Log(LevelInfo, format, args...)
}

// Warn logs a warning-level message.
func (l *Logger) Warn(format string, args ...interface{}) {
	l.Log(LevelWarn, format, args...)
}

// Error logs an error-level message.
func (l *Logger) Error(format string, args ...interface{}) {
	l.Log(LevelError, format, args...)
}

// SetLevel updates the minimum logging severity dynamically.
func (l *Logger) SetLevel(lvl LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = lvl
}
