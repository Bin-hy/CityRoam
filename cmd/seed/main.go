package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"trippilot/internal/domain"
	"trippilot/internal/logging"
	"trippilot/internal/repo"
)

const (
	minLat = 39.4
	maxLat = 41.1
	minLng = 115.4
	maxLng = 117.5
)

func main() {
	path := flag.String("file", "data/poi/beijing.json", "POI snapshot to import")
	flag.Parse()

	if err := run(*path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(path string) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read snapshot: %w", err)
	}
	var pois []domain.POI
	if err := json.Unmarshal(raw, &pois); err != nil {
		return fmt.Errorf("parse snapshot: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := repo.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer store.Close()

	logger := logging.New(slog.LevelInfo)
	imported, skipped := 0, 0
	seen := make(map[string]struct{}, len(pois))
	for _, poi := range pois {
		if err := validatePOI(poi, seen); err != nil {
			logger.Warn("skip POI", "id", poi.ID, "error", err)
			skipped++
			continue
		}
		seen[poi.ID] = struct{}{}
		if err := store.UpsertPOI(ctx, poi); err != nil {
			return err
		}
		imported++
	}

	count, err := store.CountPOIs(ctx, "beijing")
	if err != nil {
		return err
	}
	logger.Info("poi snapshot imported", "imported", imported, "skipped", skipped, "total", count)
	return nil
}

func validatePOI(poi domain.POI, seen map[string]struct{}) error {
	if poi.ID == "" || poi.City == "" || poi.Name == "" || poi.Category == "" {
		return errors.New("id, city, name, and category are required")
	}
	if _, duplicated := seen[poi.ID]; duplicated {
		return errors.New("duplicate id")
	}
	if poi.City != "beijing" {
		return fmt.Errorf("unsupported city %q", poi.City)
	}
	if poi.Lat < minLat || poi.Lat > maxLat || poi.Lng < minLng || poi.Lng > maxLng {
		return fmt.Errorf("coordinates out of Beijing range: %f,%f", poi.Lat, poi.Lng)
	}
	if poi.CoordSystem != "" && poi.CoordSystem != domain.CoordSystemGCJ02 {
		return fmt.Errorf("unsupported coordinate system %q", poi.CoordSystem)
	}
	if len(poi.OpenHours) == 0 {
		return errors.New("open hours are required")
	}
	for _, openRange := range poi.OpenHours {
		if len(openRange.Days) == 0 {
			return errors.New("open range has no days")
		}
		for _, day := range openRange.Days {
			if day < 0 || day > 6 {
				return fmt.Errorf("weekday %d out of range", day)
			}
		}
		start, startErr := domain.ParseClock(openRange.Start)
		end, endErr := domain.ParseClock(openRange.End)
		if startErr != nil || endErr != nil {
			return errors.New("open range must use HH:MM")
		}
		if end <= start {
			return errors.New("open range end must be after start")
		}
	}
	if poi.SuggestedMinutes <= 0 {
		return errors.New("suggested_minutes must be positive")
	}
	if poi.Source == "" {
		return errors.New("source is required")
	}
	return nil
}
