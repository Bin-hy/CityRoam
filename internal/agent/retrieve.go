package agent

import (
	"context"
	"math"
	"strings"

	"trippilot/internal/domain"
)

const (
	minCandidates = 40
	maxCandidates = 60
)

// CandidateSource is implemented by the snapshot, AMap and MCP search tools.
type CandidateSource interface {
	SearchCandidates(ctx context.Context, query domain.POIQuery) ([]domain.POI, error)
}

type RetrieveInput struct {
	RunID       string
	Constraints domain.ConstraintSet
}

type RetrieveOutput struct {
	Candidates       []domain.POI
	Attempts         int
	MissingMustVisit []string
}

func WithSnapshotSource(source CandidateSource) NodeOption {
	return func(n *Nodes) { n.snapshot = source }
}

func WithExternalSource(source CandidateSource) NodeOption {
	return func(n *Nodes) { n.external = source }
}

func WithCandidateLimit(limit int) NodeOption {
	return func(n *Nodes) { n.candidateLimit = limit }
}

// Retrieve gathers candidates from the snapshot and the external provider in
// parallel, deduplicates them and retries once with widened keywords when a
// must-visit place is missing.
func (n *Nodes) Retrieve(ctx context.Context, in RetrieveInput) (RetrieveOutput, error) {
	if n.snapshot == nil {
		return RetrieveOutput{}, &NodeError{Code: "source_unavailable", Message: "候选数据源未配置"}
	}
	city := strings.TrimSpace(in.Constraints.City)
	if city == "" {
		return RetrieveOutput{}, &NodeError{Code: "missing_city", Message: "缺少城市"}
	}

	candidates, err := n.retrieveOnce(ctx, city, in.Constraints.MustVisit, false)
	if err != nil {
		return RetrieveOutput{}, err
	}
	attempts := 1
	missing := missingMustVisit(candidates, in.Constraints.MustVisit)

	if len(missing) > 0 {
		attempts = 2
		widened, err := n.retrieveOnce(ctx, city, missing, true)
		if err != nil {
			return RetrieveOutput{}, err
		}
		candidates = mergeCandidates(n.candidateLimit, candidates, widened)
		missing = missingMustVisit(candidates, in.Constraints.MustVisit)
	}

	if len(missing) > 0 {
		return RetrieveOutput{
				Candidates:       candidates,
				Attempts:         attempts,
				MissingMustVisit: missing,
			}, &NodeError{
				Code:    "must_visit_missing",
				Message: "必去地点在候选集中找不到：" + strings.Join(missing, "、"),
			}
	}

	if err := n.events.Publish(ctx, in.RunID, "candidate.retrieved", map[string]any{
		"count":      len(candidates),
		"attempts":   attempts,
		"candidates": candidates,
	}); err != nil {
		return RetrieveOutput{}, &NodeError{Code: "event_failed", Message: "发布候选事件失败", Err: err}
	}

	if len(candidates) < minCandidates {
		n.logger.Warn("candidate set below target",
			"run_id", in.RunID, "count", len(candidates), "target", minCandidates)
	}

	n.logger.Info("candidates retrieved",
		"run_id", in.RunID,
		"count", len(candidates),
		"attempts", attempts,
	)
	return RetrieveOutput{Candidates: candidates, Attempts: attempts}, nil
}

type sourceResult struct {
	pois []domain.POI
	err  error
}

func (n *Nodes) retrieveOnce(ctx context.Context, city string, mustVisit []string, widened bool) ([]domain.POI, error) {
	query := domain.POIQuery{
		City:      city,
		MustVisit: append([]string(nil), mustVisit...),
		Limit:     n.candidateLimit,
	}
	if widened {
		query.Keywords = append([]string(nil), mustVisit...)
	}

	snapshotCh := make(chan sourceResult, 1)
	go func() {
		pois, err := n.snapshot.SearchCandidates(ctx, query)
		snapshotCh <- sourceResult{pois: pois, err: err}
	}()

	var externalCh chan sourceResult
	if n.external != nil {
		externalCh = make(chan sourceResult, 1)
		go func() {
			pois, err := n.external.SearchCandidates(ctx, query)
			externalCh <- sourceResult{pois: pois, err: err}
		}()
	}

	snapshot := <-snapshotCh
	if snapshot.err != nil {
		return nil, &NodeError{Code: "retrieve_failed", Message: "快照检索失败", Err: snapshot.err}
	}

	var external []domain.POI
	if externalCh != nil {
		result := <-externalCh
		if result.err != nil {
			n.logger.Warn("external POI search failed, using snapshot only",
				"error", result.err, "city", city)
		} else {
			external = result.pois
		}
	}

	return mergeCandidates(n.candidateLimit, snapshot.pois, external), nil
}

// mergeCandidates keeps groups in priority order, drops duplicates by place
// and caps the result. The snapshot group already promotes must-visit hits.
func mergeCandidates(limit int, groups ...[]domain.POI) []domain.POI {
	if limit <= 0 || limit > maxCandidates {
		limit = maxCandidates
	}
	merged := make([]domain.POI, 0, limit)
	for _, group := range groups {
		for _, poi := range group {
			if len(merged) >= limit {
				return merged
			}
			if containsPlace(merged, poi) {
				continue
			}
			merged = append(merged, poi)
		}
	}
	return merged
}

func containsPlace(pois []domain.POI, candidate domain.POI) bool {
	for _, existing := range pois {
		if existing.ID == candidate.ID {
			return true
		}
		if samePlace(existing, candidate) {
			return true
		}
	}
	return false
}

func samePlace(a, b domain.POI) bool {
	if !equalName(normalizeName(a.Name), normalizeName(b.Name)) {
		return false
	}
	return distanceMeters(a.Lat, a.Lng, b.Lat, b.Lng) < 1000
}

func missingMustVisit(candidates []domain.POI, wanted []string) []string {
	missing := make([]string, 0, len(wanted))
	for _, name := range wanted {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		found := false
		normalized := normalizeName(trimmed)
		for _, poi := range candidates {
			if poi.ID == trimmed || nameRelated(normalized, normalizeName(poi.Name)) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, trimmed)
		}
	}
	return missing
}

func normalizeName(name string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(name), ""))
	suffixes := []string{"博物院", "博物馆", "风景区", "风景名胜区", "景区", "公园"}
	for _, suffix := range suffixes {
		normalized = strings.TrimSuffix(normalized, suffix)
	}
	return normalized
}

func equalName(a, b string) bool {
	return a != "" && a == b
}

func nameRelated(wanted, candidate string) bool {
	if wanted == "" || candidate == "" {
		return false
	}
	if wanted == candidate {
		return true
	}
	if len([]rune(wanted)) >= 2 && strings.Contains(candidate, wanted) {
		return true
	}
	if len([]rune(candidate)) >= 2 && strings.Contains(wanted, candidate) {
		return true
	}
	return false
}

func distanceMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadius = 6371000.0
	toRad := func(degrees float64) float64 { return degrees * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLng := toRad(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadius * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
