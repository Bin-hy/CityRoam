package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"trippilot/internal/agent"
	"trippilot/internal/domain"
	"trippilot/internal/domain/diff"
	"trippilot/internal/repo"
)

type createAdjustmentRequest struct {
	Input           string `json:"input"`
	BaseVersion     int32  `json:"base_version"`
	ClientRequestID string `json:"client_request_id"`
}

type adjustmentResponse struct {
	RunID     string `json:"run_id"`
	EventsURL string `json:"events_url"`
}

func handleCreateAdjustment(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createAdjustmentRequest
		if err := decodeJSON(w, r, &req); err != nil {
			WriteError(w, r, err)
			return
		}
		input := strings.TrimSpace(req.Input)
		if input == "" || utf8.RuneCountInString(input) > maxInputRunes {
			WriteError(w, r, &Error{Status: http.StatusBadRequest, Code: "invalid_input", Message: "调整需求为空或过长"})
			return
		}
		if req.BaseVersion <= 0 {
			WriteError(w, r, &Error{Status: http.StatusBadRequest, Code: "invalid_base_version", Message: "缺少基础版本"})
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
				if existing.TripID != tripID || existing.Mode != "adjustment" {
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
		var constraints domain.ConstraintSet
		if len(base.Constraints) > 0 {
			if err := json.Unmarshal(base.Constraints, &constraints); err != nil {
				WriteError(w, r, errInternal(err))
				return
			}
		}
		var violations []domain.Violation
		if len(base.Violations) > 0 {
			_ = json.Unmarshal(base.Violations, &violations)
		}

		var idempotency *string
		if requestID != "" {
			idempotency = &requestID
		}
		baseVersionID := base.ID
		run, err := deps.Store.CreateRun(r.Context(), repo.CreateRunParams{
			TripID:        trip.ID,
			Mode:          "adjustment",
			Status:        agent.StatusQueued,
			Stage:         "adjust",
			Input:         input,
			Constraints:   base.Constraints,
			BaseVersionID: &baseVersionID,
			RequestID:     idempotency,
		})
		if err != nil {
			WriteError(w, r, errInternal(err))
			return
		}

		previous := plan
		state := agent.TripState{
			TripID:      trip.ID,
			RunID:       run.ID,
			Mode:        "adjustment",
			Input:       input,
			Constraints: constraints,
			Plan:        plan,
			Draft:       draftFromPlan(plan),
			Violations:  violations,
			Previous:    &previous,
			PreviousSeq: int(base.Seq),
			Status:      agent.StatusQueued,
			Stage:       "adjust",
		}
		encoded, err := state.Encode()
		if err != nil {
			WriteError(w, r, errInternal(err))
			return
		}
		if err := deps.Store.SaveRunState(r.Context(), repo.UpdateRunStateParams{
			ID:     run.ID,
			State:  encoded,
			Stage:  "adjust",
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
			WriteError(w, r, &Error{Status: http.StatusInternalServerError, Code: "start_failed", Message: "启动调整失败"})
			return
		}

		writeJSON(w, http.StatusAccepted, adjustmentResponse{
			RunID:     run.ID,
			EventsURL: eventsURL(run.ID),
		})
	}
}

// draftFromPlan rebuilds a draft so the adjustment scheduler can re-merge a
// patched day and compare untouched days against the stored version.
func draftFromPlan(plan domain.Itinerary) domain.DraftPlan {
	draft := domain.DraftPlan{Days: make([]domain.DraftDay, 0, len(plan.Days))}
	for _, day := range plan.Days {
		items := make([]domain.DraftItem, 0, len(day.Blocks))
		for _, block := range day.Blocks {
			if block.Type == domain.BlockTypeTransit || block.POIID == "" {
				continue
			}
			items = append(items, domain.DraftItem{
				POIID:       block.POIID,
				Type:        block.Type,
				DurationMin: block.DurationMin,
			})
		}
		draft.Days = append(draft.Days, domain.DraftDay{Date: day.Date, Items: items})
	}
	return draft
}

func handleListVersions(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versions, err := deps.Store.ListPlanVersions(r.Context(), chi.URLParam(r, "tripID"))
		if err != nil {
			WriteError(w, r, errInternal(err))
			return
		}
		if versions == nil {
			versions = []repo.PlanVersion{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
	}
}

func handleGetVersion(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seq, err := strconv.ParseInt(chi.URLParam(r, "seq"), 10, 32)
		if err != nil || seq <= 0 {
			WriteError(w, r, &Error{Status: http.StatusBadRequest, Code: "invalid_seq", Message: "版本号不合法"})
			return
		}
		version, err := deps.Store.GetPlanVersionBySeq(r.Context(), chi.URLParam(r, "tripID"), int32(seq))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				WriteError(w, r, &Error{Status: http.StatusNotFound, Code: "version_not_found", Message: "版本不存在"})
				return
			}
			WriteError(w, r, errInternal(err))
			return
		}
		writeJSON(w, http.StatusOK, version)
	}
}

func handleDiff(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tripID := chi.URLParam(r, "tripID")
		versions, err := deps.Store.ListPlanVersions(r.Context(), tripID)
		if err != nil {
			WriteError(w, r, errInternal(err))
			return
		}
		from, to, err := resolveDiffSeqs(r, versions)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		before, err := deps.Store.GetPlanVersionBySeq(r.Context(), tripID, from)
		if err != nil {
			WriteError(w, r, mapVersionError(err))
			return
		}
		after, err := deps.Store.GetPlanVersionBySeq(r.Context(), tripID, to)
		if err != nil {
			WriteError(w, r, mapVersionError(err))
			return
		}
		var beforePlan, afterPlan domain.Itinerary
		if err := json.Unmarshal(before.Plan, &beforePlan); err != nil {
			WriteError(w, r, errInternal(err))
			return
		}
		if err := json.Unmarshal(after.Plan, &afterPlan); err != nil {
			WriteError(w, r, errInternal(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"from": from,
			"to":   to,
			"diff": diff.Diff(beforePlan, afterPlan),
		})
	}
}

func resolveDiffSeqs(r *http.Request, versions []repo.PlanVersion) (int32, int32, error) {
	parse := func(key string) (int32, bool, error) {
		raw := r.URL.Query().Get(key)
		if raw == "" {
			return 0, false, nil
		}
		value, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || value <= 0 {
			return 0, false, &Error{Status: http.StatusBadRequest, Code: "invalid_seq", Message: "版本号不合法"}
		}
		return int32(value), true, nil
	}
	from, hasFrom, err := parse("from")
	if err != nil {
		return 0, 0, err
	}
	to, hasTo, err := parse("to")
	if err != nil {
		return 0, 0, err
	}

	if !hasFrom && !hasTo {
		if len(versions) < 2 {
			return 0, 0, &Error{Status: http.StatusConflict, Code: "not_enough_versions", Message: "至少需要两个版本才能比较"}
		}
		return versions[1].Seq, versions[0].Seq, nil
	}
	if !hasTo {
		to = versions[0].Seq
	}
	if !hasFrom {
		from = to - 1
		if from <= 0 {
			return 0, 0, &Error{Status: http.StatusBadRequest, Code: "invalid_seq", Message: "缺少可比较的起始版本"}
		}
	}
	return from, to, nil
}

func mapVersionError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return &Error{Status: http.StatusNotFound, Code: "version_not_found", Message: "版本不存在"}
	}
	return errInternal(err)
}
