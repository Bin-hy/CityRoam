package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"trippilot/internal/agent"
	"trippilot/internal/domain"
	"trippilot/internal/repo"
)

const (
	maxInputRunes = 2000
)

// TripStore is the subset of the repository the trip API needs.
type TripStore interface {
	CreateTripAndRun(ctx context.Context, trip repo.CreateTripParams, run repo.CreateRunParams) (repo.Trip, repo.Run, bool, error)
	GetTrip(ctx context.Context, id string) (repo.Trip, error)
	SoftDeleteTrip(ctx context.Context, id string) error
	ListRunsByTrip(ctx context.Context, tripID string) ([]repo.Run, error)
	ListPlanVersions(ctx context.Context, tripID string) ([]repo.PlanVersion, error)
	GetPlanVersionBySeq(ctx context.Context, tripID string, seq int32) (repo.PlanVersion, error)
	GetPlanVersion(ctx context.Context, id string) (repo.PlanVersion, error)
	RestorePlanVersion(ctx context.Context, arg repo.RestorePlanVersionParams) (repo.PlanVersion, error)
	CreateShareLink(ctx context.Context, tripID, versionID string) (repo.ShareLink, error)
	GetShareLink(ctx context.Context, slug string) (repo.ShareLink, error)
	GetRunByRequestID(ctx context.Context, requestID string) (repo.Run, error)
	CreateRun(ctx context.Context, arg repo.CreateRunParams) (repo.Run, error)
	SaveRunState(ctx context.Context, arg repo.UpdateRunStateParams) error
	GetRun(ctx context.Context, id string) (repo.Run, error)
}

// RunStarter is implemented by agent.Runner.
type RunStarter interface {
	Start(ctx context.Context, runID string) error
	StartRequest(ctx context.Context, runID, requestID string) error
	Resume(ctx context.Context, runID string, action agent.ResumeAction) error
	Cancel(ctx context.Context, runID string) error
}

type createTripRequest struct {
	Input           string `json:"input"`
	City            string `json:"city"`
	ClientRequestID string `json:"client_request_id"`
}

type createTripResponse struct {
	TripID    string `json:"trip_id"`
	RunID     string `json:"run_id"`
	EventsURL string `json:"events_url"`
}

type tripResponse struct {
	ID            string            `json:"id"`
	City          string            `json:"city"`
	Days          int32             `json:"days"`
	BudgetYuan    *int32            `json:"budget_yuan,omitempty"`
	Status        string            `json:"status"`
	Constraints   json.RawMessage   `json:"constraints"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	LatestRun     *runSummary       `json:"latest_run,omitempty"`
	LatestVersion *repo.PlanVersion `json:"latest_version,omitempty"`
}

type runSummary struct {
	ID        string     `json:"id"`
	Mode      string     `json:"mode"`
	Status    string     `json:"status"`
	Stage     string     `json:"stage"`
	Round     int32      `json:"round"`
	CreatedAt time.Time  `json:"created_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	ErrorCode *string    `json:"error_code,omitempty"`
}

func handleCreateTrip(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createTripRequest
		if err := decodeJSON(w, r, &req); err != nil {
			WriteError(w, r, err)
			return
		}
		input := strings.TrimSpace(req.Input)
		if input == "" {
			WriteError(w, r, &Error{Status: http.StatusBadRequest, Code: "invalid_input", Message: "行程需求不能为空"})
			return
		}
		if utf8.RuneCountInString(input) > maxInputRunes {
			WriteError(w, r, &Error{Status: http.StatusBadRequest, Code: "input_too_long", Message: "行程需求过长"})
			return
		}
		city, err := normalizeCity(req.City)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		requestID := strings.TrimSpace(req.ClientRequestID)
		if requestID == "" {
			requestID = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		}
		if requestID != "" && !validRequestID(requestID) {
			WriteError(w, r, &Error{Status: http.StatusBadRequest, Code: "invalid_request_id", Message: "幂等键不合法"})
			return
		}

		days := inferDays(input)
		constraints, err := json.Marshal(map[string]any{"city": city, "days": days})
		if err != nil {
			WriteError(w, r, errInternal(err))
			return
		}
		var idempotency *string
		if requestID != "" {
			idempotency = &requestID
		}
		trip, run, created, err := deps.Store.CreateTripAndRun(r.Context(), repo.CreateTripParams{
			City:        city,
			Days:        days,
			Constraints: constraints,
			Status:      "planning",
			RequestID:   idempotency,
		}, repo.CreateRunParams{
			Mode:        "initial",
			Status:      agent.StatusQueued,
			Stage:       "admit",
			Input:       input,
			Constraints: constraints,
			RequestID:   idempotency,
		})
		if err != nil {
			WriteError(w, r, errInternal(err))
			return
		}

		if created {
			startCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Second)
			defer cancel()
			if requestID != "" {
				err = deps.Runner.StartRequest(startCtx, run.ID, requestID)
			} else {
				err = deps.Runner.Start(startCtx, run.ID)
			}
			if err != nil && !errors.Is(err, agent.ErrInvalidRunStatus) {
				WriteError(w, r, &Error{Status: http.StatusInternalServerError, Code: "start_failed", Message: "启动规划失败"})
				return
			}
		}

		writeJSON(w, http.StatusAccepted, createTripResponse{
			TripID:    trip.ID,
			RunID:     run.ID,
			EventsURL: eventsURL(run.ID),
		})
	}
}

