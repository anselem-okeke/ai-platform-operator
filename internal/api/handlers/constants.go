package handlers

const (
	codeRequestTooLarge         = "REQUEST_TOO_LARGE"
	codeInvalidJSON             = "INVALID_JSON"
	codeValidationFailed        = "VALIDATION_FAILED"
	codeMethodNotAllowed        = "METHOD_NOT_ALLOWED"
	codeInvalidModelServiceName = "INVALID_MODEL_SERVICE_NAME"
	codeModelServiceNotFound    = "MODEL_SERVICE_NOT_FOUND"
	codeKubernetesUnavailable   = "KUBERNETES_UNAVAILABLE"

	messageMethodNotAllowed            = "method not allowed"
	messageModelServiceNameRequired    = "ModelService name is required"
	messageRequestBodyTooLarge         = "request body is too large"
	messageRequestBodyInvalidJSON      = "request body contains invalid JSON"
	messageRequestBodySingleJSONObject = "request body must contain one JSON object"
	messageRequestValidationFailed     = "request validation failed"
	messageUnableToLoadModelService    = "unable to load ModelService"
)
