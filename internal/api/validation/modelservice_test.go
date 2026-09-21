package validation

import (
	"testing"

	apirequest "github.com/anselem-okeke/ai-platform-operator/internal/api/request"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/response"
)

func TestValidateCreateDeploymentDefaults(
	t *testing.T,
) {
	request := validDeploymentRequest()

	details := ValidateCreateModelService(
		request,
		10,
	)

	if len(details) != 0 {
		t.Fatalf(
			"expected valid Deployment request, got %#v",
			details,
		)
	}
}

func TestValidateCreateKServe(
	t *testing.T,
) {
	request := validKServeRequest()

	details := ValidateCreateModelService(
		request,
		10,
	)

	if len(details) != 0 {
		t.Fatalf(
			"expected valid KServe request, got %#v",
			details,
		)
	}
}

func TestValidateCreateKServeRequiresPredictor(
	t *testing.T,
) {
	request := validKServeRequest()
	request.Predictor = nil

	details := ValidateCreateModelService(
		request,
		10,
	)

	assertValidationFields(
		t,
		details,
		"predictor",
	)
}

func TestValidateCreateKServeRejectsDeploymentFields(
	t *testing.T,
) {
	request := validKServeRequest()
	request.Image = "example/model:v1"
	request.Port = 8080
	request.Exposure.Enabled = true
	request.Storage.Enabled = true

	details := ValidateCreateModelService(
		request,
		10,
	)

	assertValidationFields(
		t,
		details,
		"image",
		"port",
		"exposure.enabled",
		"storage.enabled",
	)
}

func TestValidateCreateKServePredictorFields(
	t *testing.T,
) {
	request := validKServeRequest()
	request.Predictor.ModelFormat.Name = "pytorch"
	request.Predictor.ModelFormat.Version = "2"
	request.Predictor.Runtime = "custom-runtime"
	request.Predictor.StorageURI = "https://example.com/model"
	request.Predictor.ServiceAccountName = "default"

	details := ValidateCreateModelService(
		request,
		10,
	)

	assertValidationFields(
		t,
		details,
		"predictor.modelFormat.name",
		"predictor.modelFormat.version",
		"predictor.runtime",
		"predictor.storageUri",
		"predictor.serviceAccountName",
	)
}

func TestValidateCreateDeploymentRejectsPredictor(
	t *testing.T,
) {
	request := validDeploymentRequest()
	request.Predictor =
		validKServeRequest().Predictor

	details := ValidateCreateModelService(
		request,
		10,
	)

	assertValidationFields(
		t,
		details,
		"predictor",
	)
}

func TestValidateCreateRejectsUnknownBackend(
	t *testing.T,
) {
	request := validDeploymentRequest()
	request.Backend = "Unknown"

	details := ValidateCreateModelService(
		request,
		10,
	)

	assertValidationFields(
		t,
		details,
		"backend",
	)
}

func TestValidateUpdateKServe(
	t *testing.T,
) {
	createRequest := validKServeRequest()

	request := apirequest.UpdateModelServiceRequest{
		Backend:   createRequest.Backend,
		Replicas:  createRequest.Replicas,
		Predictor: createRequest.Predictor,
	}

	details := ValidateUpdateModelService(
		"iris-api",
		request,
		10,
	)

	if len(details) != 0 {
		t.Fatalf(
			"expected valid KServe update, got %#v",
			details,
		)
	}
}

func validDeploymentRequest() apirequest.CreateModelServiceRequest {
	return apirequest.CreateModelServiceRequest{
		Name:     "fraud-model",
		Image:    "example/fraud:v1",
		Replicas: 2,
		Port:     8080,
	}
}

func validKServeRequest() apirequest.CreateModelServiceRequest {
	return apirequest.CreateModelServiceRequest{
		Name:     "iris-api",
		Backend:  "KServe",
		Replicas: 1,
		Predictor: &apirequest.PredictorRequest{
			ModelFormat: apirequest.ModelFormatRequest{
				Name:    "sklearn",
				Version: "1",
			},
			Runtime: "kserve-sklearnserver",
			StorageURI: "s3://models/sklearn/" +
				"iris/v1",
			ServiceAccountName: "kserve-model-reader",
		},
	}
}

func assertValidationFields(
	t *testing.T,
	details []response.ValidationDetail,
	expected ...string,
) {
	t.Helper()

	actual := make(
		map[string]bool,
		len(details),
	)

	for _, detail := range details {
		actual[detail.Field] = true
	}

	for _, field := range expected {
		if !actual[field] {
			t.Fatalf(
				"expected validation field %q, got %#v",
				field,
				details,
			)
		}
	}
}