func handleGetTrip(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tripID := chi.URLParam(r, "tripID")
		trip, err := deps.Store.GetTrip(r.Context(), tripID)
		if err != nil {
			WriteError(w, r, mapStoreError(err))
			return
		}
		runs, err := deps.Store.ListRunsByTrip(r.Context(), tripID)
		if err != nil {
			WriteError(w, r, errInternal(err))
			return
		}
		versions, err := deps.Store.ListPlanVersions(r.Context(), tripID)
		if err != nil {
			WriteError(w, r, errInternal(err))
			return
		}

		response := tripResponse{
			ID:          trip.ID,
			City:        trip.City,
			Days:        trip.Days,
			BudgetYuan:  trip.BudgetYuan,
			Status:      trip.Status,
			Constraints: trip.Constraints,
			CreatedAt:   trip.CreatedAt,
			UpdatedAt:   trip.UpdatedAt,
		}
		if len(runs) > 0 {
			latest := runs[0]
			response.LatestRun = &runSummary{
				ID:        latest.ID,
				Mode:      latest.Mode,
				Status:    latest.Status,
				Stage:     latest.Stage,
				Round:     latest.Round,
				CreatedAt: latest.CreatedAt,
				StartedAt: latest.StartedAt,
				ErrorCode: latest.ErrorCode,
			}
		}
		if len(versions) > 0 {
			latest := versions[0]
			latest.Research = normalizeVersionResearch(latest.Research)
			response.LatestVersion = &latest
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func handleDeleteTrip(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := deps.Store.SoftDeleteTrip(r.Context(), chi.URLParam(r, "tripID")); err != nil {
			WriteError(w, r, errInternal(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type replanRequest struct {
	BaseVersion     int32                `json:"base_version"`
	Constraints     domain.ConstraintSet `json:"constraints"`
	ClientRequestID string               `json:"client_request_id"`
}

// handleReplan starts a fresh run with the full constraint set as a patch so
// budget or lodging switches can change research inputs without an adjustment
// proposal.
func handleReplan(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req replanRequest
		if err := decodeJSONStrict(w, r, &req); err != nil {
			WriteError(w, r, err)
			return
		}
		if req.BaseVersion <= 0 {
			WriteError(w, r, &Error{Status: http.StatusBadRequest, Code: "invalid_base_version", Message: "缺少基础版本"})
			return
		}
		if strings.TrimSpace(req.Constraints.City) == "" {
			WriteError(w, r, &Error{Status: http.StatusBadRequest, Code: "invalid_city", Message: "缺少城市"})
			return
		}
		city, err := normalizeCity(req.Constraints.City)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if req.Constraints.Days < 1 || req.Constraints.Days > 4 {
			WriteError(w, r, &Error{Status: http.StatusBadRequest, Code: "invalid_days", Message: "行程天数不合法"})
			return
		}
		requestID := strings.TrimSpace(req.ClientRequestID)
		if requestID == "" {
			requestID = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		}
		if requestID != "" && !validRequestID(requestID) {
			WriteError(w, r, &Error{Status: http.StatusBadRequest, Code: "invalid_request_id", Message: "幂等键不合法"})
			return
		}

		tripID := chi.URLParam(r, "tripID")
		trip, err := deps.Store.GetTrip(r.Context(), tripID)
		if err != nil {
			WriteError(w, r, mapStoreError(err))
			return
		}
		base, err := deps.Store.GetPlanVersionBySeq(r.Context(), tripID, req.BaseVersion)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				WriteError(w, r, &Error{Status: http.StatusNotFound, Code: "version_not_found", Message: "基础版本不存在"})
				return
			}
			WriteError(w, r, errInternal(err))
			return
		}

		if requestID != "" {
			if existing, err := deps.Store.GetRunByRequestID(r.Context(), requestID); err == nil {
				if existing.TripID != tripID || existing.Mode != "replan" {
					WriteError(w, r, &Error{Status: http.StatusConflict, Code: "request_id_conflict", Message: "幂等键已被其他请求使用"})
					return
				}
				writeJSON(w, http.StatusAccepted, adjustmentResponse{RunID: existing.ID, EventsURL: eventsURL(existing.ID)})
				return
			}
		}

		var plan domain.Itinerary
		if err := json.Unmarshal(base.Plan, &plan); err != nil {
			WriteError(w, r, errInternal(err))
			return
		}
		req.Constraints.City = city
		constraintsJSON, err := json.Marshal(req.Constraints)
		if err != nil {
			WriteError(w, r, errInternal(err))
			return
		}

		var idempotency *string
		if requestID != "" {
			idempotency = &requestID
		}
		baseVersionID := base.ID
		run, err := deps.Store.CreateRun(r.Context(), repo.CreateRunParams{
			TripID:        trip.ID,
			Mode:          "replan",
			Status:        agent.StatusQueued,
			Stage:         "parse",
			Input:         replanSummary(city, req.Constraints.Days),
			Constraints:   constraintsJSON,
			BaseVersionID: &baseVersionID,
			RequestID:     idempotency,
		})
		if err != nil {
			WriteError(w, r, errInternal(err))
			return
		}

		previous := plan
		patch := req.Constraints
		state := agent.TripState{
			TripID:          trip.ID,
			RunID:           run.ID,
			Mode:            "replan",
			Input:           replanSummary(city, req.Constraints.Days),
			Constraints:     req.Constraints,
			ConstraintPatch: &patch,
			Previous:        &previous,
			PreviousSeq:     int(base.Seq),
			Status:          agent.StatusQueued,
			Stage:           "parse",
		}
		encoded, err := state.Encode()
		if err != nil {
			WriteError(w, r, errInternal(err))
			return
		}
		if err := deps.Store.SaveRunState(r.Context(), repo.UpdateRunStateParams{
			ID:     run.ID,
			State:  encoded,
			Stage:  "parse",
			Round:  0,
			Status: agent.StatusQueued,
		}); err != nil {
			WriteError(w, r, errInternal(err))
			return
		}

		startCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Second)
		defer cancel()
		if requestID != "" {
			err = deps.Runner.StartRequest(startCtx, run.ID, requestID)
		} else {
			err = deps.Runner.Start(startCtx, run.ID)
		}
		if err != nil && !errors.Is(err, agent.ErrInvalidRunStatus) {
			WriteError(w, r, &Error{Status: http.StatusInternalServerError, Code: "start_failed", Message: "启动重排失败"})
			return
		}

		writeJSON(w, http.StatusAccepted, adjustmentResponse{
			RunID:     run.ID,
			EventsURL: eventsURL(run.ID),
		})
	}
}

func replanSummary(city string, days int) string {
	name := city
	if city == "beijing" {
		name = "北京"
	}
	return fmt.Sprintf("按最新约束重排%s %d 天行程", name, days)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	return decodeJSONBody(w, r, dst, false)
}

// decodeJSONStrict rejects unknown fields, which keeps replan patches from
// silently dropping a misspelled constraint.
func decodeJSONStrict(w http.ResponseWriter, r *http.Request, dst any) error {
	return decodeJSONBody(w, r, dst, true)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, strict bool) error {
	decoder := json.NewDecoder(r.Body)
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return &Error{Status: http.StatusRequestEntityTooLarge, Code: "payload_too_large", Message: "请求体超过大小限制"}
		}
		if strict {
			return &Error{Status: http.StatusBadRequest, Code: "invalid_json", Message: "请求体包含未知字段或不是合法 JSON"}
		}
		return &Error{Status: http.StatusBadRequest, Code: "invalid_json", Message: "请求体不是合法 JSON"}
	}
	return nil
}

