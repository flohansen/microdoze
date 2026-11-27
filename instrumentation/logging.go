package instrumentation

import (
	"io"
	"log/slog"
	"os"
)

type LoggerConfig struct {
	Level  LogLevel
	Format LogFormat
	Writer io.Writer
}

type LogLevel int

const (
	LogLevelInfo LogLevel = iota
	LogLevelDebug
	LogLevelWarning
	LogLevelError
)

// ParseLogLevel parses a log level name. Unknown values fall back to
// LogLevelInfo.
func ParseLogLevel(s string) LogLevel {
	switch s {
	case "debug":
		return LogLevelDebug
	case "warning":
		return LogLevelWarning
	case "error":
		return LogLevelError
	default:
		return LogLevelInfo
	}
}

type LogFormat int

const (
	LogFormatJSON LogFormat = iota
	LogFormatPlain
)

// ParseLogFormat parses a log format name. Unknown values fall back to
// LogFormatJSON.
func ParseLogFormat(s string) LogFormat {
	switch s {
	case "plain":
		return LogFormatPlain
	default:
		return LogFormatJSON
	}
}

// NewLogger creates a logger from the given config. A nil config falls back to
// the zero value (info level, JSON format) and a nil writer to stdout.
func NewLogger(cfg *LoggerConfig) *slog.Logger {
	if cfg == nil {
		cfg = &LoggerConfig{}
	}

	w := cfg.Writer
	if w == nil {
		w = os.Stdout
	}

	var level slog.Level
	switch cfg.Level {
	case LogLevelDebug:
		level = slog.LevelDebug
	case LogLevelWarning:
		level = slog.LevelWarn
	case LogLevelError:
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	switch cfg.Format {
	case LogFormatPlain:
		handler = slog.NewTextHandler(w, opts)
	default:
		handler = slog.NewJSONHandler(w, opts)
	}

	return slog.New(handler)
}
