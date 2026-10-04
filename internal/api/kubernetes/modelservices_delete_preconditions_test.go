package kubernetes

import (
	"context"
	"testing"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type deleteOptionsRecorder struct {
	client.Client
	options client.DeleteOptions
}

func (c *deleteOptionsRecorder) Delete(_ context.Context, _ client.Object, options ...client.DeleteOption) error {
	for _, option := range options {
		option.ApplyToDelete(&c.options)
	}
	return nil
}

func TestDeleteUsesInspectedObjectPreconditions(t *testing.T) {
	recorder := &deleteOptionsRecorder{}
	store := NewModelServiceStore(recorder, "ai-platform")
	model := &platformv1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "api-model",
			UID:             "inspected-object-uid",
			ResourceVersion: "42",
		},
	}
	if err := store.Delete(context.Background(), model); err != nil {
		t.Fatal(err)
	}
	p := recorder.options.Preconditions
	if p == nil || p.UID == nil || *p.UID != model.UID || p.ResourceVersion == nil || *p.ResourceVersion != "42" {
		t.Fatalf("delete did not protect the inspected UID and resourceVersion: %#v", p)
	}
}
