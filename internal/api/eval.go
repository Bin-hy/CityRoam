package api

import (
	"context"
	"net/http"

	"trippilot/internal/repo"
)

// EvalService is implemented by internal/eval once evaluation batches exist.
// Keeping it an interface lets the HTTP layer stay storage-neutral.
type EvalService interface {
	ListCases(ctx context.Context) ([]repo.EvalCase, error)
	Start(ctx context.Context) error
	ListRuns(ctx context.Context) ([]repo.EvalRun, error)
}

func handleListEvalCases(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Eval == nil {
			writeEvalUnavailable(w, r)
			return
		}
		cases, err := deps.Eval.ListCases(r.Context())
		if err != nil {
			logError(deps.Logger, "list eval cases", r, err)
			WriteError(w, r, &Error{Status: http.StatusInternalServerError, Code: "eval_failed", Message: "读取评测用例失败"})
			return
		}
		if cases == nil {
			cases = []repo.EvalCase{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"cases": cases})
	}
}

func handleStartEval(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Eval == nil {
			writeEvalUnavailable(w, r)
			return
		}
		if err := deps.Eval.Start(r.Context()); err != nil {
			logError(deps.Logger, "start eval", r, err)
			WriteError(w, r, &Error{Status: http.StatusInternalServerError, Code: "eval_failed", Message: "启动评测失败"})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
	}
}

func handleListEvalRuns(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Eval == nil {
			writeEvalUnavailable(w, r)
			return
		}
		runs, err := deps.Eval.ListRuns(r.Context())
		if err != nil {
			logError(deps.Logger, "list eval runs", r, err)
			WriteError(w, r, &Error{Status: http.StatusInternalServerError, Code: "eval_failed", Message: "读取评测结果失败"})
			return
		}
		if runs == nil {
			runs = []repo.EvalRun{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
	}
}

func writeEvalUnavailable(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, &Error{Status: http.StatusServiceUnavailable, Code: "eval_unavailable", Message: "评测服务未启用"})
}
