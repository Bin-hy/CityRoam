package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Error is an HTTP error with a stable machine-readable code.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

var (
	errNotFound       = &Error{Status: http.StatusNotFound, Code: "not_found", Message: "资源不存在"}
	errNotImplemented = &Error{Status: http.StatusNotImplemented, Code: "not_implemented", Message: "接口尚未实现"}
	errBadRequest     = &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "请求参数不合法"}
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// WriteError renders the unified error envelope.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	code := "internal"
	message := "服务暂时不可用"
	if apiErr, ok := err.(*Error); ok {
		status = apiErr.Status
		code = apiErr.Code
		message = apiErr.Message
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: errorDetail{
		Code:      code,
		Message:   message,
		RequestID: RequestIDFrom(r.Context()),
	}})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func handleNotImplemented() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, errNotImplemented)
	}
}

func logError(logger *slog.Logger, message string, r *http.Request, err any) {
	logger.Error(message,
		"method", r.Method,
		"path", r.URL.Path,
		"request_id", RequestIDFrom(r.Context()),
		"error", err,
	)
}
