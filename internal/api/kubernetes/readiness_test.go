package kubernetes

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
)

func TestReadinessChecker(t *testing.T) {
	scheme := runtime.NewScheme()

	if err := platformv1alpha1.AddToScheme(
		scheme,
	); err != nil {
		t.Fatalf(
			"ModelService AddToScheme() returned error: %v",
			err,
		)
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	checker := NewReadinessChecker(
		k8sClient,
		"ai-platform",
	)

	if err := checker.Check(
		context.Background(),
	); err != nil {
		t.Fatalf(
			"Check() returned error: %v",
			err,
		)
	}
}
