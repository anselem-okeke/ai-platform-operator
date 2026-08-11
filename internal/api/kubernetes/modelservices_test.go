package kubernetes

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
)

func TestModelServiceStoreList(
	t *testing.T,
) {
	scheme := runtime.NewScheme()

	if err := platformv1alpha1.AddToScheme(
		scheme,
	); err != nil {
		t.Fatalf(
			"AddToScheme() returned error: %v",
			err,
		)
	}

	inScope := &platformv1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fraud-model",
			Namespace: "ai-platform",
		},
	}

	outOfScope := &platformv1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-model",
			Namespace: "default",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			inScope,
			outOfScope,
		).
		Build()

	store := NewModelServiceStore(
		k8sClient,
		"ai-platform",
	)

	items, err := store.List(
		context.Background(),
	)
	if err != nil {
		t.Fatalf(
			"List() returned error: %v",
			err,
		)
	}

	if len(items) != 1 {
		t.Fatalf(
			"expected 1 ModelService, got %d",
			len(items),
		)
	}

	if items[0].Name != "fraud-model" {
		t.Fatalf(
			"expected fraud-model, got %q",
			items[0].Name,
		)
	}
}

func TestModelServiceStoreGet(
	t *testing.T,
) {
	scheme := runtime.NewScheme()

	if err := platformv1alpha1.AddToScheme(
		scheme,
	); err != nil {
		t.Fatalf(
			"AddToScheme() returned error: %v",
			err,
		)
	}

	modelService := &platformv1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fraud-model",
			Namespace: "ai-platform",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(modelService).
		Build()

	store := NewModelServiceStore(
		k8sClient,
		"ai-platform",
	)

	item, err := store.Get(
		context.Background(),
		"fraud-model",
	)
	if err != nil {
		t.Fatalf(
			"Get() returned error: %v",
			err,
		)
	}

	if item.Name != "fraud-model" {
		t.Fatalf(
			"expected fraud-model, got %q",
			item.Name,
		)
	}

	if item.Namespace != "ai-platform" {
		t.Fatalf(
			"expected ai-platform namespace, got %q",
			item.Namespace,
		)
	}
}

func TestModelServiceStoreCreate(
	t *testing.T,
) {
	scheme := runtime.NewScheme()

	if err := platformv1alpha1.AddToScheme(
		scheme,
	); err != nil {
		t.Fatalf(
			"AddToScheme() returned error: %v",
			err,
		)
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	store := NewModelServiceStore(
		k8sClient,
		"ai-platform",
	)

	modelService := &platformv1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fraud-model",
			Namespace: "attacker-namespace",
		},
		Spec: platformv1alpha1.ModelServiceSpec{
			Image:    "example/fraud:v1",
			Replicas: 2,
			Port:     8080,
		},
	}

	if err := store.Create(
		context.Background(),
		modelService,
	); err != nil {
		t.Fatalf(
			"Create() returned error: %v",
			err,
		)
	}

	if modelService.Namespace != "ai-platform" {
		t.Fatalf(
			"expected input namespace to be forced to ai-platform, got %q",
			modelService.Namespace,
		)
	}

	created := &platformv1alpha1.ModelService{}

	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{
			Name:      "fraud-model",
			Namespace: "ai-platform",
		},
		created,
	); err != nil {
		t.Fatalf(
			"failed to get created ModelService: %v",
			err,
		)
	}

	if created.Namespace != "ai-platform" {
		t.Fatalf(
			"expected stored namespace ai-platform, got %q",
			created.Namespace,
		)
	}

	if created.Spec.Image != "example/fraud:v1" {
		t.Fatalf(
			"expected image example/fraud:v1, got %q",
			created.Spec.Image,
		)
	}
}

