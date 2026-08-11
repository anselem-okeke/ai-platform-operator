package response

import (
	"encoding/json"
	"net/http"
)

func WriteJSON(
	w http.ResponseWriter,
	statusCode int,
	value any,
) {
	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	w.WriteHeader(statusCode)

	_ = json.NewEncoder(w).Encode(value)
}
