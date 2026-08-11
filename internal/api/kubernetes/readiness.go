package kubernetes

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
)

type ReadinessChecker struct {
	client    client.Client
	namespace string
}

func NewReadinessChecker(
	k8sClient client.Client,
	namespace string,
) *ReadinessChecker {
	return &ReadinessChecker{
		client:    k8sClient,
		namespace: namespace,
	}
}

func (c *ReadinessChecker) Check(
	ctx context.Context,
) error {
	var modelServices platformv1alpha1.ModelServiceList

	if err := c.client.List(
		ctx,
		&modelServices,
		client.InNamespace(c.namespace),
		client.Limit(1),
	); err != nil {
		return fmt.Errorf(
			"list ModelServices in namespace %q: %w",
			c.namespace,
			err,
		)
	}

	return nil
}
