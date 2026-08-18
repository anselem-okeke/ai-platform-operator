package handlers

import (
	"net/http"
)

type ModelServiceResourceHandler struct {
	get    http.Handler
	update http.Handler
	patch  http.Handler
	delete http.Handler
}

func NewModelServiceResourceHandler(
	get http.Handler,
	update http.Handler,
	patch http.Handler,
	deleteHandler http.Handler,
) *ModelServiceResourceHandler {
	return &ModelServiceResourceHandler{
		get:    get,
		update: update,
		patch:  patch,
		delete: deleteHandler,
	}
}

func (h *ModelServiceResourceHandler) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	switch r.Method {
	case http.MethodGet:
		h.get.ServeHTTP(
			w,
			r,
		)

	case http.MethodPut:
		h.update.ServeHTTP(
			w,
			r,
		)

	case http.MethodPatch:
		h.patch.ServeHTTP(
			w,
			r,
		)

	case http.MethodDelete:
		h.delete.ServeHTTP(
			w,
			r,
		)

	default:
		w.Header().Set(
			"Allow",
			"GET, PUT",
		)

		http.Error(
			w,
			messageMethodNotAllowed,
			http.StatusMethodNotAllowed,
		)
	}
}
