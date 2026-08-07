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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ModelServiceStorage defines persistent storage for a model service.
type ModelServiceStorage struct {
	// Enabled determines whether persistent storage should be provisioned.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Size is the requested persistent volume capacity.
	// +kubebuilder:default="1Gi"
	// +kubebuilder:validation:Pattern=`^[0-9]+(Mi|Gi|Ti)$`
	Size string `json:"size,omitempty"`

	// MountPath is the path where storage is mounted in the model container.
	// +kubebuilder:default="/models"
	// +kubebuilder:validation:Pattern=`^/.*`
	MountPath string `json:"mountPath,omitempty"`
}

// ModelServiceSecurity defines Pod-level and container-level security settings
// for the model service.
type ModelServiceSecurity struct {
	// RunAsNonRoot requires the model container to run as a non-root user.
	// +kubebuilder:default=true
	RunAsNonRoot bool `json:"runAsNonRoot,omitempty"`

	// RunAsUser is the Linux user ID used by the model container.
	// +kubebuilder:default=101
	// +kubebuilder:validation:Minimum=1
	RunAsUser int64 `json:"runAsUser,omitempty"`

	// RunAsGroup is the primary Linux group ID used by the model container.
	// +kubebuilder:default=101
	// +kubebuilder:validation:Minimum=1
	RunAsGroup int64 `json:"runAsGroup,omitempty"`

	// FSGroup is the supplemental group applied to mounted volumes.
	// It allows the non-root process to access supported persistent volumes.
	// +kubebuilder:default=101
	// +kubebuilder:validation:Minimum=1
	FSGroup int64 `json:"fsGroup,omitempty"`

	// ReadOnlyRootFilesystem prevents writes to the container root filesystem.
	// Writable application data should use mounted volumes such as emptyDir
	// or PersistentVolumeClaim volumes.
	// +kubebuilder:default=true
	ReadOnlyRootFilesystem bool `json:"readOnlyRootFilesystem,omitempty"`

	// AutomountServiceAccountToken controls whether Kubernetes API credentials
	// are automatically mounted into the workload Pod.
	//
	// Model-serving workloads should normally not require direct Kubernetes
	// API access.
	//
	// +kubebuilder:default=false
	// +optional
	AutomountServiceAccountToken *bool `json:"automountServiceAccountToken,omitempty"`
}

// ModelServiceHealth defines HTTP health check configuration.
type ModelServiceHealth struct {
	// StartupPath is the HTTP path used while the model service is starting.
	// Readiness and liveness probes do not begin until this probe succeeds.
	// +kubebuilder:default="/"
	// +kubebuilder:validation:Pattern=`^/.*`
	StartupPath string `json:"startupPath,omitempty"`

	// StartupFailureThreshold controls how many startup-probe failures are
	// allowed before Kubernetes restarts the container.
	// Combined with StartupPeriodSeconds, this determines the maximum startup
	// duration.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=120
	StartupFailureThreshold int32 `json:"startupFailureThreshold,omitempty"`

	// StartupPeriodSeconds controls how frequently the startup probe runs.
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=60
	StartupPeriodSeconds int32 `json:"startupPeriodSeconds,omitempty"`

	// ReadinessPath is the HTTP path used to determine whether the
	// model service is ready to receive traffic.
	// +kubebuilder:default="/"
	// +kubebuilder:validation:Pattern=`^/.*`
	ReadinessPath string `json:"readinessPath,omitempty"`

	// LivenessPath is the HTTP path used to determine whether the
	// model service is still healthy.
	// +kubebuilder:default="/"
	// +kubebuilder:validation:Pattern=`^/.*`
	LivenessPath string `json:"livenessPath,omitempty"`
}

// ModelServiceRollout defines graceful termination and Deployment rollout
// behavior for the model service.
type ModelServiceRollout struct {
	// TerminationGracePeriodSeconds is the total time Kubernetes allows for
	// the preStop hook and normal container shutdown.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=600
	TerminationGracePeriodSeconds int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// PreStopDelaySeconds delays container termination to allow Service and
	// EndpointSlice changes to propagate before the process exits.
	// This value must be lower than TerminationGracePeriodSeconds.
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=120
	PreStopDelaySeconds int32 `json:"preStopDelaySeconds,omitempty"`

	// MinReadySeconds is the minimum time a new Pod must remain ready before
	// Kubernetes considers it available.
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=300
	MinReadySeconds int32 `json:"minReadySeconds,omitempty"`

	// ProgressDeadlineSeconds is the maximum time allowed for a Deployment
	// rollout to make progress.
	// +kubebuilder:default=600
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3600
	ProgressDeadlineSeconds int32 `json:"progressDeadlineSeconds,omitempty"`

	// MaxUnavailable is the maximum number or percentage of replicas that may
	// be unavailable during a rolling update.
	// +kubebuilder:default="0"
	// +kubebuilder:validation:Pattern=`^([0-9]+|[0-9]+%)$`
	MaxUnavailable string `json:"maxUnavailable,omitempty"`

	// MaxSurge is the maximum number or percentage of additional replicas
	// created during a rolling update.
	// +kubebuilder:default="1"
	// +kubebuilder:validation:Pattern=`^([0-9]+|[0-9]+%)$`
	MaxSurge string `json:"maxSurge,omitempty"`
}

