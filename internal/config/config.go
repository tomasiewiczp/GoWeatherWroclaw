package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all runtime settings for the ETL pipeline.
// Values are resolved from .env first, then overridden by actual OS env vars.
type Config struct {
	OpenWeatherAPIKey    string
	Latitude             string
	Longitude            string
	FetchIntervalSeconds int
	MetricsPort          string

	DBHost             string
	DBPort             string
	DBUser             string
	DBPassword         string
	DBName             string
	DBConnectionString string
}

// LoadConfig reads configuration from an optional .env file and OS environment.
// OS environment variables always take precedence over .env file values.
// Returns an error if any required variable is missing or any value is invalid.
func LoadConfig(envFile string) (*Config, error) {
	_ = godotenv.Load(envFile)

	apiKey := os.Getenv("OPENWEATHER_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("config: OPENWEATHER_API_KEY is required but not set")
	}

	lat := getEnvOrDefault("LATITUDE", "51.107883")
	lon := getEnvOrDefault("LONGITUDE", "17.038538")
	metricsPort := getEnvOrDefault("METRICS_PORT", "8080")

	intervalStr := getEnvOrDefault("FETCH_INTERVAL_SECONDS", "30")
	interval, err := strconv.Atoi(intervalStr)
	if err != nil || interval <= 0 {
		return nil, fmt.Errorf("config: FETCH_INTERVAL_SECONDS must be a positive integer, got %q", intervalStr)
	}

	dbHost := getEnvOrDefault("DB_HOST", "db")
	dbPort := getEnvOrDefault("DB_PORT", "5432")
	dbUser := getEnvOrDefault("DB_USER", "etl")
	dbPassword := getEnvOrDefault("DB_PASSWORD", "etlpass")
	dbName := getEnvOrDefault("DB_NAME", "weather")

	return &Config{
		OpenWeatherAPIKey:    apiKey,
		Latitude:             lat,
		Longitude:            lon,
		FetchIntervalSeconds: interval,
		MetricsPort:          metricsPort,

		DBHost:     dbHost,
		DBPort:     dbPort,
		DBUser:     dbUser,
		DBPassword: dbPassword,
		DBName:     dbName,
		DBConnectionString: fmt.Sprintf(
			"postgres://%s:%s@%s:%s/%s?sslmode=disable",
			dbUser, dbPassword, dbHost, dbPort, dbName,
		),
	}, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
