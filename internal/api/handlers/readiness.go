package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

const readinessTimeout = 3 * time.Second

type ReadinessChecker interface {
	Check(context.Context) error
}

type ReadinessHandler struct {
	checker ReadinessChecker
}

type readinessResponse struct {
	Status string `json:"status"`
}

func NewReadinessHandler(
	checker ReadinessChecker,
) *ReadinessHandler {
	return &ReadinessHandler{
		checker: checker,
	}
}

func (h *ReadinessHandler) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(
			w,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	if h.checker == nil {
		writeReadinessResponse(
			w,
			http.StatusServiceUnavailable,
			"not-ready",
		)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		readinessTimeout,
	)
	defer cancel()

	if err := h.checker.Check(ctx); err != nil {
		writeReadinessResponse(
			w,
			http.StatusServiceUnavailable,
			"not-ready",
		)
		return
	}

	writeReadinessResponse(
		w,
		http.StatusOK,
		"ready",
	)
}

func writeReadinessResponse(
	w http.ResponseWriter,
	statusCode int,
	status string,
) {
	w.WriteHeader(statusCode)

	_ = json.NewEncoder(w).Encode(
		readinessResponse{
			Status: status,
		},
	)
}
