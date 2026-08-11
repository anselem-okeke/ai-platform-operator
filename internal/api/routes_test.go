package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/auth"
	"github.com/anselem-okeke/ai-platform-operator/internal/api/handlers"
)

type routeTestVerifier struct {
	identities map[string]auth.Identity
	invalid    map[string]bool
}

func (v routeTestVerifier) Verify(
	_ context.Context,
	token string,
) (auth.Identity, error) {
	if v.invalid[token] {
		return auth.Identity{},
			errors.New("invalid token")
	}

	identity, ok := v.identities[token]
	if !ok {
		return auth.Identity{},
			errors.New("unknown token")
	}

	return identity, nil
}

type routeTestStore struct {
	item *platformv1alpha1.ModelService
}

func (s *routeTestStore) List(
	context.Context,
) ([]platformv1alpha1.ModelService, error) {
	if s.item == nil {
		return []platformv1alpha1.ModelService{},
			nil
	}

	return []platformv1alpha1.ModelService{
		*s.item.DeepCopy(),
	}, nil
}

func (s *routeTestStore) Get(
	_ context.Context,
	name string,
) (*platformv1alpha1.ModelService, error) {
	if s.item == nil ||
		s.item.Name != name {
		return nil,
			errors.New(
				"ModelService not found",
			)
	}

	return s.item.DeepCopy(), nil
}

func (s *routeTestStore) Create(
	_ context.Context,
	modelService *platformv1alpha1.ModelService,
) error {
	s.item = modelService.DeepCopy()
	s.item.Namespace = "ai-platform"

	return nil
}

func (s *routeTestStore) Update(
	_ context.Context,
	modelService *platformv1alpha1.ModelService,
) error {
	s.item = modelService.DeepCopy()

	return nil
}

func (s *routeTestStore) Delete(
	_ context.Context,
	modelService *platformv1alpha1.ModelService,
) error {
	if s.item != nil &&
		s.item.Name == modelService.Name {
		s.item = nil
	}

	return nil
}

func routeTestLogger() *slog.Logger {
	return slog.New(
		slog.NewJSONHandler(
			io.Discard,
			nil,
		),
	)
}

func routeTestModelService() *platformv1alpha1.ModelService {
	return &platformv1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "fraud-model",
			Namespace:  "ai-platform",
			Generation: 1,
		},
		Spec: platformv1alpha1.ModelServiceSpec{
			Image:    "example/fraud:v1",
			Replicas: 2,
			Port:     8080,
		},
	}
}

func routeTestDefaults() handlers.ModelServiceDefaults {
	return handlers.ModelServiceDefaults{
		GatewayName:               "shared-gateway",
		GatewayNamespace:          "gateway-system",
		GatewaySectionName:        "fraud-model-https",
		GatewayDataPlaneNamespace: "envoy-gateway-system",
	}
}

func newRouteTestRouter(
	store *routeTestStore,
	verifier auth.Verifier,
) http.Handler {
	logger := routeTestLogger()

	readiness :=
		handlers.NewReadinessHandler(
			nil,
		)

	list :=
		handlers.NewListModelServicesHandler(
			logger,
			store,
		)

	get :=
		handlers.NewGetModelServiceHandler(
			logger,
			store,
		)

	status :=
		handlers.NewGetModelServiceStatusHandler(
			logger,
			store,
		)

	create :=
		handlers.NewCreateModelServiceHandler(
			logger,
			store,
			10,
			routeTestDefaults(),
		)

	update :=
		handlers.NewUpdateModelServiceHandler(
			logger,
			store,
			10,
			routeTestDefaults(),
		)

	patch :=
		handlers.NewPatchModelServiceHandler(
			logger,
			store,
			10,
			routeTestDefaults(),
		)

	deleteHandler :=
		handlers.NewDeleteModelServiceHandler(
			logger,
			store,
		)

	return newRouter(
		logger,
		verifier,
		readiness,
		list,
		get,
		status,
		create,
		update,
		patch,
		deleteHandler,
	)
}

func routeTestVerifierWithRoles() routeTestVerifier {
	return routeTestVerifier{
		identities: map[string]auth.Identity{
			"viewer-token": {
				Subject:           "viewer-1",
				PreferredUsername: "viewer-user",
				Roles: []string{
					auth.RoleModelViewer,
				},
			},

			"deployer-token": {
				Subject:           "deployer-1",
				PreferredUsername: "deployer-user",
				Roles: []string{
					auth.RoleModelViewer,
					auth.RoleModelDeployer,
				},
			},

			"admin-token": {
				Subject:           "admin-1",
				PreferredUsername: "admin-user",
				Roles: []string{
					auth.RoleModelViewer,
					auth.RoleModelDeployer,
					auth.RolePlatformAdmin,
				},
			},
		},

		invalid: map[string]bool{
			"invalid-token": true,
		},
	}
}

