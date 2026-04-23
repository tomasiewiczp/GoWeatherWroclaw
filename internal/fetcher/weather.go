package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"etl-pipeline/internal/config"
)

const weatherEndpoint = "https://api.openweathermap.org/data/2.5/weather"

var client = &http.Client{Timeout: 10 * time.Second}

func Fetch(ctx context.Context, cfg *config.Config) ([]byte, error) {
	u, err := url.Parse(weatherEndpoint)
	if err != nil {
		return nil, fmt.Errorf("fetcher: parse url: %w", err)
	}

	q := u.Query()
	q.Set("lat", cfg.Latitude)
	q.Set("lon", cfg.Longitude)
	q.Set("appid", cfg.OpenWeatherAPIKey)
	q.Set("units", "metric")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("fetcher: create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetcher: http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fetcher: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		preview := body
		if len(preview) > 256 {
			preview = preview[:256]
		}
		return nil, fmt.Errorf("fetcher: unexpected status %d %s: %s", resp.StatusCode, resp.Status, preview)
	}

	return body, nil
}
