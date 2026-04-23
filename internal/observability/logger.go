package observability

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Logger bundles a *slog.Logger with the underlying file so the caller can
// close it during graceful shutdown without needing to know about the file.
type Logger struct {
	*slog.Logger
	file *os.File
}

// Close flushes and closes the underlying log file.
// Always call this (via defer) after NewLogger succeeds.
func (l *Logger) Close() error {
	_ = l.file.Sync()
	return l.file.Close()
}

// NewLogger creates a JSON-structured logger that appends to logs/etl.log.
// The parent directory is created automatically if it does not exist.
func NewLogger(logPath string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("observability: create log directory: %w", err)
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("observability: open log file %q: %w", logPath, err)
	}

	handler := slog.NewJSONHandler(io.MultiWriter(f, os.Stdout), &slog.HandlerOptions{
		Level: slog.LevelDebug,
		// AddSource adds file:line to every record — useful in production.
		AddSource: true,
	})

	return &Logger{
		Logger: slog.New(handler),
		file:   f,
	}, nil
}
