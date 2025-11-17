package logging

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithContext(t *testing.T) {
	t.Run("should store logger as context value", func(t *testing.T) {
		// arrange
		ctx := t.Context()
		logger := &slogLogger{}

		// act
		newCtx := WithContext(ctx, logger)

		// assert
		assert.Equal(t, logger, newCtx.Value(ctxKey("logger")))
	})
}

func TestFromContext(t *testing.T) {
	t.Run("should return logger stored as context value", func(t *testing.T) {
		// arrange
		expectedLogger := &slogLogger{}
		ctx := context.WithValue(t.Context(), ctxKey("logger"), expectedLogger)

		// act
		logger := FromContext(ctx)

		// assert
		assert.Same(t, expectedLogger, logger)
	})

	t.Run("should return new logger if none is set as context value", func(t *testing.T) {
		// arrange
		ctx := t.Context()

		// act
		logger := FromContext(ctx)

		// assert
		require.NotNil(t, logger)
		assert.Equal(t, &slogLogger{log: slog.New(slog.NewJSONHandler(os.Stdout, nil))}, logger)
	})
}
