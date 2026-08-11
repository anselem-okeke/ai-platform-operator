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
	"fmt"
	"reflect"

	platformv1alpha1 "github.com/anselem-okeke/ai-platform-operator/api/v1alpha1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	modelContainerName        = "model"
	httpPortName              = "http"
	modelStorageName          = "model-storage"
	gatewayDataPlaneNamespace = "envoy-gateway-system"
	namespaceNameLabel        = "kubernetes.io/metadata.name"

	runtimeTmpVolumeName = "runtime-tmp"
	runtimeTmpMountPath  = "/tmp"

	defaultStartupPath             = "/"
	defaultStartupFailureThreshold = int32(30)
	defaultStartupPeriodSeconds    = int32(10)

	defaultReadinessPath = "/"
	defaultLivenessPath  = "/"

	defaultRunAsUser  int64 = 101
	defaultRunAsGroup int64 = 101
	defaultFSGroup    int64 = 101

	defaultTerminationGracePeriodSeconds = int64(30)
	defaultPreStopDelaySeconds           = int32(5)
	defaultMinReadySeconds               = int32(5)
	defaultProgressDeadlineSeconds       = int32(600)
	defaultMaxUnavailable                = "0"
	defaultMaxSurge                      = "1"
)

// ModelServiceReconciler reconciles a ModelService object.
type ModelServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Permissions for ModelService resources.
//
// +kubebuilder:rbac:groups=platform.anselem.dev,resources=modelservices,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=platform.anselem.dev,resources=modelservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.anselem.dev,resources=modelservices/finalizers,verbs=update
//
// Permissions for owned ServiceAccounts.
//
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//
// Permissions for owned Deployments.
//
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get
//
// Permissions for owned Services.
//
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//
// Permissions for owned PersistentVolumeClaims.
//
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims/status,verbs=get
//
// Permissions for owned PodDisruptionBudgets.
//
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets/status,verbs=get
//
// Permissions for owned NetworkPolicies.
//
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
//
// Permissions for owned HTTPRoutes.
//
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get
// Reconcile moves the current cluster state toward the desired state declared
// by a ModelService.

