package handlers

const (
	codeRequestTooLarge         = "REQUEST_TOO_LARGE"
	codeInvalidJSON             = "INVALID_JSON"
	codeValidationFailed        = "VALIDATION_FAILED"
	codeMethodNotAllowed        = "METHOD_NOT_ALLOWED"
	codeInvalidModelServiceName = "INVALID_MODEL_SERVICE_NAME"
	codeModelServiceNotFound    = "MODEL_SERVICE_NOT_FOUND"
	codeKubernetesUnavailable   = "KUBERNETES_UNAVAILABLE"

	messageMethodNotAllowed         = "method not allowed"
	messageModelServiceNameRequired = "ModelService name is required"
)
