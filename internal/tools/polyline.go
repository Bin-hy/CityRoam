package tools

import (
	"math"
	"strconv"
	"strings"
)

const (
	metersPerDegreeLat = 110540.0
	metersPerDegreeLng = 111320.0
)

// parsePolyline decodes AMap's "lng,lat;lng,lat" polylines.
func parsePolyline(raw string) [][2]float64 {
	segments := strings.Split(raw, ";")
	points := make([][2]float64, 0, len(segments))
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		parts := strings.Split(segment, ",")
		if len(parts) != 2 {
			continue
		}
		lng, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			continue
		}
		lat, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			continue
		}
		point := [2]float64{lng, lat}
		if len(points) > 0 && points[len(points)-1] == point {
			continue
		}
		points = append(points, point)
	}
	return points
}

// simplifyPolyline applies Douglas-Peucker with plain-meter tolerance.
func simplifyPolyline(points [][2]float64, toleranceMeters float64) [][2]float64 {
	if len(points) < 3 || toleranceMeters <= 0 {
		return append([][2]float64(nil), points...)
	}
	keep := make([]bool, len(points))
	keep[0] = true
	keep[len(points)-1] = true
	simplifySegment(points, 0, len(points)-1, toleranceMeters, keep)
	out := make([][2]float64, 0, len(points))
	for index, point := range points {
		if keep[index] {
			out = append(out, point)
		}
	}
	return out
}

func simplifySegment(points [][2]float64, first, last int, tolerance float64, keep []bool) {
	if last <= first+1 {
		return
	}
	origin := points[0]
	maxDistance := 0.0
	maxIndex := first + 1
	for index := first + 1; index < last; index++ {
		distance := pointSegmentDistance(project(points[index], origin), project(points[first], origin), project(points[last], origin))
		if distance > maxDistance {
			maxDistance = distance
			maxIndex = index
		}
	}
	if maxDistance <= tolerance {
		return
	}
	keep[maxIndex] = true
	simplifySegment(points, first, maxIndex, tolerance, keep)
	simplifySegment(points, maxIndex, last, tolerance, keep)
}

func project(point, origin [2]float64) [2]float64 {
	scale := math.Cos(origin[1] * math.Pi / 180)
	return [2]float64{
		(point[0] - origin[0]) * metersPerDegreeLng * scale,
		(point[1] - origin[1]) * metersPerDegreeLat,
	}
}

func pointSegmentDistance(point, start, end [2]float64) float64 {
	dx := end[0] - start[0]
	dy := end[1] - start[1]
	if dx == 0 && dy == 0 {
		return math.Hypot(point[0]-start[0], point[1]-start[1])
	}
	t := ((point[0]-start[0])*dx + (point[1]-start[1])*dy) / (dx*dx + dy*dy)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	projectedX := start[0] + t*dx
	projectedY := start[1] + t*dy
	return math.Hypot(point[0]-projectedX, point[1]-projectedY)
}

// capPolyline evenly samples points down to max while preserving endpoints.
func capPolyline(points [][2]float64, max int) [][2]float64 {
	if max < 2 {
		if len(points) <= 2 {
			return append([][2]float64(nil), points...)
		}
		return [][2]float64{points[0], points[len(points)-1]}
	}
	if len(points) <= max {
		return append([][2]float64(nil), points...)
	}
	out := make([][2]float64, 0, max)
	step := float64(len(points)-1) / float64(max-1)
	for index := 0; index < max; index++ {
		position := int(math.Round(float64(index) * step))
		if position >= len(points) {
			position = len(points) - 1
		}
		if len(out) > 0 && out[len(out)-1] == points[position] {
			continue
		}
		out = append(out, points[position])
	}
	if last := points[len(points)-1]; out[len(out)-1] != last {
		out[len(out)-1] = last
	}
	return out
}
