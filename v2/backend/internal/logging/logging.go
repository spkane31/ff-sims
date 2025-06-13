package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Logger is our custom logger built on top of slog
type Logger struct {
	slogger *slog.Logger
}

// LogLevel represents the severity of a log entry
type LogLevel string

// Log levels
const (
	DebugLevel LogLevel = "DEBUG"
	InfoLevel  LogLevel = "INFO"
	WarnLevel  LogLevel = "WARN"
	ErrorLevel LogLevel = "ERROR"
	FatalLevel LogLevel = "FATAL"
)

// LoggerOption is a function that configures a Logger
type LoggerOption func(*Logger)

// CustomHandler implements slog.Handler with our custom format
type CustomHandler struct {
	level  slog.Level
	writer io.Writer
	attrs  []slog.Attr
	groups []string
}

// Enabled checks if logging is enabled for the given level
func (h *CustomHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle formats and writes a log record
func (h *CustomHandler) Handle(_ context.Context, record slog.Record) error {
	// Get caller information
	var file string
	var line int

	// Skip frames to get to the actual caller
	for i := 0; i < 10; i++ {
		_, f, l, ok := runtime.Caller(i)
		if !ok {
			break
		}

		// Skip frames from the logging package and slog
		if filepath.Base(filepath.Dir(f)) != "logging" && filepath.Base(filepath.Dir(f)) != "slog" {
			file = filepath.Base(f)
			line = l
			break
		}
	}

	// Convert time to EST
	est, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(fmt.Sprintf("Failed to load EST location: %v", err))
	}
	timeEST := record.Time.In(est).Format("2006-01-02 15:04:05.000 -0500")

	// Format the log message
	levelStr := record.Level.String()
	msg := record.Message

	// Build the log line
	logLine := fmt.Sprintf("%s [%s] %s:%d: %s\n",
		timeEST,
		levelStr,
		file,
		line,
		msg)

	// Write to output
	_, err = h.writer.Write([]byte(logLine))
	return err
}

// WithAttrs returns a new handler with the given attributes
func (h *CustomHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandler := *h
	newHandler.attrs = append(h.attrs, attrs...)
	return &newHandler
}

// WithGroup returns a new handler with the given group
func (h *CustomHandler) WithGroup(name string) slog.Handler {
	newHandler := *h
	newHandler.groups = append(h.groups, name)
	return &newHandler
}

// WithOutput sets the output writer for the logger
func WithOutput(w io.Writer) LoggerOption {
	return func(l *Logger) {
		handler := &CustomHandler{
			level:  slog.LevelDebug,
			writer: w,
		}
		l.slogger = slog.New(handler)
	}
}

// WithLevel sets the minimum log level
func WithLevel(level LogLevel) LoggerOption {
	return func(l *Logger) {
		var slogLevel slog.Level
		switch level {
		case DebugLevel:
			slogLevel = slog.LevelDebug
		case InfoLevel:
			slogLevel = slog.LevelInfo
		case WarnLevel:
			slogLevel = slog.LevelWarn
		case ErrorLevel:
			slogLevel = slog.LevelError
		default:
			slogLevel = slog.LevelInfo
		}

		h := l.slogger.Handler().(*CustomHandler)
		h.level = slogLevel
		l.slogger = slog.New(h)
	}
}

//----------------------------------------------------------------------------
// Logger methods (standard and formatted)
//----------------------------------------------------------------------------

// Debug logs a debug message
func (l *Logger) Debug(msg string, args ...any) {
	l.slogger.Debug(msg, args...)
}

// Debugf logs a formatted debug message
func (l *Logger) Debugf(format string, args ...any) {
	l.slogger.Debug(fmt.Sprintf(format, args...))
}

// Info logs an info message
func (l *Logger) Info(msg string, args ...any) {
	l.slogger.Info(msg, args...)
}

// Infof logs a formatted info message
func (l *Logger) Infof(format string, args ...any) {
	l.slogger.Info(fmt.Sprintf(format, args...))
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, args ...any) {
	l.slogger.Warn(msg, args...)
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(format string, args ...any) {
	l.slogger.Warn(fmt.Sprintf(format, args...))
}

// Error logs an error message
func (l *Logger) Error(msg string, args ...any) {
	l.slogger.Error(msg, args...)
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(format string, args ...any) {
	l.slogger.Error(fmt.Sprintf(format, args...))
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(msg string, args ...any) {
	l.slogger.Error(msg, args...)
	os.Exit(1)
}

// Fatalf logs a formatted fatal message and exits
func (l *Logger) Fatalf(format string, args ...any) {
	l.slogger.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

// With returns a Logger with the given attributes
func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		slogger: l.slogger.With(args...),
	}
}

// NewLogger creates a new Logger with the given options
func NewLogger(options ...LoggerOption) *Logger {
	// Create default logger to stdout
	logger := &Logger{}

	// Apply default options
	WithOutput(os.Stdout)(logger)

	// Apply custom options
	for _, option := range options {
		option(logger)
	}

	return logger
}

//----------------------------------------------------------------------------
// Global singleton logger and package-level functions
//----------------------------------------------------------------------------

var (
	defaultLogger *Logger
	once          sync.Once
)

// Initialize the default logger
func init() {
	defaultLogger = NewLogger(
		WithOutput(os.Stdout),
		WithLevel(InfoLevel),
	)
}

// Configure sets up the default logger with the given options
func Configure(options ...LoggerOption) {
	once.Do(func() {
		for _, option := range options {
			option(defaultLogger)
		}
	})
}

// GetDefaultLogger returns the default logger instance
func GetDefaultLogger() *Logger {
	return defaultLogger
}

// SetDefaultLogger sets the default logger
func SetDefaultLogger(logger *Logger) {
	defaultLogger = logger
}

//----------------------------------------------------------------------------
// Package-level functions (standard and formatted)
//----------------------------------------------------------------------------

// Debug logs a debug message using the default logger
func Debug(msg string, args ...any) {
	defaultLogger.Debug(msg, args...)
}

// Debugf logs a formatted debug message using the default logger
func Debugf(format string, args ...any) {
	defaultLogger.Debugf(format, args...)
}

// Info logs an info message using the default logger
func Info(msg string, args ...any) {
	defaultLogger.Info(msg, args...)
}

// Infof logs a formatted info message using the default logger
func Infof(format string, args ...any) {
	defaultLogger.Infof(format, args...)
}

// Warn logs a warning message using the default logger
func Warn(msg string, args ...any) {
	defaultLogger.Warn(msg, args...)
}

// Warnf logs a formatted warning message using the default logger
func Warnf(format string, args ...any) {
	defaultLogger.Warnf(format, args...)
}

// Error logs an error message using the default logger
func Error(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
}

// Errorf logs a formatted error message using the default logger
func Errorf(format string, args ...any) {
	defaultLogger.Errorf(format, args...)
}

// Fatal logs a fatal message and exits using the default logger
func Fatal(msg string, args ...any) {
	defaultLogger.Fatal(msg, args...)
}

// Fatalf logs a formatted fatal message and exits using the default logger
func Fatalf(format string, args ...any) {
	defaultLogger.Fatalf(format, args...)
}

// With returns a Logger with the given attributes using the default logger
func With(args ...any) *Logger {
	return defaultLogger.With(args...)
}