func performRouteRequest(
	router http.Handler,
	method string,
	path string,
	token string,
	body string,
) *httptest.ResponseRecorder {
	var requestBody io.Reader

	if body != "" {
		requestBody =
			strings.NewReader(body)
	}

	request := httptest.NewRequest(
		method,
		path,
		requestBody,
	)

	if token != "" {
		request.Header.Set(
			"Authorization",
			"Bearer "+token,
		)
	}

	if body != "" {
		request.Header.Set(
			"Content-Type",
			"application/json",
		)
	}

	recorder := httptest.NewRecorder()

	router.ServeHTTP(
		recorder,
		request,
	)

	return recorder
}

func validCreateBody() string {
	return `
{
  "name": "new-model",
  "image": "example/model:v1",
  "replicas": 1,
  "port": 8080,
  "exposure": {
    "enabled": false
  },
  "storage": {
    "enabled": false
  }
}
`
}

func validUpdateBody() string {
	return `
{
  "image": "example/fraud:v2",
  "replicas": 3,
  "port": 8080,
  "exposure": {
    "enabled": false
  },
  "storage": {
    "enabled": false
  }
}
`
}

func validPatchBody() string {
	return `{"replicas":3}`
}

func TestRouterRequiresAuthentication(
	t *testing.T,
) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "list",
			method: http.MethodGet,
			path:   "/api/v1/model-services",
		},
		{
			name:   "get",
			method: http.MethodGet,
			path: "/api/v1/model-services/" +
				"fraud-model",
		},
		{
			name:   "status",
			method: http.MethodGet,
			path: "/api/v1/model-services/" +
				"fraud-model/status",
		},
		{
			name:   "create",
			method: http.MethodPost,
			path:   "/api/v1/model-services",
			body:   validCreateBody(),
		},
		{
			name:   "update",
			method: http.MethodPut,
			path: "/api/v1/model-services/" +
				"fraud-model",
			body: validUpdateBody(),
		},
		{
			name:   "patch",
			method: http.MethodPatch,
			path: "/api/v1/model-services/" +
				"fraud-model",
			body: validPatchBody(),
		},
		{
			name:   "delete",
			method: http.MethodDelete,
			path: "/api/v1/model-services/" +
				"fraud-model",
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				store := &routeTestStore{
					item: routeTestModelService(),
				}

				router :=
					newRouteTestRouter(
						store,
						routeTestVerifierWithRoles(),
					)

				recorder :=
					performRouteRequest(
						router,
						tt.method,
						tt.path,
						"",
						tt.body,
					)

				if recorder.Code !=
					http.StatusUnauthorized {
					t.Fatalf(
						"expected %d, got %d: %s",
						http.StatusUnauthorized,
						recorder.Code,
						recorder.Body.String(),
					)
				}

				if !strings.Contains(
					recorder.Body.String(),
					`"code":"UNAUTHORIZED"`,
				) {
					t.Fatalf(
						"unexpected response: %s",
						recorder.Body.String(),
					)
				}
			},
		)
	}
}

func TestRouterRejectsInvalidToken(
	t *testing.T,
) {
	store := &routeTestStore{
		item: routeTestModelService(),
	}

	router :=
		newRouteTestRouter(
			store,
			routeTestVerifierWithRoles(),
		)

	recorder :=
		performRouteRequest(
			router,
			http.MethodGet,
			"/api/v1/model-services",
			"invalid-token",
			"",
		)

	if recorder.Code !=
		http.StatusUnauthorized {
		t.Fatalf(
			"expected %d, got %d",
			http.StatusUnauthorized,
			recorder.Code,
		)
	}

	if !strings.Contains(
		recorder.Body.String(),
		`"code":"UNAUTHORIZED"`,
	) {
		t.Fatalf(
			"unexpected response: %s",
			recorder.Body.String(),
		)
	}
}

