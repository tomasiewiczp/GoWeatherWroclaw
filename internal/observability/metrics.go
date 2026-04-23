package observability

import (
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	ApiRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "etl_api_requests_total",
			Help: "Total number of OpenWeather API requests, labelled by status.",
		},
		[]string{"status"},
	)

	TransformErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "etl_transform_errors_total",
			Help: "Total number of payload transformation failures.",
		},
	)

	DataSavedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "etl_data_saved_total",
			Help: "Total number of successful data saves, labelled by type (raw or processed).",
		},
		[]string{"type"},
	)

	DBWritesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "etl_db_writes_total",
			Help: "Total number of Postgres write attempts, labelled by status and type.",
		},
		[]string{"status", "type"},
	)

	LastSuccessfulRunTimestamp = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "etl_last_successful_run_timestamp_seconds",
			Help: "Unix timestamp of the last fully successful ETL cycle.",
		},
	)
)

func StartMetricsServer(port string, logger *slog.Logger) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"up"}`))
	})

	logger.Info("metrics server listening", slog.String("port", port))
	return http.ListenAndServe(":"+port, mux)
}