func (r *ModelServiceReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// Read the parent ModelService from the Kubernetes API.
	modelService := &platformv1alpha1.ModelService{}
	if err := r.Get(ctx, req.NamespacedName, modelService); err != nil {
		if apierrors.IsNotFound(err) {
			// The ModelService was deleted.
			// Kubernetes garbage collection handles owned resources.
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	labels := labelsForModelService(modelService)

	// -------------------------------------------------------------------------
	// Reconcile ServiceAccount
	// -------------------------------------------------------------------------

	if err := r.reconcileServiceAccount(
		ctx,
		modelService,
		labels,
	); err != nil {
		return ctrl.Result{}, err
	}

	// -------------------------------------------------------------------------
	// Reconcile Deployment
	// -------------------------------------------------------------------------

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modelService.Name,
			Namespace: modelService.Namespace,
		},
	}

	deploymentOperationResult, err := controllerutil.CreateOrUpdate(
		ctx,
		r.Client,
		deployment,
		func() error {
			deployment.Labels = labels

			rollout := rolloutForModelService(modelService)

			deployment.Spec.Replicas = pointerToInt32(
				modelService.Spec.Replicas,
			)

			deployment.Spec.Strategy = appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: intOrStringPointer(
						intstr.Parse(rollout.MaxUnavailable),
					),
					MaxSurge: intOrStringPointer(
						intstr.Parse(rollout.MaxSurge),
					),
				},
			}

			deployment.Spec.MinReadySeconds =
				rollout.MinReadySeconds

			deployment.Spec.ProgressDeadlineSeconds =
				int32Pointer(rollout.ProgressDeadlineSeconds)

			deployment.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: labels,
			}

			deployment.Spec.Template.Labels = labels

			health := healthForModelService(modelService)
			security := securityForModelService(modelService)

			// Define the model-serving container.
			container := corev1.Container{
				Name:            modelContainerName,
				Image:           modelService.Spec.Image,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       modelService.Spec.Resources,

				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      runtimeTmpVolumeName,
						MountPath: runtimeTmpMountPath,
					},
				},

				SecurityContext: containerSecurityContextForModelService(
					security,
				),

				Lifecycle: lifecycleForModelService(rollout),

				Ports: []corev1.ContainerPort{
					{
						Name:          httpPortName,
						ContainerPort: modelService.Spec.Port,
						Protocol:      corev1.ProtocolTCP,
					},
				},

				StartupProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: health.StartupPath,
							Port: intstr.FromString(httpPortName),
						},
					},
					PeriodSeconds:    health.StartupPeriodSeconds,
					TimeoutSeconds:   2,
					FailureThreshold: health.StartupFailureThreshold,
				},

				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: health.ReadinessPath,
							Port: intstr.FromString(httpPortName),
						},
					},
					InitialDelaySeconds: 2,
					PeriodSeconds:       5,
					TimeoutSeconds:      2,
					FailureThreshold:    3,
				},

				LivenessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: health.LivenessPath,
							Port: intstr.FromString(httpPortName),
						},
					},
					InitialDelaySeconds: 10,
					PeriodSeconds:       10,
					TimeoutSeconds:      2,
					FailureThreshold:    3,
				},
			}

			// Start with a PodSpec containing the model container and the Pod-level
			// security configuration.
			podSecurityContext := &corev1.PodSecurityContext{
				RunAsNonRoot: boolPointer(
					security.RunAsNonRoot,
				),
				FSGroup: int64Pointer(
					security.FSGroup,
				),
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			}

			// Only force a non-root UID and GID when non-root execution is enabled.
			// When runAsNonRoot is false, the image's configured user is preserved.
			if security.RunAsNonRoot {
				podSecurityContext.RunAsUser = int64Pointer(
					security.RunAsUser,
				)

				podSecurityContext.RunAsGroup = int64Pointer(
					security.RunAsGroup,
				)
			}

			podSpec := corev1.PodSpec{
				TerminationGracePeriodSeconds: int64Pointer(
					rollout.TerminationGracePeriodSeconds,
				),

				ServiceAccountName: modelService.Name,

				AutomountServiceAccountToken: boolPointer(
					resolveAutomountServiceAccountToken(modelService),
				),

				SecurityContext: podSecurityContext,

				Containers: []corev1.Container{
					container,
				},

				Volumes: []corev1.Volume{
					{
						Name: runtimeTmpVolumeName,
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
				},
			}

			// When storage is enabled, mount the ModelService PVC into the
			// container at the configured mount path.
			if modelService.Spec.Storage != nil &&
				modelService.Spec.Storage.Enabled {

				podSpec.Containers[0].VolumeMounts = append(
					podSpec.Containers[0].VolumeMounts,
					corev1.VolumeMount{
						Name:      modelStorageName,
						MountPath: modelService.Spec.Storage.MountPath,
					},
				)

				podSpec.Volumes = append(
					podSpec.Volumes,
					corev1.Volume{
						Name: modelStorageName,
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: modelService.Name,
								ReadOnly:  false,
							},
						},
					},
				)
			}

			deployment.Spec.Template.Spec = podSpec

			return controllerutil.SetControllerReference(
				modelService,
				deployment,
				r.Scheme,
			)
		},
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info(
		"reconciled ModelService Deployment",
		"modelService", req.NamespacedName,
		"deployment", client.ObjectKeyFromObject(deployment),
		"operation", deploymentOperationResult,
	)

	// -------------------------------------------------------------------------
	// Reconcile Service
	// -------------------------------------------------------------------------

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modelService.Name,
			Namespace: modelService.Namespace,
		},
	}

	serviceOperationResult, err := controllerutil.CreateOrUpdate(
		ctx,
		r.Client,
		service,
		func() error {
			service.Labels = labels

			// The selector must match the labels on the Pods created by the
			// Deployment.
			service.Spec.Selector = labels

			service.Spec.Ports = []corev1.ServicePort{
				{
					Name:       httpPortName,
					Port:       modelService.Spec.Port,
					TargetPort: intstr.FromString(httpPortName),
					Protocol:   corev1.ProtocolTCP,
				},
			}

			// ClusterIP gives the workload a stable internal endpoint.
			service.Spec.Type = corev1.ServiceTypeClusterIP

			return controllerutil.SetControllerReference(
				modelService,
				service,
				r.Scheme,
			)
		},
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info(
		"reconciled ModelService Service",
		"modelService", req.NamespacedName,
		"service", client.ObjectKeyFromObject(service),
		"operation", serviceOperationResult,
	)

	// -------------------------------------------------------------------------
	// Reconcile PersistentVolumeClaim
	// -------------------------------------------------------------------------

	if err := r.reconcilePersistentVolumeClaim(
		ctx,
		modelService,
	); err != nil {
		return ctrl.Result{}, err
	}

	// -------------------------------------------------------------------------
	// Reconcile PodDisruptionBudget
	// -------------------------------------------------------------------------

	if err := r.reconcilePodDisruptionBudget(
		ctx,
		modelService,
		labels,
	); err != nil {
		return ctrl.Result{}, err
	}

	// -------------------------------------------------------------------------
	// Reconcile NetworkPolicy
	// -------------------------------------------------------------------------

	if err := r.reconcileNetworkPolicy(
		ctx,
		modelService,
		labels,
	); err != nil {
		return ctrl.Result{}, err
	}

	// -------------------------------------------------------------------------
	// Reconcile HTTPRoute
	// -------------------------------------------------------------------------

	if err := r.reconcileHTTPRoute(
		ctx,
		modelService,
		labels,
	); err != nil {
		return ctrl.Result{}, err
	}

	// -------------------------------------------------------------------------
	// Reconcile ModelService status
	// -------------------------------------------------------------------------

	if err := r.updateModelServiceStatus(
		ctx,
		modelService,
		deployment,
	); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileServiceAccount creates or updates the dedicated ServiceAccount used
// by the model-serving Pods.
func (r *ModelServiceReconciler) reconcileServiceAccount(
	ctx context.Context,
	modelService *platformv1alpha1.ModelService,
	labels map[string]string,
) error {
	logger := logf.FromContext(ctx)

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modelService.Name,
			Namespace: modelService.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(
		ctx,
		r.Client,
		serviceAccount,
		func() error {
			serviceAccount.Labels = labels

			// The workload currently does not need Kubernetes API credentials.
			serviceAccount.AutomountServiceAccountToken =
				boolPointer(
					resolveAutomountServiceAccountToken(modelService),
				)

			return controllerutil.SetControllerReference(
				modelService,
				serviceAccount,
				r.Scheme,
			)
		},
	)
	if err != nil {
		return err
	}

	logger.Info(
		"reconciled ModelService ServiceAccount",
		"modelService",
		client.ObjectKeyFromObject(modelService),
		"serviceAccount",
		client.ObjectKeyFromObject(serviceAccount),
		"operation",
		operationResult,
	)

	return nil
}

func resolveAutomountServiceAccountToken(
	modelService *platformv1alpha1.ModelService,
) bool {
	if modelService.Spec.Security == nil {
		return false
	}

	if modelService.Spec.Security.AutomountServiceAccountToken == nil {
		return false
	}

	return *modelService.Spec.Security.AutomountServiceAccountToken
}

func containerSecurityContextForModelService(
	security platformv1alpha1.ModelServiceSecurity,
) *corev1.SecurityContext {
	securityContext := &corev1.SecurityContext{
		AllowPrivilegeEscalation: boolPointer(false),
		ReadOnlyRootFilesystem: boolPointer(
			security.ReadOnlyRootFilesystem,
		),
	}

	if security.RunAsNonRoot {
		securityContext.Capabilities = &corev1.Capabilities{
			Drop: []corev1.Capability{
				corev1.Capability("ALL"),
			},
		}
	}

	return securityContext
}

// reconcilePersistentVolumeClaim creates, updates, or deletes the PVC requested
// by the ModelService storage configuration.
func (r *ModelServiceReconciler) reconcilePersistentVolumeClaim(
	ctx context.Context,
	modelService *platformv1alpha1.ModelService,
) error {
	logger := logf.FromContext(ctx)

	pvcKey := types.NamespacedName{
		Name:      modelService.Name,
		Namespace: modelService.Namespace,
	}

	storageEnabled := modelService.Spec.Storage != nil &&
		modelService.Spec.Storage.Enabled

	// If storage is disabled or not configured, remove an existing PVC only
	// when the PVC is owned by this ModelService.
	if !storageEnabled {
		existingPVC := &corev1.PersistentVolumeClaim{}

		err := r.Get(ctx, pvcKey, existingPVC)

		switch {
		case apierrors.IsNotFound(err):
			// No PVC exists, so there is nothing to delete.
			return nil

		case err != nil:
			return err

		default:
			// Do not delete a PVC that belongs to another resource or user.
			if !metav1.IsControlledBy(existingPVC, modelService) {
				logger.Info(
					"PVC exists but is not controlled by ModelService; refusing deletion",
					"persistentVolumeClaim",
					client.ObjectKeyFromObject(existingPVC),
				)

				return nil
			}

			logger.Info(
				"deleting PVC because storage is disabled",
				"persistentVolumeClaim",
				client.ObjectKeyFromObject(existingPVC),
			)

			return r.Delete(ctx, existingPVC)
		}
	}

	// Convert a value such as "1Gi" into a Kubernetes resource quantity.
	storageSize, err := resource.ParseQuantity(
		modelService.Spec.Storage.Size,
	)
	if err != nil {
		return err
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modelService.Name,
			Namespace: modelService.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(
		ctx,
		r.Client,
		pvc,
		func() error {
			pvc.Labels = labelsForModelService(modelService)

			pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			}

			pvc.Spec.Resources = corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageSize,
				},
			}

			return controllerutil.SetControllerReference(
				modelService,
				pvc,
				r.Scheme,
			)
		},
	)
	if err != nil {
		return err
	}

	logger.Info(
		"reconciled ModelService PersistentVolumeClaim",
		"modelService",
		client.ObjectKeyFromObject(modelService),
		"persistentVolumeClaim",
		client.ObjectKeyFromObject(pvc),
		"operation",
		operationResult,
	)

	return nil
}

