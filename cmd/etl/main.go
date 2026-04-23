package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"etl-pipeline/internal/config"
	"etl-pipeline/internal/fetcher"
	"etl-pipeline/internal/observability"
	"etl-pipeline/internal/storage"
	"etl-pipeline/internal/transformer"
)

func main() {
	cfg, err := config.LoadConfig(".env")
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	logger, err := observability.NewLogger("logs/etl.log")
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	logger.Info("ETL pipeline starting",
		slog.Int("interval_seconds", cfg.FetchIntervalSeconds),
		slog.String("lat", cfg.Latitude),
		slog.String("lon", cfg.Longitude),
		slog.String("metrics_port", cfg.MetricsPort),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := storage.InitDB(ctx, cfg.DBConnectionString)
	if err != nil {
		logger.Error("failed to connect to database", slog.Any("error", err))
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("database connection established")

	if err := storage.Migrate(ctx, db); err != nil {
		logger.Error("failed to run database migration", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("database migration complete")

	metricsErr := make(chan error, 1)
	go func() {
		metricsErr <- observability.StartMetricsServer(cfg.MetricsPort, logger.Logger)
	}()

	ticker := time.NewTicker(time.Duration(cfg.FetchIntervalSeconds) * time.Second)
	defer ticker.Stop()

	executeCycle(ctx, cfg, logger.Logger, db)

	for {
		select {
		case <-ticker.C:
			executeCycle(ctx, cfg, logger.Logger, db)
		case err := <-metricsErr:
			logger.Error("metrics server died", slog.Any("error", err))
			return
		case <-ctx.Done():
			logger.Info("Shutting down gracefully...")
			return
		}
	}
}

func executeCycle(ctx context.Context, cfg *config.Config, logger *slog.Logger, db *sql.DB) {
	rawData, err := fetcher.Fetch(ctx, cfg)
	if err != nil {
		logger.Error("failed to fetch data", slog.Any("error", err))
		observability.ApiRequestsTotal.WithLabelValues("failure").Inc()
		return
	}
	observability.ApiRequestsTotal.WithLabelValues("success").Inc()
	logger.Info("data fetched successfully", slog.Int("bytes", len(rawData)))

	if err := storage.SaveToPostgres(ctx, db, rawData); err != nil {
		logger.Error("failed to save raw data to postgres", slog.Any("error", err))
		observability.DBWritesTotal.WithLabelValues("failure", "raw").Inc()
		return
	}
	observability.DBWritesTotal.WithLabelValues("success", "raw").Inc()

	if err := storage.AppendRaw("data/raw", rawData); err != nil {
		logger.Error("failed to append raw data", slog.Any("error", err))
		return
	}
	observability.DataSavedTotal.WithLabelValues("raw").Inc()
	logger.Info("raw data appended to local lake")

	processedData, err := transformer.Transform(rawData)
	if err != nil {
		logger.Error("failed to transform data", slog.Any("error", err))
		observability.TransformErrorsTotal.Inc()
		return
	}

	if err := storage.SaveProcessedToPostgres(ctx, db, processedData); err != nil {
		logger.Error("failed to save processed data to postgres", slog.Any("error", err))
		observability.DBWritesTotal.WithLabelValues("failure", "processed").Inc()
		return
	}
	observability.DBWritesTotal.WithLabelValues("success", "processed").Inc()

	if err := storage.AppendProcessed("data/processed", processedData); err != nil {
		logger.Error("failed to append processed data", slog.Any("error", err))
		return
	}
	observability.DataSavedTotal.WithLabelValues("processed").Inc()
	observability.LastSuccessfulRunTimestamp.SetToCurrentTime()
	logger.Info("processed data appended to local lake",
		slog.Float64("temperature", processedData.Temperature),
		slog.Int("pressure", processedData.Pressure),
		slog.Int("humidity", processedData.Humidity),
		slog.String("description", processedData.WeatherDescription),
		slog.String("timestamp", processedData.TimestampISO),
	)
}
