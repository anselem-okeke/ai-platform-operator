/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("ModelService KServe reconciliation", func() {
	const (
		resourceName      = "iris-kserve-test"
		resourceNamespace = "default"
	)

	var (
		ctx        context.Context
		key        types.NamespacedName
		reconciler *ModelServiceReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()

		key = types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}

		reconciler = &ModelServiceReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		modelService := &platformv1alpha1.ModelService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: resourceNamespace,
			},
			Spec: platformv1alpha1.ModelServiceSpec{
				Backend: platformv1alpha1.
					ModelServiceBackendKServe,
				Replicas: 1,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.
							MustParse("100m"),
						corev1.ResourceMemory: resource.
							MustParse("256Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU: resource.
							MustParse("1"),
						corev1.ResourceMemory: resource.
							MustParse("1Gi"),
					},
				},
				Predictor: &platformv1alpha1.
					ModelServicePredictor{
					ModelFormat: platformv1alpha1.
						ModelServiceModelFormat{
						Name:    "sklearn",
						Version: "1",
					},
					Runtime:            "kserve-sklearnserver",
					StorageURI:         "s3://models/sklearn/iris/v1",
					ServiceAccountName: "kserve-model-reader",
				},
			},
		}

		Expect(k8sClient.Create(ctx, modelService)).
			To(Succeed())
	})

	AfterEach(func() {
		inferenceService := newInferenceService(
			resourceName,
			resourceNamespace,
		)

		Expect(client.IgnoreNotFound(
			k8sClient.Delete(ctx, inferenceService),
		)).To(Succeed())

		modelService := &platformv1alpha1.ModelService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: resourceNamespace,
			},
		}

		Expect(client.IgnoreNotFound(
			k8sClient.Delete(ctx, modelService),
		)).To(Succeed())
	})

	reconcileResource := func() {
		_, err := reconciler.Reconcile(
			ctx,
			reconcile.Request{
				NamespacedName: key,
			},
		)
		Expect(err).NotTo(HaveOccurred())
	}

	It("creates only an owned KServe InferenceService", func() {
		reconcileResource()

		inferenceService := newInferenceService(
			resourceName,
			resourceNamespace,
		)

		Expect(k8sClient.Get(
			ctx,
			key,
			inferenceService,
		)).To(Succeed())

		modelService := &platformv1alpha1.ModelService{}
		Expect(k8sClient.Get(
			ctx,
			key,
			modelService,
		)).To(Succeed())

		Expect(metav1.IsControlledBy(
			inferenceService,
			modelService,
		)).To(BeTrue())

		storageURI, found, err := unstructured.NestedString(
			inferenceService.Object,
			"spec",
			"predictor",
			"model",
			"storageUri",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(storageURI).To(Equal(
			"s3://models/sklearn/iris/v1",
		))

		runtimeName, found, err := unstructured.NestedString(
			inferenceService.Object,
			"spec",
			"predictor",
			"model",
			"runtime",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(runtimeName).To(Equal(
			"kserve-sklearnserver",
		))

		serviceAccount, found, err :=
			unstructured.NestedString(
				inferenceService.Object,
				"spec",
				"predictor",
				"serviceAccountName",
			)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(serviceAccount).To(Equal(
			"kserve-model-reader",
		))

		deployment := &appsv1.Deployment{}
		err = k8sClient.Get(ctx, key, deployment)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		service := &corev1.Service{}
		err = k8sClient.Get(ctx, key, service)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		Expect(modelService.Status.Phase).
			To(Equal("Provisioning"))
		Expect(modelService.Status.ObservedGeneration).
			To(Equal(modelService.Generation))
	})

	It("propagates KServe readiness and failure status", func() {
		reconcileResource()

		inferenceService := newInferenceService(
			resourceName,
			resourceNamespace,
		)

		Expect(k8sClient.Get(
			ctx,
			key,
			inferenceService,
		)).To(Succeed())

		inferenceService.Object["status"] =
			map[string]any{
				"url": "http://iris-kserve-test.example.com",
				"conditions": []any{
					map[string]any{
						"type":   "Ready",
						"status": "True",
					},
				},
			}

		Expect(k8sClient.Status().Update(
			ctx,
			inferenceService,
		)).To(Succeed())

		reconcileResource()

		modelService := &platformv1alpha1.ModelService{}
		Expect(k8sClient.Get(
			ctx,
			key,
			modelService,
		)).To(Succeed())

		Expect(modelService.Status.Phase).
			To(Equal("Ready"))
		Expect(modelService.Status.ReadyReplicas).
			To(Equal(int32(1)))
		Expect(modelService.Status.Endpoint).
			To(Equal(
				"http://iris-kserve-test.example.com",
			))

		available := meta.FindStatusCondition(
			modelService.Status.Conditions,
			"Available",
		)
		Expect(available).NotTo(BeNil())
		Expect(available.Status).
			To(Equal(metav1.ConditionTrue))

		Expect(k8sClient.Get(
			ctx,
			key,
			inferenceService,
		)).To(Succeed())

		inferenceService.Object["status"] =
			map[string]any{
				"conditions": []any{
					map[string]any{
						"type":    "Ready",
						"status":  "False",
						"reason":  "ModelLoadFailed",
						"message": "model artifact could not be loaded",
					},
				},
			}

		Expect(k8sClient.Status().Update(
			ctx,
			inferenceService,
		)).To(Succeed())

		reconcileResource()

		Expect(k8sClient.Get(
			ctx,
			key,
			modelService,
		)).To(Succeed())

		Expect(modelService.Status.Phase).
			To(Equal("Failed"))

		available = meta.FindStatusCondition(
			modelService.Status.Conditions,
			"Available",
		)
		Expect(available).NotTo(BeNil())
		Expect(available.Status).
			To(Equal(metav1.ConditionFalse))
		Expect(available.Reason).
			To(Equal("ModelLoadFailed"))
	})

	It("reconciles specification updates and child drift", func() {
		reconcileResource()

		modelService := &platformv1alpha1.ModelService{}
		Expect(k8sClient.Get(
			ctx,
			key,
			modelService,
		)).To(Succeed())

		modelService.Spec.Replicas = 2
		modelService.Spec.Predictor.StorageURI =
			"s3://models/sklearn/iris/v2"

		Expect(k8sClient.Update(
			ctx,
			modelService,
		)).To(Succeed())

		inferenceService := newInferenceService(
			resourceName,
			resourceNamespace,
		)
		Expect(k8sClient.Get(
			ctx,
			key,
			inferenceService,
		)).To(Succeed())

		Expect(unstructured.SetNestedField(
			inferenceService.Object,
			"unapproved-runtime",
			"spec",
			"predictor",
			"model",
			"runtime",
		)).To(Succeed())

		Expect(k8sClient.Update(
			ctx,
			inferenceService,
		)).To(Succeed())

		reconcileResource()

		Expect(k8sClient.Get(
			ctx,
			key,
			inferenceService,
		)).To(Succeed())

		storageURI, _, err := unstructured.NestedString(
			inferenceService.Object,
			"spec",
			"predictor",
			"model",
			"storageUri",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(storageURI).To(Equal(
			"s3://models/sklearn/iris/v2",
		))

		runtimeName, _, err := unstructured.NestedString(
			inferenceService.Object,
			"spec",
			"predictor",
			"model",
			"runtime",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(runtimeName).To(Equal(
			"kserve-sklearnserver",
		))

		minimumReplicas, _, err := unstructured.NestedInt64(
			inferenceService.Object,
			"spec",
			"predictor",
			"minReplicas",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(minimumReplicas).To(Equal(int64(2)))

		maximumReplicas, _, err := unstructured.NestedInt64(
			inferenceService.Object,
			"spec",
			"predictor",
			"maxReplicas",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(maximumReplicas).To(Equal(int64(2)))
	})
})
