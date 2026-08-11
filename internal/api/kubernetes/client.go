package kubernetes

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
)

type Clients struct {
	Client client.Client
	Scheme *runtime.Scheme
}

func NewClient() (*Clients, error) {
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf(
			"load Kubernetes configuration: %w",
			err,
		)
	}

	scheme := runtime.NewScheme()

	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf(
			"add Kubernetes core scheme: %w",
			err,
		)
	}

	if err := platformv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf(
			"add ModelService scheme: %w",
			err,
		)
	}

	k8sClient, err := client.New(
		restConfig,
		client.Options{
			Scheme: scheme,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create Kubernetes client: %w",
			err,
		)
	}

	return &Clients{
		Client: k8sClient,
		Scheme: scheme,
	}, nil
}
