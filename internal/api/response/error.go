package response

type APIError struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code      string             `json:"code"`
	Message   string             `json:"message"`
	RequestID string             `json:"requestId,omitempty"`
	Details   []ValidationDetail `json:"details,omitempty"`
}

type ValidationDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}