// reconcilePodDisruptionBudget creates, updates, or deletes the
// PodDisruptionBudget requested by the ModelService.
func (r *ModelServiceReconciler) reconcilePodDisruptionBudget(
	ctx context.Context,
	modelService *platformv1alpha1.ModelService,
	labels map[string]string,
) error {
	logger := logf.FromContext(ctx)

	pdbKey := types.NamespacedName{
		Name:      modelService.Name,
		Namespace: modelService.Namespace,
	}

	configuration :=
		podDisruptionBudgetForModelService(modelService)

	// If the PDB is disabled, remove an existing PDB only when this
	// ModelService controls it.
	if !configuration.Enabled {
		existingPDB := &policyv1.PodDisruptionBudget{}

		err := r.Get(ctx, pdbKey, existingPDB)

		switch {
		case apierrors.IsNotFound(err):
			return nil

		case err != nil:
			return err

		default:
			if !metav1.IsControlledBy(
				existingPDB,
				modelService,
			) {
				logger.Info(
					"PodDisruptionBudget exists but is not controlled by ModelService; refusing deletion",
					"podDisruptionBudget",
					client.ObjectKeyFromObject(existingPDB),
				)

				return nil
			}

			logger.Info(
				"deleting PodDisruptionBudget because it is disabled",
				"podDisruptionBudget",
				client.ObjectKeyFromObject(existingPDB),
			)

			return r.Delete(ctx, existingPDB)
		}
	}

	maxUnavailable := intstr.Parse(
		configuration.MaxUnavailable,
	)

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modelService.Name,
			Namespace: modelService.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(
		ctx,
		r.Client,
		pdb,
		func() error {
			pdb.Labels = labels

			pdb.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: labels,
			}

			pdb.Spec.MinAvailable = nil
			pdb.Spec.MaxUnavailable = &maxUnavailable

			return controllerutil.SetControllerReference(
				modelService,
				pdb,
				r.Scheme,
			)
		},
	)
	if err != nil {
		return err
	}

	logger.Info(
		"reconciled ModelService PodDisruptionBudget",
		"modelService",
		client.ObjectKeyFromObject(modelService),
		"podDisruptionBudget",
		client.ObjectKeyFromObject(pdb),
		"operation",
		operationResult,
	)

	return nil
}

