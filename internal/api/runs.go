package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"trippilot/internal/agent"
)

type resumeRequest struct {
	Action      string          `json:"action"`
	Input       string          `json:"input"`
	Constraints json.RawMessage `json:"constraints"`
}

type runActionResponse struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

func handleResumeRun(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req resumeRequest
		if err := decodeJSON(w, r, &req); err != nil {
			WriteError(w, r, err)
			return
		}

		runID := chi.URLParam(r, "runID")
		run, err := deps.Store.GetRun(r.Context(), runID)
		if err != nil {
			WriteError(w, r, mapStoreError(err))
			return
		}

		action, err := resumeAction(req)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if err := deps.Runner.Resume(r.Context(), run.ID, action); err != nil {
			if errors.Is(err, agent.ErrInvalidRunStatus) {
				WriteError(w, r, invalidRunState())
				return
			}
			logError(deps.Logger, "resume run", r, err)
			WriteError(w, r, &Error{Status: http.StatusInternalServerError, Code: "resume_failed", Message: "恢复运行失败"})
			return
		}
		writeJSON(w, http.StatusAccepted, runActionResponse{
			RunID:  run.ID,
			Status: resumedStatus(action.Kind),
		})
	}
}

func handleCancelRun(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := chi.URLParam(r, "runID")
		run, err := deps.Store.GetRun(r.Context(), runID)
		if err != nil {
			WriteError(w, r, mapStoreError(err))
			return
		}
		if err := deps.Runner.Cancel(r.Context(), runID); err != nil {
			logError(deps.Logger, "cancel run", r, err)
			WriteError(w, r, &Error{Status: http.StatusInternalServerError, Code: "cancel_failed", Message: "取消运行失败"})
			return
		}
		status := run.Status
		if !terminalRunStatus(status) {
			status = agent.StatusCancelled
		}
		writeJSON(w, http.StatusAccepted, runActionResponse{RunID: runID, Status: status})
	}
}

func resumeAction(req resumeRequest) (agent.ResumeAction, error) {
	switch req.Action {
	case "provide_constraints":
		input := strings.TrimSpace(req.Input)
		if len(req.Constraints) > 0 {
			if err := validateConstraints(req.Constraints); err != nil {
				return agent.ResumeAction{}, err
			}
			input = strings.TrimSpace(input + " 补充约束：" + string(req.Constraints))
		}
		if input == "" {
			return agent.ResumeAction{}, &Error{Status: http.StatusBadRequest, Code: "invalid_input", Message: "缺少补充信息"}
		}
		if utf8.RuneCountInString(input) > maxInputRunes {
			return agent.ResumeAction{}, &Error{Status: http.StatusBadRequest, Code: "invalid_input", Message: "补充信息过长"}
		}
		return agent.ResumeAction{Kind: agent.ResumeSupplement, Input: input}, nil
	case "accept":
		return agent.ResumeAction{Kind: agent.ResumeAccept}, nil
	case "reject":
		return agent.ResumeAction{Kind: agent.ResumeReject}, nil
	default:
		return agent.ResumeAction{}, &Error{Status: http.StatusBadRequest, Code: "invalid_action", Message: "不支持的恢复动作"}
	}
}

func validateConstraints(raw json.RawMessage) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return &Error{Status: http.StatusBadRequest, Code: "invalid_constraints", Message: "补充约束必须是 JSON 对象"}
	}
	return nil
}

func invalidRunState() error {
	return &Error{
		Status:  http.StatusConflict,
		Code:    "invalid_run_state",
		Message: "当前状态不允许该操作",
	}
}

func resumedStatus(kind agent.ResumeKind) string {
	switch kind {
	case agent.ResumeAccept:
		return agent.StatusCompleted
	case agent.ResumeReject:
		return agent.StatusCancelled
	default:
		return agent.StatusQueued
	}
}

func terminalRunStatus(status string) bool {
	switch status {
	case agent.StatusCompleted, agent.StatusFailed, agent.StatusCancelled:
		return true
	default:
		return false
	}
}
