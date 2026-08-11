package kubernetes

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
)

type ModelServiceStore struct {
	client    client.Client
	namespace string
}

func NewModelServiceStore(
	k8sClient client.Client,
	namespace string,
) *ModelServiceStore {
	return &ModelServiceStore{
		client:    k8sClient,
		namespace: namespace,
	}
}

func (s *ModelServiceStore) List(
	ctx context.Context,
) ([]platformv1alpha1.ModelService, error) {
	var modelServices platformv1alpha1.ModelServiceList

	if err := s.client.List(
		ctx,
		&modelServices,
		client.InNamespace(s.namespace),
	); err != nil {
		return nil, fmt.Errorf(
			"list ModelServices in namespace %q: %w",
			s.namespace,
			err,
		)
	}

	return modelServices.Items, nil
}

func (s *ModelServiceStore) Get(
	ctx context.Context,
	name string,
) (*platformv1alpha1.ModelService, error) {
	var modelService platformv1alpha1.ModelService

	if err := s.client.Get(
		ctx,
		types.NamespacedName{
			Namespace: s.namespace,
			Name:      name,
		},
		&modelService,
	); err != nil {
		return nil, err
	}

	return &modelService, nil
}

func (s *ModelServiceStore) Create(
	ctx context.Context,
	modelService *platformv1alpha1.ModelService,
) error {
	modelService.Namespace = s.namespace

	if err := s.client.Create(
		ctx,
		modelService,
	); err != nil {
		return fmt.Errorf(
			"create ModelService %q in namespace %q: %w",
			modelService.Name,
			s.namespace,
			err,
		)
	}

	return nil
}

func (s *ModelServiceStore) Update(
	ctx context.Context,
	modelService *platformv1alpha1.ModelService,
) error {
	modelService.Namespace = s.namespace

	if err := s.client.Update(
		ctx,
		modelService,
	); err != nil {
		return fmt.Errorf(
			"update ModelService %q in namespace %q: %w",
			modelService.Name,
			s.namespace,
			err,
		)
	}

	return nil
}

func (s *ModelServiceStore) Delete(
	ctx context.Context,
	modelService *platformv1alpha1.ModelService,
) error {
	modelService.Namespace = s.namespace

	if err := s.client.Delete(
		ctx,
		modelService,
	); err != nil {
		return fmt.Errorf(
			"delete ModelService %q in namespace %q: %w",
			modelService.Name,
			s.namespace,
			err,
		)
	}

	return nil
}
