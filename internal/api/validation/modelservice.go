package validation

import (
	"net"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
	apirequest "github.com/anselem-okeke/ai-platform-operator/internal/api/request"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/response"
)

func ValidateCreateModelService(
	request apirequest.CreateModelServiceRequest,
	maxReplicas int,
) []response.ValidationDetail {
	var details []response.ValidationDetail

	validateName(
		request.Name,
		&details,
	)

	validateReplicas(
		request.Replicas,
		maxReplicas,
		&details,
	)

	backend := platformv1alpha1.ModelServiceBackend(
		request.Backend,
	)

	if backend == "" {
		backend = platformv1alpha1.
			ModelServiceBackendDeployment
	}

	switch backend {
	case platformv1alpha1.ModelServiceBackendDeployment:
		validateDeploymentRequest(
			request,
			&details,
		)

	case platformv1alpha1.ModelServiceBackendKServe:
		validateKServeRequest(
			request,
			&details,
		)

	default:
		details = append(
			details,
			response.ValidationDetail{
				Field: "backend",
				Message: "must be one of: " +
					"Deployment, KServe",
			},
		)
	}

	return details
}

func ValidateUpdateModelService(
	name string,
	request apirequest.UpdateModelServiceRequest,
	maxReplicas int,
) []response.ValidationDetail {
	return ValidateCreateModelService(
		apirequest.CreateModelServiceRequest{
			Name:      name,
			Backend:   request.Backend,
			Image:     request.Image,
			Replicas:  request.Replicas,
			Port:      request.Port,
			Predictor: request.Predictor,
			Exposure:  request.Exposure,
			Storage:   request.Storage,
		},
		maxReplicas,
	)
}

func validateReplicas(
	replicas int32,
	maxReplicas int,
	details *[]response.ValidationDetail,
) {
	if replicas < 1 ||
		int(replicas) > maxReplicas {
		*details = append(
			*details,
			response.ValidationDetail{
				Field: "replicas",
				Message: "must be between 1 and " +
					strconv.Itoa(maxReplicas),
			},
		)
	}
}

func validateDeploymentRequest(
	request apirequest.CreateModelServiceRequest,
	details *[]response.ValidationDetail,
) {
	if strings.TrimSpace(request.Image) == "" {
		*details = append(
			*details,
			response.ValidationDetail{
				Field:   "image",
				Message: "must not be empty",
			},
		)
	}

	if request.Port < 1 ||
		request.Port > 65535 {
		*details = append(
			*details,
			response.ValidationDetail{
				Field: "port",
				Message: "must be between 1 " +
					"and 65535",
			},
		)
	}

	if request.Predictor != nil {
		*details = append(
			*details,
			response.ValidationDetail{
				Field: "predictor",
				Message: "may only be configured " +
					"for KServe",
			},
		)
	}

	validateExposure(
		request.Exposure,
		details,
	)

	validateStorage(
		request.Storage,
		details,
	)
}

func validateKServeRequest(
	request apirequest.CreateModelServiceRequest,
	details *[]response.ValidationDetail,
) {
	if strings.TrimSpace(request.Image) != "" {
		*details = append(
			*details,
			response.ValidationDetail{
				Field: "image",
				Message: "must be omitted for " +
					"KServe",
			},
		)
	}

	if request.Port != 0 {
		*details = append(
			*details,
			response.ValidationDetail{
				Field: "port",
				Message: "must be omitted for " +
					"KServe",
			},
		)
	}

	if request.Exposure.Enabled {
		*details = append(
			*details,
			response.ValidationDetail{
				Field: "exposure.enabled",
				Message: "must be false for " +
					"KServe",
			},
		)
	}

	if request.Storage.Enabled {
		*details = append(
			*details,
			response.ValidationDetail{
				Field: "storage.enabled",
				Message: "must be false for " +
					"KServe",
			},
		)
	}

	if request.Predictor == nil {
		*details = append(
			*details,
			response.ValidationDetail{
				Field:   "predictor",
				Message: "is required for KServe",
			},
		)

		return
	}

	validatePredictor(
		*request.Predictor,
		details,
	)
}

