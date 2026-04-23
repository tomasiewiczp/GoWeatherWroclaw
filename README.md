# OpenWeather ETL Pipeline

A background service written in Go that fetches current weather conditions from the [OpenWeather API](https://openweathermap.org/current) on a configurable interval, stores both the raw API response and a normalised record in PostgreSQL, and exposes operational metrics via Prometheus.

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Setup & Running](#2-setup--running)
3. [Data Lake Structure](#3-data-lake-structure)
4. [Observability](#4-observability)
5. [Productionisation & Cloud Architecture](#5-productionisation--cloud-architecture)
6. [Scalability & Reliability](#6-scalability--reliability)

---

## 1. Project Overview

The pipeline runs as a daemon and executes an ETL cycle on a configurable interval (default: 30 s):

| Phase | What happens |
|---|---|
| **Extract** | HTTP GET to OpenWeather Current Weather Data API 2.5 for a given lat/lon |
| **Store Raw** | Saves the full JSON payload to `raw_weather_data` table in PostgreSQL and appends to `data/raw/raw_data.json` (JSONL) |
| **Transform** | Parses only the required fields; converts Unix timestamp → ISO 8601 UTC |
| **Store Processed** | Saves the normalised record to `processed_weather_data` table in PostgreSQL and appends to `data/processed/processed_data.json` (JSONL) |
| **Observability** | HTTP server exposes `/health` and `/metrics` (Prometheus) |

```
internal/
├── config/         # Environment-based configuration
├── fetcher/        # HTTP client for OpenWeather API
├── transformer/    # Payload parsing & field extraction
├── storage/        # JSONL file writers + mock DB
└── observability/  # slog JSON logger + Prometheus metrics + HTTP server
cmd/etl/main.go     # Entry point, ticker loop, graceful shutdown
```

Note on PostgreSQL: Both raw and processed data are persisted to PostgreSQL 15 alongside the local JSONL lake. The database is provisioned automatically via docker-compose — no manual setup required. On first startup the app creates both table schemas via `CREATE TABLE IF NOT EXISTS` migrations in `storage.Migrate`. The app container waits for `db` to pass `pg_isready` before starting, thanks to `depends_on: condition: service_healthy`.

**Database tables:**

| Table | Content | Key columns |
|---|---|---|
| `raw_weather_data` | Full JSON payload from OpenWeather API | `payload` (JSONB), `created_at` |
| `processed_weather_data` | Normalised, typed weather record | `latitude`, `longitude`, `temperature`, `pressure`, `humidity`, `weather_description`, `observed_at` |

---

## 2. Setup & Running

### Prerequisites

- Docker (recommended) **or** Go 1.22+
- An [OpenWeather Current Weather Data API 2.5](https://openweathermap.org/current) key (free tier)

### Configuration

Copy `.env.example` to `.env` and fill in your API key:

```bash
cp .env.example .env
```

> All variables can also be set as regular OS environment variables, which always take precedence over the `.env` file.

### Running with Docker Compose (recommended)

This is the primary way to run the project. A single command builds and starts both the Go ETL worker and a PostgreSQL 15 database:

```bash
docker-compose up -d --build
```

**What this does:**

| Container | Role | Persistence |
|---|---|---|
| `db` (postgres:15-alpine) | Stores raw payloads in `raw_weather_data` table | `./postgres_data` → `/var/lib/postgresql/data` |
| `app` (Go binary) | Runs the ETL loop, writes files, exposes metrics | `./data/raw`, `./data/processed`, `./logs` mounted as volumes |

The `app` container will not start until the `db` container passes its healthcheck (`pg_isready`), preventing connection errors on startup.

**View logs:**

```bash
docker-compose logs -f app
```

**Stop and clean up:**

```bash
docker-compose down
```

To also delete the Postgres data volume: `docker-compose down -v` (removes `postgres_data/`).

**Querying the database:**

Port `5432` is exposed on localhost, so you can connect directly:

```bash
psql postgres://etl:etlpass@localhost:5432/weather
```

Or without `psql` installed locally — exec into the container:

```bash
docker compose exec db psql -U etl -d weather
```

Example queries:

```sql
-- last  processed weather records
SELECT observed_at, temperature, humidity, weather_description
FROM processed_weather_data
ORDER BY observed_at DESC;

-- last  raw payloads (extract city name from JSON)
SELECT id, created_at, payload->>'name' AS city
FROM raw_weather_data
ORDER BY created_at DESC;

-- count records collected today
SELECT COUNT(*) FROM processed_weather_data
WHERE observed_at >= CURRENT_DATE;
```

### Running locally (without Docker)

Requires a running PostgreSQL instance. Set `DB_HOST=localhost` in `.env`.

```bash
go run ./cmd/etl
```

---

## 3. Data Lake Structure

```
data/
├── raw/
│   └── raw_data.json        # Full API payloads, one per line
└── processed/
    └── processed_data.json  # Normalised records, one per line
```

### Why append-only JSON Lines (JSONL)?

Each ETL cycle **appends** one line to the file rather than overwriting it. This pattern, known as JSON Lines (`.jsonl`), has several advantages:

- **Crash safety** – a partial write corrupts at most one line; all previous records remain readable.
- **Streaming-friendly** – tools like `jq`, `grep`, and cloud ingestion services (AWS Glue, BigQuery) can process the file incrementally without loading it entirely into memory.
- **Natural audit trail** – the file doubles as an immutable event log of every API response received.

### Schema of `processed_data.json`

Each line is a standalone JSON object:
(each line is a compact JSON object – formatted here for readability)
```json
{
  "latitude": 51.107883,
  "longitude": 17.038538,
  "temperature": 7.53,
  "pressure": 1021,
  "humidity": 82,
  "weather_description": "broken clouds",
  "timestamp_iso": "2024-03-15T14:00:00Z"
}
```

| Field | Type | Source | Notes |
|---|---|---|---|
| `latitude` | float64 | `coord.lat` | As returned by the API |
| `longitude` | float64 | `coord.lon` | As returned by the API |
| `temperature` | float64 | `main.temp` | °C (`units=metric` is set in the request) |
| `pressure` | int | `main.pressure` | hPa |
| `humidity` | int | `main.humidity` | % |
| `weather_description` | string | `weather[0].description` | Human-readable condition |
| `timestamp_iso` | string | `dt` | Unix → ISO 8601 UTC (`time.RFC3339`) |

---

## 4. Observability

Once the pipeline is running, two HTTP endpoints are available on `METRICS_PORT` (default `8080`):

### `/health`

Simple liveness probe. Returns `200 OK` as long as the process is up.

```bash
curl http://localhost:8080/health
# {"status":"up"}
```

### `/metrics`

Prometheus-compatible metrics scrape endpoint.

```bash
curl http://localhost:8080/metrics
```

#### Custom metrics

| Metric | Type | Labels | Description |
|---|---|---|---|
| `etl_api_requests_total` | Counter | `status="success"\|"failure"` | Every call to the OpenWeather API |
| `etl_transform_errors_total` | Counter | – | Payload parsing failures (malformed API response) |
| `etl_data_saved_total` | Counter | `type="raw"\|"processed"` | Successful file write operations |
| `etl_db_writes_total` | Counter | `status="success"\|"failure"`, `type="raw"\|"processed"` | Every Postgres write attempt |
| `etl_last_successful_run_timestamp_seconds` | Gauge | – | Unix timestamp of the last fully successful ETL cycle |

#### Why these metrics matter in production

- **`etl_api_requests_total{status="failure"}`** — a spike indicates API key expiry, network issues, or upstream rate-limiting (HTTP 429). Alert when the failure rate exceeds ~5 % over a 5-minute window.
- **`etl_transform_errors_total`** — any non-zero value in steady state is a signal that the upstream API changed its response schema. A critical alert for data-pipeline integrity.
- **`etl_data_saved_total{type="raw"}`** — confirms the Extract and Store stages completed. Divergence between `raw` and `processed` counters pinpoints where the pipeline is dropping records.

---

## 5. Productionisation & Cloud Architecture

The current implementation is a single-binary daemon suitable for a single machine. Moving it to a cloud environment would involve replacing local components with managed services:

```
┌─────────────────────────────────────────────────────────────────┐
│                         AWS                                     │
│                                                                 │
│  Scheduler          Worker              Storage                 │
│  ──────────         ──────────          ────────                │
│  EventBridge  ───►  ECS Fargate  ──────► S3          (raw lake) │
│  (cron 30s)         (container)   ├────► RDS Postgres (records) │
│                          │        └────► S3          (processed)|
│                          │                                      │
│                     CloudWatch  ◄──── /metrics scrape           │
│                     + Managed        or                         │
│                     Prometheus       AWS Managed Prometheus     │
└─────────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────────┐
│                      Google Cloud Platform                      │
│                                                                 │
│  Scheduler          Worker              Storage                 │
│  ──────────         ──────────          ────────                │
│ Cloud Scheduler ───► Cloud Run   ──────► GCS (raw)              │
│  (cron 30sec )      (container)   ├────► Cloud SQL (PostgreSQL) │
│                          │        └────► GCS (processed)        │
│                          │                                      │
│                    Cloud Logging ◄──── Managed Service for      │
│                                        Prometheus (GMP)         │
└─────────────────────────────────────────────────────────────────┘
```

| Current component | Cloud replacement | Rationale |
|---|---|---|
| OS ticker (30 s) | **AWS EventBridge Scheduler** / Cloud Scheduler (GCP) | Decouples scheduling from the worker; survives restarts |
| Binary daemon | **AWS ECS Fargate** / Cloud Run (GCP) | Serverless containers; no EC2 to manage |
| `data/raw/raw_data.json` | **Amazon S3** with date-partitioned prefix (`year=YYYY/month=MM/day=DD/`) | Unlimited retention, native integration with Glue/Athena |
| Self-hosted PostgreSQL (docker-compose) | **Amazon RDS (PostgreSQL)** / Cloud SQL | Managed, replicated, point-in-time recovery |
| `data/processed/` | **S3 + AWS Glue Data Catalog** | Queryable via Athena without ETL pre-processing |
| Local `logs/etl.log` | **CloudWatch Logs** / Cloud Logging | Centralised, searchable, retention policies |
| `/metrics` endpoint | **AWS Managed Prometheus + Grafana** | Dashboards, alerting, long-term storage |

---

## 6. Scalability & Reliability

### Horizontal scaling

The current worker is **stateless** — it does not hold any in-memory state between cycles. This makes horizontal scaling straightforward:

- Deploy multiple container instances, each targeting a **different geographic region** (different `LATITUDE`/`LONGITUDE`), partitioned by a shard key in S3 (`region=wroclaw/`, `region=warsaw/`, …).
- For higher-frequency data (e.g. 1-second intervals), introduce a **message queue** (SQS / Pub/Sub) between the scheduler and the workers to absorb burst load and decouple ingestion from storage writes.
- S3 handles unlimited concurrent writes without coordination. RDS write throughput scales vertically (instance class) or via **read replicas** for query offloading.

### Reliability

| Concern | Current behaviour | Production recommendation |
|---|---|---|
| API transient errors | Single attempt; error logged | Exponential back-off with jitter (3 retries, max 30 s delay) using `golang.org/x/time/rate` |
| API rate limiting (HTTP 429) | Error logged, cycle skipped | Back-off + alert on `etl_api_requests_total{status="failure"}` |
| File write failure | Error logged, cycle skipped | Dead-letter queue (SQS DLQ) to retry failed payloads |
| Process crash mid-cycle | Next cycle starts fresh (idempotent writes) | ECS auto-restart policy + CloudWatch alarm on task stop events |
| Graceful shutdown | `SIGINT`/`SIGTERM` → in-flight cycle completes, ticker stops | Same pattern works inside Fargate; ECS sends `SIGTERM` 30 s before `SIGKILL` |
| Schema change in API | `Transform` returns error; processed write skipped | Schema validation layer (e.g. JSON Schema) + alert on `etl_transform_errors_total > 0` |
| DB unavailable at startup | 5 retries × 2 s (ctx-cancellable), then fatal exit | Longer backoff with jitter; DB auto-failover via RDS Multi-AZ |
| DB write transient failure | Error logged, cycle continues (raw file still saved) | Retry with backoff; if persistent, alert on `etl_db_writes_total{status="failure"}` |