func TestModelServiceStoreUpdate(
	t *testing.T,
) {
	scheme := runtime.NewScheme()

	if err := platformv1alpha1.AddToScheme(
		scheme,
	); err != nil {
		t.Fatalf(
			"AddToScheme() returned error: %v",
			err,
		)
	}

	existing := &platformv1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fraud-model",
			Namespace: "ai-platform",
		},
		Spec: platformv1alpha1.ModelServiceSpec{
			Image:    "example/fraud:v1",
			Replicas: 2,
			Port:     8080,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	store := NewModelServiceStore(
		k8sClient,
		"ai-platform",
	)

	modelService, err := store.Get(
		context.Background(),
		"fraud-model",
	)
	if err != nil {
		t.Fatalf(
			"Get() returned error: %v",
			err,
		)
	}

	modelService.Spec.Image =
		"example/fraud:v2"

	modelService.Spec.Replicas = 3

	modelService.Namespace =
		"attacker-namespace"

	if err := store.Update(
		context.Background(),
		modelService,
	); err != nil {
		t.Fatalf(
			"Update() returned error: %v",
			err,
		)
	}

	if modelService.Namespace != "ai-platform" {
		t.Fatalf(
			"expected namespace to be forced to ai-platform, got %q",
			modelService.Namespace,
		)
	}

	updated := &platformv1alpha1.ModelService{}

	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{
			Name:      "fraud-model",
			Namespace: "ai-platform",
		},
		updated,
	); err != nil {
		t.Fatalf(
			"failed to get updated ModelService: %v",
			err,
		)
	}

	if updated.Spec.Image !=
		"example/fraud:v2" {
		t.Fatalf(
			"expected image example/fraud:v2, got %q",
			updated.Spec.Image,
		)
	}

	if updated.Spec.Replicas != 3 {
		t.Fatalf(
			"expected replicas 3, got %d",
			updated.Spec.Replicas,
		)
	}
}

func TestModelServiceStoreDelete(
	t *testing.T,
) {
	scheme := runtime.NewScheme()

	if err := platformv1alpha1.AddToScheme(
		scheme,
	); err != nil {
		t.Fatalf(
			"AddToScheme() returned error: %v",
			err,
		)
	}

	existing := &platformv1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fraud-model",
			Namespace: "ai-platform",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	store := NewModelServiceStore(
		k8sClient,
		"ai-platform",
	)

	modelService, err := store.Get(
		context.Background(),
		"fraud-model",
	)
	if err != nil {
		t.Fatalf(
			"Get() returned error: %v",
			err,
		)
	}

	modelService.Namespace =
		"attacker-namespace"

	if err := store.Delete(
		context.Background(),
		modelService,
	); err != nil {
		t.Fatalf(
			"Delete() returned error: %v",
			err,
		)
	}

	if modelService.Namespace != "ai-platform" {
		t.Fatalf(
			"expected namespace to be forced to ai-platform, got %q",
			modelService.Namespace,
		)
	}

	deleted := &platformv1alpha1.ModelService{}

	err = k8sClient.Get(
		context.Background(),
		client.ObjectKey{
			Name:      "fraud-model",
			Namespace: "ai-platform",
		},
		deleted,
	)

	if !apierrors.IsNotFound(err) {
		t.Fatalf(
			"expected ModelService to be deleted, got error: %v",
			err,
		)
	}
}

func TestModelServiceStoreCreateDuplicate(
	t *testing.T,
) {
	scheme := runtime.NewScheme()

	if err := platformv1alpha1.AddToScheme(
		scheme,
	); err != nil {
		t.Fatalf(
			"AddToScheme() returned error: %v",
			err,
		)
	}

	existing := &platformv1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fraud-model",
			Namespace: "ai-platform",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	store := NewModelServiceStore(
		k8sClient,
		"ai-platform",
	)

	duplicate := &platformv1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fraud-model",
			Namespace: "default",
		},
	}

	err := store.Create(
		context.Background(),
		duplicate,
	)

	if err == nil {
		t.Fatal(
			"expected duplicate Create() to return an error",
		)
	}

	if !apierrors.IsAlreadyExists(err) {
		t.Fatalf(
			"expected AlreadyExists error, got: %v",
			err,
		)
	}
}

func TestModelServiceStoreDeleteMissing(
	t *testing.T,
) {
	scheme := runtime.NewScheme()

	if err := platformv1alpha1.AddToScheme(
		scheme,
	); err != nil {
		t.Fatalf(
			"AddToScheme() returned error: %v",
			err,
		)
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	store := NewModelServiceStore(
		k8sClient,
		"ai-platform",
	)

	modelService := &platformv1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-model",
			Namespace: "default",
		},
	}

	err := store.Delete(
		context.Background(),
		modelService,
	)

	if err == nil {
		t.Fatal(
			"expected Delete() to return an error",
		)
	}

	if !apierrors.IsNotFound(err) {
		t.Fatalf(
			"expected NotFound error, got: %v",
			err,
		)
	}
}