// reconcileNetworkPolicy creates, updates, or deletes the application-scoped
// NetworkPolicy requested by the ModelService.
func (r *ModelServiceReconciler) reconcileNetworkPolicy(
	ctx context.Context,
	modelService *platformv1alpha1.ModelService,
	labels map[string]string,
) error {
	logger := logf.FromContext(ctx)

	networkPolicyKey := types.NamespacedName{
		Name:      modelService.Name,
		Namespace: modelService.Namespace,
	}

	configuration :=
		networkPolicyForModelService(modelService)

	if !configuration.Enabled {
		existingNetworkPolicy :=
			&networkingv1.NetworkPolicy{}

		err := r.Get(
			ctx,
			networkPolicyKey,
			existingNetworkPolicy,
		)

		switch {
		case apierrors.IsNotFound(err):
			return nil

		case err != nil:
			return err

		default:
			if !metav1.IsControlledBy(
				existingNetworkPolicy,
				modelService,
			) {
				logger.Info(
					"NetworkPolicy exists but is not controlled by ModelService; refusing deletion",
					"networkPolicy",
					client.ObjectKeyFromObject(
						existingNetworkPolicy,
					),
				)

				return nil
			}

			logger.Info(
				"deleting NetworkPolicy because it is disabled",
				"networkPolicy",
				client.ObjectKeyFromObject(
					existingNetworkPolicy,
				),
			)

			return r.Delete(
				ctx,
				existingNetworkPolicy,
			)
		}
	}

	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modelService.Name,
			Namespace: modelService.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(
		ctx,
		r.Client,
		networkPolicy,
		func() error {
			networkPolicy.Labels = labels

			networkPolicy.Spec.PodSelector =
				metav1.LabelSelector{
					MatchLabels: labels,
				}

			networkPolicy.Spec.PolicyTypes =
				[]networkingv1.PolicyType{
					networkingv1.PolicyTypeIngress,
					networkingv1.PolicyTypeEgress,
				}

			networkPolicy.Spec.Ingress =
				networkPolicyIngressRules(
					modelService,
					configuration,
				)

			networkPolicy.Spec.Egress =
				networkPolicyEgressRules(
					configuration,
				)

			return controllerutil.SetControllerReference(
				modelService,
				networkPolicy,
				r.Scheme,
			)
		},
	)
	if err != nil {
		return err
	}

	logger.Info(
		"reconciled ModelService NetworkPolicy",
		"modelService",
		client.ObjectKeyFromObject(modelService),
		"networkPolicy",
		client.ObjectKeyFromObject(networkPolicy),
		"operation",
		operationResult,
	)

	return nil
}

