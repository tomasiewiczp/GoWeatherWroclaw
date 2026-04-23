package transformer

import (
	"encoding/json"
	"fmt"
	"time"
)

type ProcessedWeather struct {
	Latitude           float64 `json:"latitude"`
	Longitude          float64 `json:"longitude"`
	Temperature        float64 `json:"temperature"`
	Pressure           int     `json:"pressure"`
	Humidity           int     `json:"humidity"`
	WeatherDescription string  `json:"weather_description"`
	TimestampISO       string  `json:"timestamp_iso"`
}

// rawResponse mirrors the 2.5/weather endpoint structure.
type rawResponse struct {
	Coord   rawCoord     `json:"coord"`
	Dt      int64        `json:"dt"`
	Main    rawMain      `json:"main"`
	Weather []rawWeather `json:"weather"`
}

type rawCoord struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type rawMain struct {
	Temperature float64 `json:"temp"`
	Pressure    int     `json:"pressure"`
	Humidity    int     `json:"humidity"`
}

type rawWeather struct {
	Description string `json:"description"`
}

func Transform(raw []byte) (*ProcessedWeather, error) {
	var resp rawResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("transformer: unmarshal payload: %w", err)
	}

	if len(resp.Weather) == 0 {
		return nil, fmt.Errorf("transformer: missing weather description in payload")
	}
	description := resp.Weather[0].Description

	return &ProcessedWeather{
		Latitude:           resp.Coord.Lat,
		Longitude:          resp.Coord.Lon,
		Temperature:        resp.Main.Temperature,
		Pressure:           resp.Main.Pressure,
		Humidity:           resp.Main.Humidity,
		WeatherDescription: description,
		TimestampISO:       time.Unix(resp.Dt, 0).UTC().Format(time.RFC3339),
	}, nil
}
