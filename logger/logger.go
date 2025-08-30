package logger

import (
	"log/slog"
	"os"
	"strings"
)

var Logger *slog.Logger

func init() {
	// Get log level from environment
	logLevel := strings.ToLower(os.Getenv("LOG_LEVEL"))
	
	var level slog.Level
	switch logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// Create text handler for human-readable logs
	opts := &slog.HandlerOptions{
		Level: level,
	}
	
	handler := slog.NewTextHandler(os.Stdout, opts)
	Logger = slog.New(handler)
	
	// Set as default logger
	slog.SetDefault(Logger)
}

// Convenience functions for structured logging
func Debug(msg string, args ...any) {
	Logger.Debug(msg, args...)
}

func Info(msg string, args ...any) {
	Logger.Info(msg, args...)
}

func Warn(msg string, args ...any) {
	Logger.Warn(msg, args...)
}

func Error(msg string, args ...any) {
	Logger.Error(msg, args...)
}

// WithContext creates a new logger with additional context
func WithContext(args ...any) *slog.Logger {
	return Logger.With(args...)
}