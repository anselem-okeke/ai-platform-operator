package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	apimetrics "github.com/anselem-okeke/ai-platform-operator/internal/api/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRequestMetricsRecordsRequest(
	t *testing.T,
) {
	const (
		routePattern = "/api/v1/model-services/{name}"
		status       = "201"
	)

	counter :=
		apimetrics.HTTPRequestsTotal.
			WithLabelValues(
				http.MethodPost,
				routePattern,
				status,
			)

	before := testutil.ToFloat64(counter)

	mux := http.NewServeMux()

	mux.HandleFunc(
		routePattern,
		func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			w.WriteHeader(http.StatusCreated)
		},
	)

	handler := RequestMetrics(mux)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/model-services/test-model",
		nil,
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusCreated {
		t.Fatalf(
			"expected %d, got %d",
			http.StatusCreated,
			recorder.Code,
		)
	}

	after := testutil.ToFloat64(counter)

	if after != before+1 {
		t.Fatalf(
			"expected request counter to increase by 1: before=%f after=%f",
			before,
			after,
		)
	}

	inFlight := testutil.ToFloat64(
		apimetrics.HTTPRequestsInFlight,
	)

	if inFlight != 0 {
		t.Fatalf(
			"expected zero in-flight requests after completion, got %f",
			inFlight,
		)
	}
}

func TestRequestMetricsSkipsMetricsEndpoint(
	t *testing.T,
) {
	const (
		routePattern = "/metrics"
		status       = "200"
	)

	counter :=
		apimetrics.HTTPRequestsTotal.
			WithLabelValues(
				http.MethodGet,
				routePattern,
				status,
			)

	before := testutil.ToFloat64(counter)

	handler := RequestMetrics(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				w.WriteHeader(http.StatusOK)
			},
		),
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/metrics",
		nil,
	)

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(
		recorder,
		request,
	)

	after := testutil.ToFloat64(counter)

	if after != before {
		t.Fatalf(
			"expected /metrics request not to change API request counter: before=%f after=%f",
			before,
			after,
		)
	}
}

func TestMetricRouteNormalizesModelServiceStatus(
	t *testing.T,
) {
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/model-services/fraud-model/status",
		nil,
	)

	actual := metricRoute(request)

	const expected = "/api/v1/model-services/{name}/status"

	if actual != expected {
		t.Fatalf(
			"expected route %q, got %q",
			expected,
			actual,
		)
	}
}
