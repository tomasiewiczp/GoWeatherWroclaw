# ─── Stage 1: Build ───────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /build

# Download dependencies first (layer is cached as long as go.mod/go.sum don't change)
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o etl-app ./cmd/etl

# ─── Stage 2: Final image ─────────────────────────────────────────────────────
FROM alpine:3.19

# ca-certificates: required for TLS verification on HTTPS calls to OpenWeather API.
RUN apk add --no-cache ca-certificates

# Run as a non-root user — principle of least privilege.
RUN adduser -D -u 10001 etl

WORKDIR /app

RUN mkdir -p /app/data/raw /app/data/processed /app/logs && chown -R etl:etl /app

COPY --from=builder /build/etl-app .

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q --spider http://localhost:8080/health || exit 1

USER etl

CMD ["./etl-app"]