func normalizeCity(raw string) (string, error) {
	city := strings.ToLower(strings.TrimSpace(raw))
	switch city {
	case "", "beijing", "北京", "北京市":
		return "beijing", nil
	default:
		return "", &Error{Status: http.StatusBadRequest, Code: "unsupported_city", Message: "当前仅支持北京"}
	}
}

func inferDays(input string) int32 {
	terms := []struct {
		word string
		days int32
	}{
		{"四天", 4}, {"4天", 4}, {"四日", 4}, {"4日", 4},
		{"三天", 3}, {"3天", 3}, {"三日", 3}, {"3日", 3},
		{"两天", 2}, {"2天", 2}, {"二天", 2}, {"两日", 2}, {"2日", 2},
		{"一天", 1}, {"1天", 1}, {"一日", 1}, {"1日", 1},
	}
	for _, term := range terms {
		if strings.Contains(input, term.word) {
			return term.days
		}
	}
	return 2
}

func eventsURL(runID string) string {
	return "/api/runs/" + runID + "/events"
}

func mapStoreError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return &Error{Status: http.StatusNotFound, Code: "not_found", Message: "行程不存在"}
	}
	return errInternal(err)
}

func errInternal(error) error {
	return &Error{Status: http.StatusInternalServerError, Code: "internal", Message: "服务暂时不可用"}
}