// reconcileHTTPRoute creates, updates, or deletes the HTTPRoute requested by
// the ModelService exposure configuration.
func (r *ModelServiceReconciler) reconcileHTTPRoute(
	ctx context.Context,
	modelService *platformv1alpha1.ModelService,
	labels map[string]string,
) error {
	logger := logf.FromContext(ctx)

	routeKey := types.NamespacedName{
		Name:      modelService.Name,
		Namespace: modelService.Namespace,
	}

	exposure := exposureForModelService(modelService)

	// When exposure is disabled, delete an existing HTTPRoute only when this
	// ModelService owns it.
	if !exposure.Enabled {
		existingRoute := &gatewayv1.HTTPRoute{}

		err := r.Get(
			ctx,
			routeKey,
			existingRoute,
		)

		switch {
		case apierrors.IsNotFound(err):
			return nil

		case err != nil:
			return err

		default:
			if !metav1.IsControlledBy(
				existingRoute,
				modelService,
			) {
				logger.Info(
					"HTTPRoute exists but is not controlled by ModelService; refusing deletion",
					"httpRoute",
					client.ObjectKeyFromObject(existingRoute),
				)

				return nil
			}

			logger.Info(
				"deleting HTTPRoute because exposure is disabled",
				"httpRoute",
				client.ObjectKeyFromObject(existingRoute),
			)

			return r.Delete(
				ctx,
				existingRoute,
			)
		}
	}

	if exposure.Hostname == "" {
		return fmt.Errorf(
			"spec.exposure.hostname must be set when exposure is enabled",
		)
	}

	gatewayNamespace :=
		gatewayv1.Namespace(exposure.GatewayNamespace)

	gatewaySectionName :=
		gatewayv1.SectionName(exposure.GatewaySectionName)

	hostname :=
		gatewayv1.Hostname(exposure.Hostname)

	pathType :=
		gatewayv1.PathMatchPathPrefix

	pathValue :=
		exposure.PathPrefix

	serviceKind :=
		gatewayv1.Kind("Service")

	servicePort :=
		modelService.Spec.Port

	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modelService.Name,
			Namespace: modelService.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(
		ctx,
		r.Client,
		route,
		func() error {
			route.Labels = labels

			route.Spec.ParentRefs =
				[]gatewayv1.ParentReference{
					{
						Name: gatewayv1.ObjectName(
							exposure.GatewayName,
						),
						Namespace:   &gatewayNamespace,
						SectionName: &gatewaySectionName,
					},
				}

			route.Spec.Hostnames =
				[]gatewayv1.Hostname{
					hostname,
				}

			route.Spec.Rules =
				[]gatewayv1.HTTPRouteRule{
					{
						Matches: []gatewayv1.HTTPRouteMatch{
							{
								Path: &gatewayv1.HTTPPathMatch{
									Type:  &pathType,
									Value: &pathValue,
								},
							},
						},

						BackendRefs: []gatewayv1.HTTPBackendRef{
							{
								BackendRef: gatewayv1.BackendRef{
									BackendObjectReference: gatewayv1.BackendObjectReference{
										Kind: &serviceKind,
										Name: gatewayv1.ObjectName(
											modelService.Name,
										),
										Port: &servicePort,
									},
									Weight: int32Pointer(1),
								},
							},
						},
					},
				}

			return controllerutil.SetControllerReference(
				modelService,
				route,
				r.Scheme,
			)
		},
	)
	if err != nil {
		return err
	}

	logger.Info(
		"reconciled ModelService HTTPRoute",
		"modelService",
		client.ObjectKeyFromObject(modelService),
		"httpRoute",
		client.ObjectKeyFromObject(route),
		"operation",
		operationResult,
	)

	return nil
}

func networkPolicyIngressRules(
	modelService *platformv1alpha1.ModelService,
	configuration resolvedNetworkPolicyConfiguration,
) []networkingv1.NetworkPolicyIngressRule {
	peers := []networkingv1.NetworkPolicyPeer{}

	if configuration.AllowSameNamespaceIngress {
		peers = append(
			peers,
			networkingv1.NetworkPolicyPeer{
				// A PodSelector without a NamespaceSelector selects
				// Pods in the policy's namespace.
				PodSelector: &metav1.LabelSelector{},
			},
		)
	}

	exposure := exposureForModelService(modelService)

	if exposure.Enabled {
		peers = append(
			peers,
			networkingv1.NetworkPolicyPeer{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						namespaceNameLabel: exposure.
							GatewayDataPlaneNamespace,
					},
				},
			},
		)
	}

	if len(peers) == 0 {
		return []networkingv1.NetworkPolicyIngressRule{}
	}

	port := intstr.FromInt32(modelService.Spec.Port)

	return []networkingv1.NetworkPolicyIngressRule{
		{
			From: peers,
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: protocolPointer(
						corev1.ProtocolTCP,
					),
					Port: &port,
				},
			},
		},
	}
}

