package handlers

import (
	"net/http"
)

type ModelServiceCollectionHandler struct {
	list   http.Handler
	create http.Handler
}

func NewModelServiceCollectionHandler(
	list http.Handler,
	create http.Handler,
) *ModelServiceCollectionHandler {
	return &ModelServiceCollectionHandler{
		list:   list,
		create: create,
	}
}

func (h *ModelServiceCollectionHandler) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	switch r.Method {
	case http.MethodGet:
		h.list.ServeHTTP(w, r)

	case http.MethodPost:
		h.create.ServeHTTP(w, r)

	default:
		w.Header().Set(
			"Allow",
			"GET, POST",
		)

		http.Error(
			w,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)
	}
}
