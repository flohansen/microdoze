package instrumentation

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewLogger_Level(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *LoggerConfig
		wantEnabled  slog.Level
		wantDisabled *slog.Level
	}{
		{
			name:         "nil config defaults to info",
			cfg:          nil,
			wantEnabled:  slog.LevelInfo,
			wantDisabled: new(slog.LevelDebug),
		},
		{
			name:         "debug",
			cfg:          &LoggerConfig{Level: LogLevelDebug, Writer: &bytes.Buffer{}},
			wantEnabled:  slog.LevelDebug,
			wantDisabled: nil,
		},
		{
			name:         "info",
			cfg:          &LoggerConfig{Level: LogLevelInfo, Writer: &bytes.Buffer{}},
			wantEnabled:  slog.LevelInfo,
			wantDisabled: new(slog.LevelDebug),
		},
		{
			name:         "warning",
			cfg:          &LoggerConfig{Level: LogLevelWarning, Writer: &bytes.Buffer{}},
			wantEnabled:  slog.LevelWarn,
			wantDisabled: new(slog.LevelInfo),
		},
		{
			name:         "error",
			cfg:          &LoggerConfig{Level: LogLevelError, Writer: &bytes.Buffer{}},
			wantEnabled:  slog.LevelError,
			wantDisabled: new(slog.LevelWarn),
		},
		{
			name:         "unknown level defaults to info",
			cfg:          &LoggerConfig{Level: LogLevel(42), Writer: &bytes.Buffer{}},
			wantEnabled:  slog.LevelInfo,
			wantDisabled: new(slog.LevelDebug),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// arrange
			ctx := t.Context()

			// act
			logger := NewLogger(tt.cfg)

			// assert
			assert.True(t, logger.Enabled(ctx, tt.wantEnabled))
			if tt.wantDisabled != nil {
				assert.False(t, logger.Enabled(ctx, *tt.wantDisabled))
			}
		})
	}
}

func TestNewLogger_Format(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *LoggerConfig
		wantJSON bool
	}{
		{
			name:     "zero value config defaults to json",
			cfg:      &LoggerConfig{},
			wantJSON: true,
		},
		{
			name:     "json",
			cfg:      &LoggerConfig{Format: LogFormatJSON},
			wantJSON: true,
		},
		{
			name:     "plain",
			cfg:      &LoggerConfig{Format: LogFormatPlain},
			wantJSON: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// arrange
			var buf bytes.Buffer
			tt.cfg.Writer = &buf
			logger := NewLogger(tt.cfg)

			// act
			logger.Info("hello", "key", "value")

			// assert
			assert.Equal(t, tt.wantJSON, json.Valid(buf.Bytes()))
			assert.Contains(t, buf.String(), "hello")
			assert.Contains(t, buf.String(), "value")
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name      string
		s         string
		wantLevel LogLevel
	}{
		{name: "debug", s: "debug", wantLevel: LogLevelDebug},
		{name: "info", s: "info", wantLevel: LogLevelInfo},
		{name: "warning", s: "warning", wantLevel: LogLevelWarning},
		{name: "error", s: "error", wantLevel: LogLevelError},
		{name: "empty defaults to info", s: "", wantLevel: LogLevelInfo},
		{name: "unknown defaults to info", s: "verbose", wantLevel: LogLevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// arrange
			// act
			level := ParseLogLevel(tt.s)

			// assert
			assert.Equal(t, tt.wantLevel, level)
		})
	}
}

func TestParseLogFormat(t *testing.T) {
	tests := []struct {
		name       string
		s          string
		wantFormat LogFormat
	}{
		{name: "json", s: "json", wantFormat: LogFormatJSON},
		{name: "plain", s: "plain", wantFormat: LogFormatPlain},
		{name: "empty defaults to json", s: "", wantFormat: LogFormatJSON},
		{name: "unknown defaults to json", s: "xml", wantFormat: LogFormatJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// arrange
			// act
			format := ParseLogFormat(tt.s)

			// assert
			assert.Equal(t, tt.wantFormat, format)
		})
	}
}
