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
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var inferenceServiceGVK = schema.GroupVersionKind{
	Group:   "serving.kserve.io",
	Version: "v1beta1",
	Kind:    "InferenceService",
}

const (
	defaultKServeRuntime        = "kserve-sklearnserver"
	defaultKServeModelVersion   = "1"
	defaultKServeServiceAccount = "kserve-model-reader"
	kserveFieldManager          = "ai-platform-operator"
)

// backendForModelService preserves the existing Deployment behavior when
// spec.backend is absent on an older ModelService.
func backendForModelService(
	modelService *platformv1alpha1.ModelService,
) platformv1alpha1.ModelServiceBackend {
	if modelService.Spec.Backend == "" {
		return platformv1alpha1.ModelServiceBackendDeployment
	}

	return modelService.Spec.Backend
}

// newInferenceService returns an unstructured KServe InferenceService with
// the correct API identity and namespaced object identity.
func newInferenceService(
	name string,
	namespace string,
) *unstructured.Unstructured {
	inferenceService := &unstructured.Unstructured{}
	inferenceService.SetGroupVersionKind(inferenceServiceGVK)
	inferenceService.SetName(name)
	inferenceService.SetNamespace(namespace)

	return inferenceService
}

// desiredInferenceServiceSpec translates the platform ModelService contract
// into the narrow KServe contract supported during Phase 8.
func desiredInferenceServiceSpec(
	modelService *platformv1alpha1.ModelService,
) (map[string]any, error) {
	predictor := modelService.Spec.Predictor

	if predictor == nil {
		return nil, fmt.Errorf(
			"spec.predictor is required for the KServe backend",
		)
	}

	runtimeName := predictor.Runtime
	if runtimeName == "" {
		runtimeName = defaultKServeRuntime
	}

	modelVersion := predictor.ModelFormat.Version
	if modelVersion == "" {
		modelVersion = defaultKServeModelVersion
	}

	serviceAccountName := predictor.ServiceAccountName
	if serviceAccountName == "" {
		serviceAccountName = defaultKServeServiceAccount
	}

	replicas := max(modelService.Spec.Replicas, 1)

	model := map[string]any{
		"modelFormat": map[string]any{
			"name":    predictor.ModelFormat.Name,
			"version": modelVersion,
		},
		"runtime":    runtimeName,
		"storageUri": predictor.StorageURI,
	}

	if len(modelService.Spec.Resources.Requests) > 0 ||
		len(modelService.Spec.Resources.Limits) > 0 {

		encodedResources, err := json.Marshal(
			modelService.Spec.Resources,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"marshal predictor resources: %w",
				err,
			)
		}

		resources := map[string]any{}
		if err := json.Unmarshal(
			encodedResources,
			&resources,
		); err != nil {
			return nil, fmt.Errorf(
				"convert predictor resources: %w",
				err,
			)
		}

		model["resources"] = resources
	}

	return map[string]any{
		"predictor": map[string]any{
			"serviceAccountName":           serviceAccountName,
			"automountServiceAccountToken": false,
			"minReplicas":                  int64(replicas),
			"maxReplicas":                  int64(replicas),
			"model":                        model,
		},
	}, nil
}

// reconcileKServeModelService creates or updates the KServe InferenceService
// owned by the parent ModelService.
func (r *ModelServiceReconciler) reconcileKServeModelService(
	ctx context.Context,
	modelService *platformv1alpha1.ModelService,
) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	desiredSpec, err := desiredInferenceServiceSpec(modelService)
	if err != nil {
		return ctrl.Result{}, err
	}

	inferenceService := newInferenceService(
		modelService.Name,
		modelService.Namespace,
	)

	inferenceService.SetLabels(
		labelsForModelService(modelService),
	)
	inferenceService.Object["spec"] = desiredSpec

	if err := controllerutil.SetControllerReference(
		modelService,
		inferenceService,
		r.Scheme,
	); err != nil {
		return ctrl.Result{}, err
	}

	applyPayload, err := json.Marshal(inferenceService.Object)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf(
			"marshal InferenceService apply configuration: %w",
			err,
		)
	}

	if err := r.Patch(
		ctx,
		inferenceService,
		client.RawPatch(types.ApplyPatchType, applyPayload),
		client.FieldOwner(kserveFieldManager),
		client.ForceOwnership,
	); err != nil {
		return ctrl.Result{}, fmt.Errorf(
			"apply KServe InferenceService: %w",
			err,
		)
	}

	if err := r.Get(
		ctx,
		client.ObjectKeyFromObject(inferenceService),
		inferenceService,
	); err != nil {
		return ctrl.Result{}, fmt.Errorf(
			"read applied KServe InferenceService: %w",
			err,
		)
	}

	logger.Info(
		"applied KServe InferenceService",
		"modelService",
		client.ObjectKeyFromObject(modelService),
		"inferenceService",
		client.ObjectKeyFromObject(inferenceService),
	)

	if err := r.updateModelServiceStatusFromInferenceService(
		ctx,
		modelService,
		inferenceService,
	); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// updateModelServiceStatusFromInferenceService maps KServe readiness and URL
