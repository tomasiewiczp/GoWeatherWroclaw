package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"etl-pipeline/internal/transformer"
)

func AppendRaw(dir string, data []byte) error {
	if !json.Valid(data) {
		return fmt.Errorf("storage: raw data is not valid JSON")
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err != nil {
		return fmt.Errorf("storage: compact JSON: %w", err)
	}
	return appendJSONLine(dir, "raw_data.json", buf.Bytes())
}

func AppendProcessed(dir string, data *transformer.ProcessedWeather) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("storage: marshal processed weather: %w", err)
	}
	return appendJSONLine(dir, "processed_data.json", b)
}

func appendJSONLine(dir, filename string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("storage: create directory %q: %w", dir, err)
	}

	path := filepath.Join(dir, filename)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("storage: open %q: %w", path, err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("storage: write to %q: %w", path, err)
	}
	if _, err := f.WriteString("\n"); err != nil {
		return fmt.Errorf("storage: write newline to %q: %w", path, err)
	}

	return nil
}