func validatePredictor(
	predictor apirequest.PredictorRequest,
	details *[]response.ValidationDetail,
) {
	if predictor.ModelFormat.Name != "sklearn" {
		*details = append(
			*details,
			response.ValidationDetail{
				Field:   "predictor.modelFormat.name",
				Message: "must be sklearn",
			},
		)
	}

	if predictor.ModelFormat.Version != "1" {
		*details = append(
			*details,
			response.ValidationDetail{
				Field:   "predictor.modelFormat.version",
				Message: "must be 1",
			},
		)
	}

	if predictor.Runtime !=
		"kserve-sklearnserver" {
		*details = append(
			*details,
			response.ValidationDetail{
				Field: "predictor.runtime",
				Message: "must be " +
					"kserve-sklearnserver",
			},
		)
	}

	if !validS3StorageURI(
		predictor.StorageURI,
	) {
		*details = append(
			*details,
			response.ValidationDetail{
				Field: "predictor.storageUri",
				Message: "must use the form " +
					"s3://bucket/object-path",
			},
		)
	}

	if predictor.ServiceAccountName !=
		"kserve-model-reader" {
		*details = append(
			*details,
			response.ValidationDetail{
				Field: "predictor." +
					"serviceAccountName",
				Message: "must be " +
					"kserve-model-reader",
			},
		)
	}
}

func validS3StorageURI(
	storageURI string,
) bool {
	if !strings.HasPrefix(
		storageURI,
		"s3://",
	) {
		return false
	}

	location := strings.TrimPrefix(
		storageURI,
		"s3://",
	)

	parts := strings.SplitN(
		location,
		"/",
		2,
	)

	return len(parts) == 2 &&
		parts[0] != "" &&
		parts[1] != ""
}

func validateExposure(
	exposure apirequest.ExposureRequest,
	details *[]response.ValidationDetail,
) {
	if !exposure.Enabled {
		return
	}

	if strings.TrimSpace(
		exposure.Hostname,
	) == "" {
		*details = append(
			*details,
			response.ValidationDetail{
				Field: "exposure.hostname",
				Message: "is required when " +
					"exposure is enabled",
			},
		)
	} else if !validHostname(
		exposure.Hostname,
	) {
		*details = append(
			*details,
			response.ValidationDetail{
				Field: "exposure.hostname",
				Message: "must be a valid " +
					"DNS hostname",
			},
		)
	}

	if exposure.PathPrefix != "" &&
		!strings.HasPrefix(
			exposure.PathPrefix,
			"/",
		) {
		*details = append(
			*details,
			response.ValidationDetail{
				Field:   "exposure.pathPrefix",
				Message: "must begin with /",
			},
		)
	}
}

func validateStorage(
	storage apirequest.StorageRequest,
	details *[]response.ValidationDetail,
) {
	if !storage.Enabled {
		return
	}

	if strings.TrimSpace(
		storage.Size,
	) == "" {
		*details = append(
			*details,
			response.ValidationDetail{
				Field: "storage.size",
				Message: "is required when " +
					"storage is enabled",
			},
		)
	} else if _, err := resource.ParseQuantity(
		storage.Size,
	); err != nil {
		*details = append(
			*details,
			response.ValidationDetail{
				Field: "storage.size",
				Message: "must be a valid " +
					"Kubernetes quantity",
			},
		)
	}

	if !strings.HasPrefix(
		storage.MountPath,
		"/",
	) {
		*details = append(
			*details,
			response.ValidationDetail{
				Field: "storage.mountPath",
				Message: "must be an " +
					"absolute path",
			},
		)
	}
}

func validateName(
	name string,
	details *[]response.ValidationDetail,
) {
	if strings.TrimSpace(name) == "" {
		*details = append(
			*details,
			response.ValidationDetail{
				Field:   "name",
				Message: "must not be empty",
			},
		)

		return
	}

	errors := validation.IsDNS1123Label(name)

	for _, err := range errors {
		*details = append(
			*details,
			response.ValidationDetail{
				Field:   "name",
				Message: err,
			},
		)
	}
}

func validHostname(
	hostname string,
) bool {
	if len(hostname) > 253 {
		return false
	}

	return net.ParseIP(hostname) == nil &&
		len(
			validation.IsDNS1123Subdomain(
				hostname,
			),
		) == 0
}