// ModelServicePodDisruptionBudget defines voluntary-disruption protection
// for the model-serving Pods.
type ModelServicePodDisruptionBudget struct {
	// Enabled determines whether the operator should create a
	// PodDisruptionBudget for this ModelService.
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// MaxUnavailable is the maximum number or percentage of Pods that may be
	// unavailable after a voluntary disruption.
	// Examples: "1", "25%".
	// +kubebuilder:default="1"
	// +kubebuilder:validation:Pattern=`^([0-9]+|[0-9]+%)$`
	MaxUnavailable string `json:"maxUnavailable,omitempty"`
}

// ModelServiceNetworkPolicy defines network isolation for the
// model-serving Pods.
type ModelServiceNetworkPolicy struct {
	// Enabled determines whether the operator creates a NetworkPolicy.
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// AllowSameNamespaceIngress allows Pods in the ModelService namespace
	// to connect to the model-serving port.
	// +kubebuilder:default=true
	AllowSameNamespaceIngress bool `json:"allowSameNamespaceIngress,omitempty"`

	// AllowDNSEgress allows UDP and TCP DNS traffic to CoreDNS.
	// +kubebuilder:default=true
	AllowDNSEgress bool `json:"allowDNSEgress,omitempty"`
}

// ModelServiceExposure defines optional external HTTP exposure through
// Kubernetes Gateway API.
type ModelServiceExposure struct {
	// Enabled determines whether the operator creates an HTTPRoute.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Hostname is the DNS hostname matched by the HTTPRoute.
	// Example: fraud-model.local
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Hostname string `json:"hostname,omitempty"`

	// PathPrefix is the HTTP path prefix forwarded to the ModelService.
	// +kubebuilder:default="/"
	// +kubebuilder:validation:Pattern=`^/.*`
	PathPrefix string `json:"pathPrefix,omitempty"`

	// GatewayName is the name of the shared Gateway.
	// +kubebuilder:default="shared-gateway"
	// +kubebuilder:validation:MinLength=1
	GatewayName string `json:"gatewayName,omitempty"`

	// GatewayNamespace is the namespace containing the shared Gateway.
	// +kubebuilder:default="gateway-system"
	// +kubebuilder:validation:MinLength=1
	GatewayNamespace string `json:"gatewayNamespace,omitempty"`

	// GatewaySectionName identifies the Gateway listener to which the
	// HTTPRoute should attach.
	// +kubebuilder:default="http"
	// +kubebuilder:validation:MinLength=1
	GatewaySectionName string `json:"gatewaySectionName,omitempty"`

	// GatewayDataPlaneNamespace is the namespace containing the Gateway
	// proxy Pods. The operator allows ingress from this namespace through
	// the generated NetworkPolicy.
	// +kubebuilder:default="envoy-gateway-system"
	// +kubebuilder:validation:MinLength=1
	GatewayDataPlaneNamespace string `json:"gatewayDataPlaneNamespace,omitempty"`
}

// ModelServiceSpec defines the desired state of ModelService.
type ModelServiceSpec struct {
	// Image is the container image used to run the model service.
	// Example: ghcr.io/anselem-okeke/fraud-model:v1
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// Replicas is the desired number of model-serving pods.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Replicas int32 `json:"replicas,omitempty"`

	// Port is the port exposed by the model container and Service.
	// +kubebuilder:default=8080
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// Resources defines CPU and memory requests and limits for the
	// model-serving container.
	//
	// Requests are used by the Kubernetes scheduler when deciding where
	// the Pod can run.
	//
	// Limits define the maximum CPU and memory that the container may use.
	//
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Security contains Pod-level and container-level security configuration.
	// When omitted, the controller will apply its secure defaults.
	// +optional
	Security *ModelServiceSecurity `json:"security,omitempty"`

	// Health contains HTTP health check configuration.
	// When omitted, both probe paths default to "/".
	// +optional
	Health *ModelServiceHealth `json:"health,omitempty"`

	// Rollout contains graceful termination and Deployment rollout settings.
	// When omitted, the controller applies safe defaults.
	// +optional
	Rollout *ModelServiceRollout `json:"rollout,omitempty"`

	// PodDisruptionBudget contains voluntary-disruption protection settings.
	// When omitted, the controller applies its default PDB configuration.
	// +optional
	PodDisruptionBudget *ModelServicePodDisruptionBudget `json:"podDisruptionBudget,omitempty"`

	// NetworkPolicy contains network-isolation settings.
	// When omitted, the controller applies its secure defaults.
	// +optional
	NetworkPolicy *ModelServiceNetworkPolicy `json:"networkPolicy,omitempty"`

	// Exposure contains optional Gateway API HTTP exposure configuration.
	// When disabled or omitted, no HTTPRoute is created.
	// +optional
	Exposure *ModelServiceExposure `json:"exposure,omitempty"`

	// Storage contains persistent-storage configuration.
	// +optional
	Storage *ModelServiceStorage `json:"storage,omitempty"`
}

// ModelServiceStatus defines the observed state of ModelService.
type ModelServiceStatus struct {
	// Phase represents the current lifecycle phase.
	// Expected values include Pending, Provisioning, Ready, Degraded and Failed.
	Phase string `json:"phase,omitempty"`

	// ReadyReplicas is the number of ready model-serving pods.
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Endpoint is the internal Kubernetes Service endpoint.
	Endpoint string `json:"endpoint,omitempty"`

	// ObservedGeneration is the latest resource generation processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions describe the latest observed state of the ModelService.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ms
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.status.endpoint`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ModelService is the Schema for the modelservices API.
type ModelService struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of ModelService.
	// +required
	Spec ModelServiceSpec `json:"spec"`

	// status defines the observed state of ModelService.
	// +optional
	Status ModelServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ModelServiceList contains a list of ModelService.
type ModelServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelService `json:"items"`
}
