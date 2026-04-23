package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"etl-pipeline/internal/transformer"
)

// InitDB opens a connection pool and retries PingContext up to 5 times with a
// 2-second delay between attempts. This handles the window where the app
// container starts before the database is ready to accept connections.
func InitDB(ctx context.Context, dbURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("storage: open db: %w", err)
	}

	const maxAttempts = 5
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err = db.PingContext(ctx); err == nil {
			return db, nil
		}
		if attempt == maxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			db.Close()
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	db.Close()
	return nil, fmt.Errorf("storage: db not reachable after %d attempts: %w", maxAttempts, err)
}

// Migrate creates all required tables if they do not already exist.
func Migrate(ctx context.Context, db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS raw_weather_data (
			id         SERIAL PRIMARY KEY,
			payload    JSONB                    NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS processed_weather_data (
			id                  SERIAL PRIMARY KEY,
			latitude            DOUBLE PRECISION         NOT NULL,
			longitude           DOUBLE PRECISION         NOT NULL,
			temperature         DOUBLE PRECISION         NOT NULL,
			pressure            INTEGER                  NOT NULL,
			humidity            INTEGER                  NOT NULL,
			weather_description TEXT                     NOT NULL,
			observed_at         TIMESTAMP WITH TIME ZONE NOT NULL,
			created_at          TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, q := range queries {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("storage: migrate: %w", err)
		}
	}
	return nil
}

// SaveToPostgres inserts a raw JSON payload into the raw_weather_data table.
func SaveToPostgres(ctx context.Context, db *sql.DB, rawData []byte) error {
	const query = `INSERT INTO raw_weather_data (payload) VALUES ($1)`
	if _, err := db.ExecContext(ctx, query, rawData); err != nil {
		return fmt.Errorf("storage: insert raw data: %w", err)
	}
	return nil
}

// SaveProcessedToPostgres inserts a transformed weather record into the processed_weather_data table.
func SaveProcessedToPostgres(ctx context.Context, db *sql.DB, p *transformer.ProcessedWeather) error {
	t, err := time.Parse(time.RFC3339, p.TimestampISO)
	if err != nil {
		return fmt.Errorf("storage: parse timestamp: %w", err)
	}
	const query = `
		INSERT INTO processed_weather_data
			(latitude, longitude, temperature, pressure, humidity, weather_description, observed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	if _, err := db.ExecContext(ctx, query,
		p.Latitude, p.Longitude, p.Temperature,
		p.Pressure, p.Humidity, p.WeatherDescription, t,
	); err != nil {
		return fmt.Errorf("storage: insert processed data: %w", err)
	}
	return nil
}
