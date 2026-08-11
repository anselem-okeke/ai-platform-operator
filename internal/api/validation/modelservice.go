package validation

import (
	"net"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"

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

	if strings.TrimSpace(request.Image) == "" {
		details = append(
			details,
			response.ValidationDetail{
				Field:   "image",
				Message: "must not be empty",
			},
		)
	}

	if request.Replicas < 1 ||
		int(request.Replicas) > maxReplicas {
		details = append(
			details,
			response.ValidationDetail{
				Field: "replicas",
				Message: "must be between 1 and " +
					strconv.Itoa(maxReplicas),
			},
		)
	}

	if request.Port < 1 ||
		request.Port > 65535 {
		details = append(
			details,
			response.ValidationDetail{
				Field:   "port",
				Message: "must be between 1 and 65535",
			},
		)
	}

	if request.Exposure.Enabled {
		if strings.TrimSpace(
			request.Exposure.Hostname,
		) == "" {
			details = append(
				details,
				response.ValidationDetail{
					Field:   "exposure.hostname",
					Message: "is required when exposure is enabled",
				},
			)
		} else if !validHostname(
			request.Exposure.Hostname,
		) {
			details = append(
				details,
				response.ValidationDetail{
					Field:   "exposure.hostname",
					Message: "must be a valid DNS hostname",
				},
			)
		}

		if request.Exposure.PathPrefix != "" &&
			!strings.HasPrefix(
				request.Exposure.PathPrefix,
				"/",
			) {
			details = append(
				details,
				response.ValidationDetail{
					Field:   "exposure.pathPrefix",
					Message: "must begin with /",
				},
			)
		}
	}

	if request.Storage.Enabled {
		if strings.TrimSpace(
			request.Storage.Size,
		) == "" {
			details = append(
				details,
				response.ValidationDetail{
					Field:   "storage.size",
					Message: "is required when storage is enabled",
				},
			)
		} else if _, err := resource.ParseQuantity(
			request.Storage.Size,
		); err != nil {
			details = append(
				details,
				response.ValidationDetail{
					Field:   "storage.size",
					Message: "must be a valid Kubernetes quantity",
				},
			)
		}

		if !strings.HasPrefix(
			request.Storage.MountPath,
			"/",
		) {
			details = append(
				details,
				response.ValidationDetail{
					Field:   "storage.mountPath",
					Message: "must be an absolute path",
				},
			)
		}
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
			Name:     name,
			Image:    request.Image,
			Replicas: request.Replicas,
			Port:     request.Port,
			Exposure: request.Exposure,
			Storage:  request.Storage,
		},
		maxReplicas,
	)
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
		len(validation.IsDNS1123Subdomain(hostname)) == 0
}