func TestRouterViewerAuthorization(
	t *testing.T,
) {
	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		expected int
	}{
		{
			name:     "list allowed",
			method:   http.MethodGet,
			path:     "/api/v1/model-services",
			expected: http.StatusOK,
		},
		{
			name:   "get allowed",
			method: http.MethodGet,
			path: "/api/v1/model-services/" +
				"fraud-model",
			expected: http.StatusOK,
		},
		{
			name:   "status allowed",
			method: http.MethodGet,
			path: "/api/v1/model-services/" +
				"fraud-model/status",
			expected: http.StatusOK,
		},
		{
			name:     "create forbidden",
			method:   http.MethodPost,
			path:     "/api/v1/model-services",
			body:     validCreateBody(),
			expected: http.StatusForbidden,
		},
		{
			name:   "update forbidden",
			method: http.MethodPut,
			path: "/api/v1/model-services/" +
				"fraud-model",
			body:     validUpdateBody(),
			expected: http.StatusForbidden,
		},
		{
			name:   "patch forbidden",
			method: http.MethodPatch,
			path: "/api/v1/model-services/" +
				"fraud-model",
			body:     validPatchBody(),
			expected: http.StatusForbidden,
		},
		{
			name:   "delete forbidden",
			method: http.MethodDelete,
			path: "/api/v1/model-services/" +
				"fraud-model",
			expected: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				store := &routeTestStore{
					item: routeTestModelService(),
				}

				router :=
					newRouteTestRouter(
						store,
						routeTestVerifierWithRoles(),
					)

				recorder :=
					performRouteRequest(
						router,
						tt.method,
						tt.path,
						"viewer-token",
						tt.body,
					)

				if recorder.Code != tt.expected {
					t.Fatalf(
						"expected %d, got %d: %s",
						tt.expected,
						recorder.Code,
						recorder.Body.String(),
					)
				}

				if tt.expected ==
					http.StatusForbidden &&
					!strings.Contains(
						recorder.Body.String(),
						`"code":"FORBIDDEN"`,
					) {
					t.Fatalf(
						"unexpected response: %s",
						recorder.Body.String(),
					)
				}
			},
		)
	}
}

func TestRouterDeployerAuthorization(
	t *testing.T,
) {
	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		expected int
	}{
		{
			name:     "list allowed",
			method:   http.MethodGet,
			path:     "/api/v1/model-services",
			expected: http.StatusOK,
		},
		{
			name:     "create allowed",
			method:   http.MethodPost,
			path:     "/api/v1/model-services",
			body:     validCreateBody(),
			expected: http.StatusCreated,
		},
		{
			name:   "update allowed",
			method: http.MethodPut,
			path: "/api/v1/model-services/" +
				"fraud-model",
			body:     validUpdateBody(),
			expected: http.StatusOK,
		},
		{
			name:   "patch allowed",
			method: http.MethodPatch,
			path: "/api/v1/model-services/" +
				"fraud-model",
			body:     validPatchBody(),
			expected: http.StatusOK,
		},
		{
			name:   "delete forbidden",
			method: http.MethodDelete,
			path: "/api/v1/model-services/" +
				"fraud-model",
			expected: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				store := &routeTestStore{
					item: routeTestModelService(),
				}

				router :=
					newRouteTestRouter(
						store,
						routeTestVerifierWithRoles(),
					)

				recorder :=
					performRouteRequest(
						router,
						tt.method,
						tt.path,
						"deployer-token",
						tt.body,
					)

				if recorder.Code != tt.expected {
					t.Fatalf(
						"expected %d, got %d: %s",
						tt.expected,
						recorder.Code,
						recorder.Body.String(),
					)
				}
			},
		)
	}
}

func TestRouterAdminAuthorization(
	t *testing.T,
) {
	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		expected int
	}{
		{
			name:     "list allowed",
			method:   http.MethodGet,
			path:     "/api/v1/model-services",
			expected: http.StatusOK,
		},
		{
			name:     "create allowed",
			method:   http.MethodPost,
			path:     "/api/v1/model-services",
			body:     validCreateBody(),
			expected: http.StatusCreated,
		},
		{
			name:   "update allowed",
			method: http.MethodPut,
			path: "/api/v1/model-services/" +
				"fraud-model",
			body:     validUpdateBody(),
			expected: http.StatusOK,
		},
		{
			name:   "patch allowed",
			method: http.MethodPatch,
			path: "/api/v1/model-services/" +
				"fraud-model",
			body:     validPatchBody(),
			expected: http.StatusOK,
		},
		{
			name:   "delete allowed",
			method: http.MethodDelete,
			path: "/api/v1/model-services/" +
				"fraud-model",
			expected: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				store := &routeTestStore{
					item: routeTestModelService(),
				}

				router :=
					newRouteTestRouter(
						store,
						routeTestVerifierWithRoles(),
					)

				recorder :=
					performRouteRequest(
						router,
						tt.method,
						tt.path,
						"admin-token",
						tt.body,
					)

				if recorder.Code != tt.expected {
					t.Fatalf(
						"expected %d, got %d: %s",
						tt.expected,
						recorder.Code,
						recorder.Body.String(),
					)
				}
			},
		)
	}
}