func networkPolicyEgressRules(
	configuration resolvedNetworkPolicyConfiguration,
) []networkingv1.NetworkPolicyEgressRule {
	if !configuration.AllowDNSEgress {
		return []networkingv1.NetworkPolicyEgressRule{}
	}

	dnsPort := intstr.FromInt32(53)

	return []networkingv1.NetworkPolicyEgressRule{
		{
			To: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							namespaceNameLabel: "kube-system",
						},
					},

					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"k8s-app": "kube-dns",
						},
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: protocolPointer(
						corev1.ProtocolUDP,
					),
					Port: &dnsPort,
				},
				{
					Protocol: protocolPointer(
						corev1.ProtocolTCP,
					),
					Port: &dnsPort,
				},
			},
		},
	}
}

// updateModelServiceStatus derives the ModelService status from the current
// Deployment state and writes it through the Kubernetes status subresource.
func (r *ModelServiceReconciler) updateModelServiceStatus(
	ctx context.Context,
	modelService *platformv1alpha1.ModelService,
	deployment *appsv1.Deployment,
) error {
	logger := logf.FromContext(ctx)

	// Save the current status so that we avoid unnecessary status updates.
	previousStatus := modelService.Status.DeepCopy()

	desiredReplicas := modelService.Spec.Replicas
	readyReplicas := deployment.Status.ReadyReplicas

	endpoint := fmt.Sprintf(
		"http://%s.%s.svc.cluster.local:%d",
		modelService.Name,
		modelService.Namespace,
		modelService.Spec.Port,
	)

	modelService.Status.ReadyReplicas = readyReplicas
	modelService.Status.Endpoint = endpoint
	modelService.Status.ObservedGeneration = modelService.Generation

	switch {
	case deploymentProgressDeadlineExceeded(deployment):
		modelService.Status.Phase = "Degraded"

		setModelServiceCondition(
			modelService,
			metav1.ConditionFalse,
			"ProgressDeadlineExceeded",
			"The Deployment did not become ready before its progress deadline",
		)

	case desiredReplicas > 0 &&
		readyReplicas == desiredReplicas &&
		deployment.Status.AvailableReplicas == desiredReplicas:

		modelService.Status.Phase = "Ready"

		setModelServiceCondition(
			modelService,
			metav1.ConditionTrue,
			"DeploymentAvailable",
			"Model service is ready",
		)

	default:
		modelService.Status.Phase = "Provisioning"

		message := fmt.Sprintf(
			"Waiting for ready replicas: %d of %d ready",
			readyReplicas,
			desiredReplicas,
		)

		setModelServiceCondition(
			modelService,
			metav1.ConditionFalse,
			"DeploymentNotReady",
			message,
		)
	}

	if reflect.DeepEqual(
		*previousStatus,
		modelService.Status,
	) {
		return nil
	}

	if err := r.Status().Update(ctx, modelService); err != nil {
		return err
	}

	logger.Info(
		"updated ModelService status",
		"modelService", client.ObjectKeyFromObject(modelService),
		"phase", modelService.Status.Phase,
		"readyReplicas", modelService.Status.ReadyReplicas,
		"endpoint", modelService.Status.Endpoint,
		"observedGeneration", modelService.Status.ObservedGeneration,
	)

	return nil
}

// setModelServiceCondition creates or replaces the Available condition.
func setModelServiceCondition(
	modelService *platformv1alpha1.ModelService,
	status metav1.ConditionStatus,
	reason string,
	message string,
) {
	newCondition := metav1.Condition{
		Type:               "Available",
		Status:             status,
		ObservedGeneration: modelService.Generation,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	for index := range modelService.Status.Conditions {
		existingCondition := &modelService.Status.Conditions[index]

		if existingCondition.Type != newCondition.Type {
			continue
		}

		// Preserve LastTransitionTime when the meaningful condition state has
		// not changed.
		if existingCondition.Status == newCondition.Status &&
			existingCondition.Reason == newCondition.Reason &&
			existingCondition.Message == newCondition.Message {

			newCondition.LastTransitionTime =
				existingCondition.LastTransitionTime
		}

		modelService.Status.Conditions[index] = newCondition
		return
	}

	modelService.Status.Conditions = append(
		modelService.Status.Conditions,
		newCondition,
	)
}

// deploymentProgressDeadlineExceeded reports whether Kubernetes considers the
// Deployment rollout to have exceeded its progress deadline.
func deploymentProgressDeadlineExceeded(
	deployment *appsv1.Deployment,
) bool {
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing &&
			condition.Status == corev1.ConditionFalse &&
			condition.Reason == "ProgressDeadlineExceeded" {
			return true
		}
	}

	return false
}

type resolvedHealthConfiguration struct {
	StartupPath             string
	StartupFailureThreshold int32
	StartupPeriodSeconds    int32
	ReadinessPath           string
	LivenessPath            string
}

