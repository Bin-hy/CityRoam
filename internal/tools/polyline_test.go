package tools

import (
	"math"
	"testing"
)

func TestParsePolyline(t *testing.T) {
	points := parsePolyline("116.40,39.90; bad ;116.41,39.91;116.41,39.91;116.42;116.43,oops; 116.44,39.94 ")
	if len(points) != 3 {
		t.Fatalf("points = %+v", points)
	}
	if points[0] != [2]float64{116.40, 39.90} || points[1] != [2]float64{116.41, 39.91} {
		t.Fatalf("points = %+v", points)
	}
	if points[2] != [2]float64{116.44, 39.94} {
		t.Fatalf("last point = %+v", points[2])
	}
	if got := parsePolyline(""); len(got) != 0 {
		t.Fatalf("empty parse = %+v", got)
	}
}

func TestSimplifyPolyline(t *testing.T) {
	straight := [][2]float64{{116.4000, 39.9000}, {116.4001, 39.9000}, {116.4002, 39.9000}, {116.4003, 39.9000}}
	simplified := simplifyPolyline(straight, 15)
	if len(simplified) != 2 {
		t.Fatalf("straight simplified = %+v", simplified)
	}
	if simplified[0] != straight[0] || simplified[1] != straight[len(straight)-1] {
		t.Fatalf("endpoints lost: %+v", simplified)
	}

	bend := [][2]float64{{116.4000, 39.9000}, {116.4005, 39.9020}, {116.4010, 39.9000}}
	if got := simplifyPolyline(bend, 15); len(got) != 3 {
		t.Fatalf("bend simplified = %+v", got)
	}
	if got := simplifyPolyline(nil, 15); len(got) != 0 {
		t.Fatalf("nil simplified = %+v", got)
	}
	if got := simplifyPolyline([][2]float64{{0, 0}, {1, 1}}, 15); len(got) != 2 {
		t.Fatalf("short simplified = %+v", got)
	}
}

func TestCapPolyline(t *testing.T) {
	points := make([][2]float64, 61)
	for index := range points {
		points[index] = [2]float64{116.4 + float64(index)*0.0001, 39.9 + float64(index)*0.0001}
	}
	capped := capPolyline(points, 60)
	if len(capped) != 60 {
		t.Fatalf("capped len = %d", len(capped))
	}
	if capped[0] != points[0] || capped[len(capped)-1] != points[len(points)-1] {
		t.Fatalf("endpoints lost")
	}
	for index := 1; index < len(capped); index++ {
		if math.Abs(capped[index][0]-capped[index-1][0]) < 1e-9 {
			t.Fatalf("duplicate sample at %d: %+v", index, capped[index])
		}
	}

	if got := capPolyline(points, 1); len(got) != 2 || got[0] != points[0] || got[1] != points[60] {
		t.Fatalf("max<2 = %+v", got)
	}
	if got := capPolyline(points[:5], 60); len(got) != 5 {
		t.Fatalf("short cap = %+v", got)
	}
	if got := capPolyline([][2]float64{{1, 1}}, 0); len(got) != 1 || got[0] != [2]float64{1, 1} {
		t.Fatalf("single point = %+v", got)
	}
}
