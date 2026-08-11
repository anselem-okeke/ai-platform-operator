package auth

import "testing"

func TestIdentityHasRole(t *testing.T) {
	identity := Identity{
		Roles: []string{
			"model-viewer",
			"model-deployer",
		},
	}

	if !identity.HasRole(
		"model-viewer",
	) {
		t.Fatal(
			"expected model-viewer role",
		)
	}

	if identity.HasRole(
		"platform-admin",
	) {
		t.Fatal(
			"did not expect platform-admin role",
		)
	}
}