// healthForModelService returns the configured health settings.
// Controller defaults are used when spec.health is omitted or fields are empty.
func healthForModelService(
	modelService *platformv1alpha1.ModelService,
) resolvedHealthConfiguration {
	health := resolvedHealthConfiguration{
		StartupPath:             defaultStartupPath,
		StartupFailureThreshold: defaultStartupFailureThreshold,
		StartupPeriodSeconds:    defaultStartupPeriodSeconds,
		ReadinessPath:           defaultReadinessPath,
		LivenessPath:            defaultLivenessPath,
	}

	if modelService.Spec.Health == nil {
		return health
	}

	if modelService.Spec.Health.StartupPath != "" {
		health.StartupPath =
			modelService.Spec.Health.StartupPath
	}

	if modelService.Spec.Health.StartupFailureThreshold > 0 {
		health.StartupFailureThreshold =
			modelService.Spec.Health.StartupFailureThreshold
	}

	if modelService.Spec.Health.StartupPeriodSeconds > 0 {
		health.StartupPeriodSeconds =
			modelService.Spec.Health.StartupPeriodSeconds
	}

	if modelService.Spec.Health.ReadinessPath != "" {
		health.ReadinessPath =
			modelService.Spec.Health.ReadinessPath
	}

	if modelService.Spec.Health.LivenessPath != "" {
		health.LivenessPath =
			modelService.Spec.Health.LivenessPath
	}

	return health
}

// securityForModelService returns the requested security configuration.
// Secure controller defaults are used when spec.security is omitted.
func securityForModelService(
	modelService *platformv1alpha1.ModelService,
) platformv1alpha1.ModelServiceSecurity {
	if modelService.Spec.Security != nil {
		return *modelService.Spec.Security
	}

	return platformv1alpha1.ModelServiceSecurity{
		RunAsNonRoot:           true,
		RunAsUser:              defaultRunAsUser,
		RunAsGroup:             defaultRunAsGroup,
		FSGroup:                defaultFSGroup,
		ReadOnlyRootFilesystem: true,
	}
}

type resolvedRolloutConfiguration struct {
	TerminationGracePeriodSeconds int64
	PreStopDelaySeconds           int32
	MinReadySeconds               int32
	ProgressDeadlineSeconds       int32
	MaxUnavailable                string
	MaxSurge                      string
}

// rolloutForModelService returns the configured graceful shutdown and
// Deployment rollout settings.
func rolloutForModelService(
	modelService *platformv1alpha1.ModelService,
) resolvedRolloutConfiguration {
	rollout := resolvedRolloutConfiguration{
		TerminationGracePeriodSeconds: defaultTerminationGracePeriodSeconds,
		PreStopDelaySeconds:           defaultPreStopDelaySeconds,
		MinReadySeconds:               defaultMinReadySeconds,
		ProgressDeadlineSeconds:       defaultProgressDeadlineSeconds,
		MaxUnavailable:                defaultMaxUnavailable,
		MaxSurge:                      defaultMaxSurge,
	}

	if modelService.Spec.Rollout == nil {
		return rollout
	}

	if modelService.Spec.Rollout.
		TerminationGracePeriodSeconds > 0 {

		rollout.TerminationGracePeriodSeconds =
			modelService.Spec.Rollout.
				TerminationGracePeriodSeconds
	}

	if modelService.Spec.Rollout.PreStopDelaySeconds >= 0 {
		rollout.PreStopDelaySeconds =
			modelService.Spec.Rollout.
				PreStopDelaySeconds
	}

	if modelService.Spec.Rollout.MinReadySeconds >= 0 {
		rollout.MinReadySeconds =
			modelService.Spec.Rollout.
				MinReadySeconds
	}

	if modelService.Spec.Rollout.
		ProgressDeadlineSeconds > 0 {

		rollout.ProgressDeadlineSeconds =
			modelService.Spec.Rollout.
				ProgressDeadlineSeconds
	}

	if modelService.Spec.Rollout.MaxUnavailable != "" {
		rollout.MaxUnavailable =
			modelService.Spec.Rollout.MaxUnavailable
	}

	if modelService.Spec.Rollout.MaxSurge != "" {
		rollout.MaxSurge =
			modelService.Spec.Rollout.MaxSurge
	}

	return rollout
}

// lifecycleForModelService creates the preStop hook used for graceful
// termination. A zero delay means that no hook is required.
func lifecycleForModelService(
	rollout resolvedRolloutConfiguration,
) *corev1.Lifecycle {
	if rollout.PreStopDelaySeconds <= 0 {
		return nil
	}

	return &corev1.Lifecycle{
		PreStop: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"/bin/sh",
					"-c",
					fmt.Sprintf(
						"sleep %d",
						rollout.PreStopDelaySeconds,
					),
				},
			},
		},
	}
}

// int32Pointer converts an int32 into an *int32.
func int32Pointer(value int32) *int32 {
	return &value
}

// intOrStringPointer converts an IntOrString into a pointer.
func intOrStringPointer(
	value intstr.IntOrString,
) *intstr.IntOrString {
	return &value
}

type resolvedPodDisruptionBudgetConfiguration struct {
	Enabled        bool
	MaxUnavailable string
}