// into the platform ModelService status contract.
func (r *ModelServiceReconciler) updateModelServiceStatusFromInferenceService(
	ctx context.Context,
	modelService *platformv1alpha1.ModelService,
	inferenceService *unstructured.Unstructured,
) error {
	previousStatus := modelService.Status.DeepCopy()

	replicas := max(modelService.Spec.Replicas, 1)

	modelService.Status.ObservedGeneration = modelService.Generation
	modelService.Status.ReadyReplicas = 0

	endpoint, _, err := unstructured.NestedString(
		inferenceService.Object,
		"status",
		"url",
	)
	if err != nil {
		return fmt.Errorf(
			"read InferenceService status URL: %w",
			err,
		)
	}

	modelService.Status.Endpoint = endpoint

	readyStatus, reason, message, found, err :=
		inferenceServiceReadyCondition(inferenceService)
	if err != nil {
		return err
	}

	switch {
	case !found:
		modelService.Status.Phase = modelServicePhaseProvisioning
		setModelServiceCondition(
			modelService,
			metav1.ConditionFalse,
			"InferenceServicePending",
			"Waiting for KServe InferenceService status",
		)

	case readyStatus == metav1.ConditionTrue:
		modelService.Status.Phase = modelServicePhaseReady
		modelService.Status.ReadyReplicas = replicas

		setModelServiceCondition(
			modelService,
			metav1.ConditionTrue,
			"InferenceServiceReady",
			kserveConditionMessage(
				reason,
				message,
				"KServe InferenceService is ready",
			),
		)

	case readyStatus == metav1.ConditionFalse &&
		kserveConditionFailed(reason):
		modelService.Status.Phase = modelServicePhaseFailed

		setModelServiceCondition(
			modelService,
			metav1.ConditionFalse,
			"InferenceServiceFailed",
			kserveConditionMessage(
				reason,
				message,
				"KServe InferenceService failed",
			),
		)

	case readyStatus == metav1.ConditionFalse:
		modelService.Status.Phase = modelServicePhaseProvisioning

		setModelServiceCondition(
			modelService,
			metav1.ConditionFalse,
			"InferenceServiceNotReady",
			kserveConditionMessage(
				reason,
				message,
				"KServe InferenceService is not ready",
			),
		)

	default:
		modelService.Status.Phase = modelServicePhaseProvisioning

		setModelServiceCondition(
			modelService,
			metav1.ConditionFalse,
			"InferenceServiceProgressing",
			kserveConditionMessage(
				reason,
				message,
				"KServe InferenceService is progressing",
			),
		)
	}

	if reflect.DeepEqual(*previousStatus, modelService.Status) {
		return nil
	}

	return r.Status().Update(ctx, modelService)
}

// kserveConditionFailed identifies explicit KServe failure reasons.
// Other Ready=False conditions are treated as normal provisioning.
func kserveConditionFailed(reason string) bool {
	normalized := strings.ToLower(reason)

	return strings.Contains(normalized, "failed") ||
		strings.Contains(normalized, "error") ||
		strings.Contains(normalized, "invalid")
}

// kserveConditionMessage preserves KServe's diagnostic details while the
// platform publishes a stable Kubernetes-compatible condition reason.
func kserveConditionMessage(
	reason string,
	message string,
	fallback string,
) string {
	switch {
	case reason != "" && message != "":
		return fmt.Sprintf("%s: %s", reason, message)
	case message != "":
		return message
	case reason != "":
		return reason
	default:
		return fallback
	}
}

// inferenceServiceReadyCondition returns KServe's top-level Ready condition.
func inferenceServiceReadyCondition(
	inferenceService *unstructured.Unstructured,
) (
	metav1.ConditionStatus,
	string,
	string,
	bool,
	error,
) {
	conditions, found, err := unstructured.NestedSlice(
		inferenceService.Object,
		"status",
		"conditions",
	)
	if err != nil {
		return "", "", "", false, fmt.Errorf(
			"read InferenceService conditions: %w",
			err,
		)
	}

	if !found {
		return "", "", "", false, nil
	}

	for _, item := range conditions {
		condition, ok := item.(map[string]any)
		if !ok {
			continue
		}

		conditionType, _, _ := unstructured.NestedString(
			condition,
			"type",
		)

		if conditionType != modelServicePhaseReady {
			continue
		}

		status, _, _ := unstructured.NestedString(
			condition,
			"status",
		)

		reason, _, _ := unstructured.NestedString(
			condition,
			"reason",
		)

		message, _, _ := unstructured.NestedString(
			condition,
			"message",
		)

		return metav1.ConditionStatus(status),
			reason,
			message,
			true,
			nil
	}

	return "", "", "", false, nil
}