// podDisruptionBudgetForModelService returns the requested PDB configuration.
// The controller creates a PDB allowing one unavailable Pod by default.
func podDisruptionBudgetForModelService(
	modelService *platformv1alpha1.ModelService,
) resolvedPodDisruptionBudgetConfiguration {
	configuration :=
		resolvedPodDisruptionBudgetConfiguration{
			Enabled:        true,
			MaxUnavailable: "1",
		}

	if modelService.Spec.PodDisruptionBudget == nil {
		return configuration
	}

	configuration.Enabled =
		modelService.Spec.PodDisruptionBudget.Enabled

	if modelService.Spec.PodDisruptionBudget.
		MaxUnavailable != "" {

		configuration.MaxUnavailable =
			modelService.Spec.PodDisruptionBudget.
				MaxUnavailable
	}

	return configuration
}

type resolvedNetworkPolicyConfiguration struct {
	Enabled                   bool
	AllowSameNamespaceIngress bool
	AllowDNSEgress            bool
}

// networkPolicyForModelService returns the requested network-policy
// configuration. Secure defaults are used when spec.networkPolicy is omitted.
func networkPolicyForModelService(
	modelService *platformv1alpha1.ModelService,
) resolvedNetworkPolicyConfiguration {
	configuration := resolvedNetworkPolicyConfiguration{
		Enabled:                   true,
		AllowSameNamespaceIngress: true,
		AllowDNSEgress:            true,
	}

	if modelService.Spec.NetworkPolicy == nil {
		return configuration
	}

	configuration.Enabled =
		modelService.Spec.NetworkPolicy.Enabled

	configuration.AllowSameNamespaceIngress =
		modelService.Spec.NetworkPolicy.
			AllowSameNamespaceIngress

	configuration.AllowDNSEgress =
		modelService.Spec.NetworkPolicy.
			AllowDNSEgress

	return configuration
}

type resolvedExposureConfiguration struct {
	Enabled                   bool
	Hostname                  string
	PathPrefix                string
	GatewayName               string
	GatewayNamespace          string
	GatewaySectionName        string
	GatewayDataPlaneNamespace string
}

// exposureForModelService returns the requested HTTP exposure configuration.
func exposureForModelService(
	modelService *platformv1alpha1.ModelService,
) resolvedExposureConfiguration {
	configuration := resolvedExposureConfiguration{
		Enabled:                   false,
		PathPrefix:                "/",
		GatewayName:               "shared-gateway",
		GatewayNamespace:          "gateway-system",
		GatewaySectionName:        "http",
		GatewayDataPlaneNamespace: gatewayDataPlaneNamespace,
	}

	if modelService.Spec.Exposure == nil {
		return configuration
	}

	configuration.Enabled =
		modelService.Spec.Exposure.Enabled

	configuration.Hostname =
		modelService.Spec.Exposure.Hostname

	if modelService.Spec.Exposure.PathPrefix != "" {
		configuration.PathPrefix =
			modelService.Spec.Exposure.PathPrefix
	}

	if modelService.Spec.Exposure.GatewayName != "" {
		configuration.GatewayName =
			modelService.Spec.Exposure.GatewayName
	}

	if modelService.Spec.Exposure.GatewayNamespace != "" {
		configuration.GatewayNamespace =
			modelService.Spec.Exposure.GatewayNamespace
	}

	if modelService.Spec.Exposure.GatewaySectionName != "" {
		configuration.GatewaySectionName =
			modelService.Spec.Exposure.GatewaySectionName
	}

	if modelService.Spec.Exposure.
		GatewayDataPlaneNamespace != "" {

		configuration.GatewayDataPlaneNamespace =
			modelService.Spec.Exposure.
				GatewayDataPlaneNamespace
	}

	return configuration
}

// labelsForModelService returns the labels shared by the Deployment,
// Pod template, Service selector, and PVC.
func labelsForModelService(
	modelService *platformv1alpha1.ModelService,
) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "modelservice",
		"app.kubernetes.io/instance":   modelService.Name,
		"app.kubernetes.io/managed-by": "ai-platform-operator",
	}
}

// pointerToInt32 converts an int32 value into an *int32, which is required by
// Deployment.Spec.Replicas.
func pointerToInt32(value int32) *int32 {
	return &value
}

// boolPointer converts a bool into a *bool for Kubernetes API fields.
func boolPointer(value bool) *bool {
	return &value
}

// int64Pointer converts an int64 into an *int64 for Kubernetes API fields.
func int64Pointer(value int64) *int64 {
	return &value
}

// protocolPointer converts a Protocol into a *Protocol.
func protocolPointer(
	value corev1.Protocol,
) *corev1.Protocol {
	return &value
}

// SetupWithManager configures the controller to watch ModelServices and their
// owned Deployments, Services, and PersistentVolumeClaims.
func (r *ModelServiceReconciler) SetupWithManager(
	mgr ctrl.Manager,
) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.ModelService{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Owns(&gatewayv1.HTTPRoute{}).
		Named("modelservice").
		Complete(r)
}
