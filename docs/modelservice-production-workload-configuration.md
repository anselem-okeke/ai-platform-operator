# Production-Grade Workload Configuration for `ModelService`

## 1. Purpose

This document describes the complete production-oriented workload configuration implemented for the `ModelService` custom resource and its Kubernetes controller.

The implementation extends the operator beyond basic Deployment, Service, persistent storage, and status reconciliation. It adds resource governance, configurable health checks, startup protection, hardened Pod and container security, writable runtime storage for read-only containers, graceful termination, controlled rolling updates, and voluntary-disruption protection through a PodDisruptionBudget.

The result is a `ModelService` abstraction that lets a platform user declare workload requirements in one custom resource while the operator translates those requirements into Kubernetes-native resources and policies.

---

## 2. Resulting resource model

A single `ModelService` now controls the following resources:

```text
ModelService
├── Deployment
│   └── Pod template
│       ├── model container
│       ├── resource requests and limits
│       ├── startup, readiness, and liveness probes
│       ├── container security context
│       ├── Pod security context
│       ├── preStop lifecycle hook
│       ├── termination grace period
│       ├── writable /tmp emptyDir
│       └── optional persistent model-storage mount
├── Service
├── PersistentVolumeClaim, when storage is enabled
├── PodDisruptionBudget, when enabled
└── ModelService status
```

---

## 3. Files changed

The implementation is spread across these files:

```text
api/v1alpha1/modelservice_types.go
internal/controller/modelservice_controller.go
internal/controller/modelservice_controller_test.go
config/samples/platform_v1alpha1_modelservice.yaml
config/crd/bases/platform.anselem.dev_modelservices.yaml
config/rbac/role.yaml
api/v1alpha1/zz_generated.deepcopy.go
```

The last three files are generated from API types and Kubebuilder RBAC markers.

---

# Part I — Custom Resource API

## 4. Resource requests and limits

The API exposes Kubernetes-native container resource requirements:

```go
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
```

Example:

```yaml
spec:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 512Mi
```

### Why requests matter

Requests are used by the scheduler when selecting a node. A Pod requesting `100m` CPU and `128Mi` memory is scheduled only where that amount of allocatable capacity is available.

### Why limits matter

Limits restrict maximum resource usage:

- CPU limits cause throttling when exceeded.
- Memory limits may cause the container to be terminated with `OOMKilled` when exceeded.

### Controller mapping

The controller copies the resource requirements directly into the model container:

```go
Resources: modelService.Spec.Resources,
```

---

## 5. Security configuration API

A dedicated security structure was added:

```go
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
    // +kubebuilder:default=101
    // +kubebuilder:validation:Minimum=1
    FSGroup int64 `json:"fsGroup,omitempty"`

    // ReadOnlyRootFilesystem prevents writes to the container root filesystem.
    // +kubebuilder:default=true
    ReadOnlyRootFilesystem bool `json:"readOnlyRootFilesystem,omitempty"`
}
```

The field is exposed in `ModelServiceSpec`:

```go
// Security contains Pod-level and container-level security configuration.
// When omitted, the controller applies secure defaults.
// +optional
Security *ModelServiceSecurity `json:"security,omitempty"`
```

Example:

```yaml
spec:
  security:
    runAsNonRoot: true
    runAsUser: 101
    runAsGroup: 101
    fsGroup: 101
    readOnlyRootFilesystem: true
```

---

## 6. Configurable health-check API

The original controller used `/` as a hard-coded readiness and liveness path. That worked for NGINX but created an implicit contract that every image must return success from `GET /`.

The API was expanded so model images can expose their own endpoints:

```go
// ModelServiceHealth defines HTTP health-check configuration.
type ModelServiceHealth struct {
    // StartupPath is used while the application is starting.
    // +kubebuilder:default="/"
    // +kubebuilder:validation:Pattern=`^/.*`
    StartupPath string `json:"startupPath,omitempty"`

    // StartupFailureThreshold controls how many startup failures are allowed.
    // +kubebuilder:default=30
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=120
    StartupFailureThreshold int32 `json:"startupFailureThreshold,omitempty"`

    // StartupPeriodSeconds controls how frequently the startup probe runs.
    // +kubebuilder:default=10
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=60
    StartupPeriodSeconds int32 `json:"startupPeriodSeconds,omitempty"`

    // ReadinessPath is the HTTP path used to determine whether the service
    // is ready to receive traffic.
    // +kubebuilder:default="/"
    // +kubebuilder:validation:Pattern=`^/.*`
    ReadinessPath string `json:"readinessPath,omitempty"`

    // LivenessPath is the HTTP path used to determine whether the service
    // is still healthy.
    // +kubebuilder:default="/"
    // +kubebuilder:validation:Pattern=`^/.*`
    LivenessPath string `json:"livenessPath,omitempty"`
}
```

The field in the specification:

```go
// Health contains HTTP health-check configuration.
// When omitted, probe paths default to "/".
// +optional
Health *ModelServiceHealth `json:"health,omitempty"`
```

Example:

```yaml
spec:
  health:
    startupPath: /health/startup
    startupFailureThreshold: 30
    startupPeriodSeconds: 10
    readinessPath: /health/ready
    livenessPath: /health/live
```

### Probe responsibilities

| Probe | Purpose | Failure effect |
|---|---|---|
| Startup | Determines whether initialization has completed | Container is restarted after threshold is exceeded |
| Readiness | Determines whether the Pod should receive Service traffic | Pod is removed from Service endpoints |
| Liveness | Determines whether the running process is healthy | Container is restarted |

### Startup window

The maximum startup allowance is approximately:

```text
startupFailureThreshold × startupPeriodSeconds
```

With the defaults:

```text
30 × 10 seconds = 300 seconds
```

The workload therefore has up to five minutes to initialize before Kubernetes restarts it.

---

## 7. Rollout and graceful-termination API

A rollout structure was added:

```go
// ModelServiceRollout defines graceful termination and Deployment rollout
// behavior for the model service.
type ModelServiceRollout struct {
    // +kubebuilder:default=30
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=600
    TerminationGracePeriodSeconds int64 `json:"terminationGracePeriodSeconds,omitempty"`

    // +kubebuilder:default=5
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=120
    PreStopDelaySeconds int32 `json:"preStopDelaySeconds,omitempty"`

    // +kubebuilder:default=5
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=300
    MinReadySeconds int32 `json:"minReadySeconds,omitempty"`

    // +kubebuilder:default=600
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=3600
    ProgressDeadlineSeconds int32 `json:"progressDeadlineSeconds,omitempty"`

    // +kubebuilder:default="0"
    // +kubebuilder:validation:Pattern=`^([0-9]+|[0-9]+%)$`
    MaxUnavailable string `json:"maxUnavailable,omitempty"`

    // +kubebuilder:default="1"
    // +kubebuilder:validation:Pattern=`^([0-9]+|[0-9]+%)$`
    MaxSurge string `json:"maxSurge,omitempty"`
}
```

The `ModelServiceSpec` field:

```go
// Rollout contains graceful termination and Deployment rollout settings.
// When omitted, the controller applies safe defaults.
// +optional
Rollout *ModelServiceRollout `json:"rollout,omitempty"`
```

Example:

```yaml
spec:
  rollout:
    terminationGracePeriodSeconds: 30
    preStopDelaySeconds: 5
    minReadySeconds: 5
    progressDeadlineSeconds: 600
    maxUnavailable: "0"
    maxSurge: "1"
```

---

## 8. PodDisruptionBudget API

Voluntary-disruption protection was added:

```go
// ModelServicePodDisruptionBudget defines voluntary-disruption protection
// for the model-serving Pods.
type ModelServicePodDisruptionBudget struct {
    // Enabled determines whether the operator creates a PDB.
    // +kubebuilder:default=true
    Enabled bool `json:"enabled,omitempty"`

    // MaxUnavailable is the maximum number or percentage of Pods that may be
    // unavailable after a voluntary disruption.
    // +kubebuilder:default="1"
    // +kubebuilder:validation:Pattern=`^([0-9]+|[0-9]+%)$`
    MaxUnavailable string `json:"maxUnavailable,omitempty"`
}
```

Specification field:

```go
// PodDisruptionBudget contains voluntary-disruption protection settings.
// When omitted, the controller applies its default PDB configuration.
// +optional
PodDisruptionBudget *ModelServicePodDisruptionBudget `json:"podDisruptionBudget,omitempty"`
```

Example:

```yaml
spec:
  podDisruptionBudget:
    enabled: true
    maxUnavailable: "1"
```

A PDB protects against voluntary disruptions such as node drain and planned maintenance. It does not prevent failures caused by node loss, process crashes, resource exhaustion, or involuntary eviction.

---

# Part II — Controller Implementation

## 9. Reusable controller constants

Repeated hard-coded values were moved to constants:

```go
const (
    modelContainerName = "model"
    httpPortName       = "http"
    modelStorageName   = "model-storage"

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
```

This avoids accidental mismatch between container ports, Service target ports, volume names, and default settings.

---

## 10. Configuration resolver pattern

The controller uses resolver helpers before constructing Kubernetes resources:

```go
health := healthForModelService(modelService)
security := securityForModelService(modelService)
rollout := rolloutForModelService(modelService)
```

Resolvers provide controller-side defaults when fields are omitted or when tests construct Go objects without passing through API-server defaulting.

### Health resolver

```go
type resolvedHealthConfiguration struct {
    StartupPath             string
    StartupFailureThreshold int32
    StartupPeriodSeconds    int32
    ReadinessPath           string
    LivenessPath            string
}
```

```go
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
        health.StartupPath = modelService.Spec.Health.StartupPath
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
        health.ReadinessPath = modelService.Spec.Health.ReadinessPath
    }

    if modelService.Spec.Health.LivenessPath != "" {
        health.LivenessPath = modelService.Spec.Health.LivenessPath
    }

    return health
}
```

### Security resolver

```go
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
```

### Rollout resolver

```go
type resolvedRolloutConfiguration struct {
    TerminationGracePeriodSeconds int64
    PreStopDelaySeconds           int32
    MinReadySeconds               int32
    ProgressDeadlineSeconds       int32
    MaxUnavailable                string
    MaxSurge                      string
}
```

The resolver applies defaults and then overrides them with non-empty values from `spec.rollout`.

### PDB resolver

```go
type resolvedPodDisruptionBudgetConfiguration struct {
    Enabled        bool
    MaxUnavailable string
}
```

Default behavior:

```text
podDisruptionBudget omitted
→ enabled=true, maxUnavailable="1"
```

---

## 11. Deployment rollout strategy

The controller configures a rolling Deployment strategy:

```go
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

deployment.Spec.MinReadySeconds = rollout.MinReadySeconds

deployment.Spec.ProgressDeadlineSeconds =
    int32Pointer(rollout.ProgressDeadlineSeconds)
```

Default behavior:

```text
maxUnavailable = 0
maxSurge       = 1
```

For two desired replicas, Kubernetes attempts to retain both existing available Pods while creating one replacement Pod. The old Pod is removed only after the new Pod becomes ready and satisfies `minReadySeconds`.

---

## 12. Container definition

The model container receives:

```go
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

    SecurityContext: containerSecurityContextForModelService(security),
    Lifecycle:       lifecycleForModelService(rollout),

    Ports: []corev1.ContainerPort{
        {
            Name:          httpPortName,
            ContainerPort: modelService.Spec.Port,
            Protocol:      corev1.ProtocolTCP,
        },
    },

    StartupProbe:   /* configured HTTP startup probe */,
    ReadinessProbe: /* configured HTTP readiness probe */,
    LivenessProbe:  /* configured HTTP liveness probe */,
}
```

The image is not hard-coded by the controller. It comes from:

```go
Image: modelService.Spec.Image
```

The sample and tests use concrete NGINX images, but the controller remains image-agnostic.

---

## 13. Startup probe

The startup probe is built from the resolved health configuration:

```go
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
```

Until the startup probe succeeds, Kubernetes does not activate the readiness or liveness probes. This prevents slow model loading from being misclassified as a liveness failure.

---

## 14. Readiness probe

```go
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
```

When readiness fails:

- the container remains running;
- the Pod becomes unready;
- the Pod is removed from Service endpoints;
- traffic is routed only to ready replicas.

---

## 15. Liveness probe

```go
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
```

When liveness repeatedly fails, kubelet restarts the container.

---

## 16. Pod security context

The controller creates a Pod-level security context:

```go
podSecurityContext := &corev1.PodSecurityContext{
    RunAsNonRoot: boolPointer(security.RunAsNonRoot),
    FSGroup:      int64Pointer(security.FSGroup),
    SeccompProfile: &corev1.SeccompProfile{
        Type: corev1.SeccompProfileTypeRuntimeDefault,
    },
}
```

When non-root execution is enabled, it also sets:

```go
if security.RunAsNonRoot {
    podSecurityContext.RunAsUser = int64Pointer(security.RunAsUser)
    podSecurityContext.RunAsGroup = int64Pointer(security.RunAsGroup)
}
```

This conditional behavior was important for compatibility testing. Setting `runAsNonRoot: false` should allow the image to use its declared user instead of always forcing UID and GID `101`.

### Pod-level fields

| Field | Effect |
|---|---|
| `runAsNonRoot` | Rejects a container that attempts to run as UID 0 |
| `runAsUser` | Forces the runtime UID |
| `runAsGroup` | Forces the primary runtime group |
| `fsGroup` | Adds a supplemental group for mounted volume access |
| `seccompProfile: RuntimeDefault` | Applies the runtime's default syscall filter |

---

## 17. Container security context

Strict non-root mode configures:

```go
&corev1.SecurityContext{
    AllowPrivilegeEscalation: boolPointer(false),
    ReadOnlyRootFilesystem: boolPointer(
        security.ReadOnlyRootFilesystem,
    ),
    Capabilities: &corev1.Capabilities{
        Drop: []corev1.Capability{
            corev1.Capability("ALL"),
        },
    },
}
```

### Security effects

| Setting | Purpose |
|---|---|
| `allowPrivilegeEscalation: false` | Prevents gaining additional privileges through setuid/setgid or similar mechanisms |
| `readOnlyRootFilesystem: true` | Prevents writes to the image filesystem |
| `capabilities.drop: [ALL]` | Removes Linux capabilities not required by the workload |

---

## 18. Writable `/tmp` with a read-only root filesystem

A read-only root filesystem can break applications that write runtime files under `/tmp`, `/var/run`, or cache directories.

The selected unprivileged NGINX image requires writable runtime space. The controller therefore creates an `emptyDir` volume:

```go
Volumes: []corev1.Volume{
    {
        Name: runtimeTmpVolumeName,
        VolumeSource: corev1.VolumeSource{
            EmptyDir: &corev1.EmptyDirVolumeSource{},
        },
    },
},
```

and mounts it into the container:

```go
VolumeMounts: []corev1.VolumeMount{
    {
        Name:      runtimeTmpVolumeName,
        MountPath: runtimeTmpMountPath,
    },
},
```

This allows:

```text
/tmp → writable ephemeral storage
/    → read-only image filesystem
```

Persistent model data remains mounted separately at `/models` when storage is enabled.

---

## 19. Persistent storage integration

When storage is enabled, the controller appends the PVC mount instead of replacing existing mounts:

```go
podSpec.Containers[0].VolumeMounts = append(
    podSpec.Containers[0].VolumeMounts,
    corev1.VolumeMount{
        Name:      modelStorageName,
        MountPath: modelService.Spec.Storage.MountPath,
    },
)
```

It also appends the PVC-backed volume:

```go
podSpec.Volumes = append(
    podSpec.Volumes,
    corev1.Volume{
        Name: modelStorageName,
        VolumeSource: corev1.VolumeSource{
            PersistentVolumeClaim:
                &corev1.PersistentVolumeClaimVolumeSource{
                    ClaimName: modelService.Name,
                    ReadOnly:  false,
                },
        },
    },
)
```

Using `append` is essential. Direct assignment would remove the previously configured runtime `/tmp` volume.

---

## 20. Graceful shutdown

The Pod receives a termination grace period:

```go
TerminationGracePeriodSeconds: int64Pointer(
    rollout.TerminationGracePeriodSeconds,
),
```

The container receives a `preStop` hook:

```go
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
```

### Termination sequence

```text
Pod selected for termination
        ↓
Pod begins terminating
        ↓
preStop hook runs
        ↓
Configured delay allows endpoint changes to propagate
        ↓
Runtime sends the normal termination signal
        ↓
Application exits within the remaining grace period
        ↓
Container is force-killed only if the total grace period expires
```

The configured termination grace period should always be greater than the pre-stop delay.

---

## 21. PodDisruptionBudget reconciliation

The controller imports:

```go
policyv1 "k8s.io/api/policy/v1"
```

It reconciles a PDB with the same labels used by the Deployment Pod template:

```go
pdb.Spec.Selector = &metav1.LabelSelector{
    MatchLabels: labels,
}

pdb.Spec.MinAvailable = nil
pdb.Spec.MaxUnavailable = &maxUnavailable
```

The PDB owner reference points to the `ModelService`:

```go
return controllerutil.SetControllerReference(
    modelService,
    pdb,
    r.Scheme,
)
```

The controller handles three cases:

1. **Enabled and absent** — create the PDB.
2. **Enabled and present** — update drift and specification changes.
3. **Disabled** — delete the PDB only when it is controlled by the `ModelService`.

It refuses to delete a same-named PDB that is not controlled by the custom resource.

---

## 22. PDB RBAC

Kubebuilder markers were added:

```go
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets/status,verbs=get
```

Generated RBAC must include:

```yaml
apiGroups:
  - policy
resources:
  - poddisruptionbudgets
verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
```

---

## 23. Owned-resource watches

The controller manager watches resources owned by `ModelService`:

```go
return ctrl.NewControllerManagedBy(mgr).
    For(&platformv1alpha1.ModelService{}).
    Owns(&appsv1.Deployment{}).
    Owns(&corev1.Service{}).
    Owns(&corev1.PersistentVolumeClaim{}).
    Owns(&policyv1.PodDisruptionBudget{}).
    Named("modelservice").
    Complete(r)
```

If an owned resource changes or is deleted, the owner is requeued and reconciled.

---

# Part III — Complete Example Custom Resource

## 24. Production-oriented sample

```yaml
apiVersion: platform.anselem.dev/v1alpha1
kind: ModelService
metadata:
  name: fraud-model
  namespace: ai-platform
spec:
  image: nginxinc/nginx-unprivileged:1.31-alpine
  replicas: 2
  port: 8080

  health:
    startupPath: /
    startupFailureThreshold: 30
    startupPeriodSeconds: 10
    readinessPath: /
    livenessPath: /

  rollout:
    terminationGracePeriodSeconds: 30
    preStopDelaySeconds: 5
    minReadySeconds: 5
    progressDeadlineSeconds: 600
    maxUnavailable: "0"
    maxSurge: "1"

  podDisruptionBudget:
    enabled: true
    maxUnavailable: "1"

  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 512Mi

  security:
    runAsNonRoot: true
    runAsUser: 101
    runAsGroup: 101
    fsGroup: 101
    readOnlyRootFilesystem: true

  storage:
    enabled: true
    size: 1Gi
    mountPath: /models
```

### Important distinction

The two `maxUnavailable` fields control different mechanisms:

```text
spec.rollout.maxUnavailable
→ controls Deployment rolling updates

spec.podDisruptionBudget.maxUnavailable
→ controls voluntary Pod evictions
```

---

# Part IV — Testing Strategy

## 25. Why controller tests are required

The test suite validates the desired Kubernetes objects produced by reconciliation. It does not execute the container image, so live cluster validation remains necessary.

The automated tests cover:

```text
resource propagation
health-probe generation
health-probe updates
startup-probe timing
Pod security context
container security context
runtime /tmp volume
persistent-storage volume
termination grace period
preStop lifecycle hook
Deployment rollout strategy
PodDisruptionBudget creation
PodDisruptionBudget update
PodDisruptionBudget ownership
PodDisruptionBudget deletion when disabled
optional PDB drift correction
```

---

## 26. Resource assertions

Typical assertions:

```go
Expect(container.Resources.Requests.Cpu().String()).
    To(Equal("100m"))

Expect(container.Resources.Requests.Memory().String()).
    To(Equal("128Mi"))

Expect(container.Resources.Limits.Cpu().String()).
    To(Equal("500m"))

Expect(container.Resources.Limits.Memory().String()).
    To(Equal("512Mi"))
```

---

## 27. Health-probe assertions

```go
Expect(container.StartupProbe).NotTo(BeNil())
Expect(container.StartupProbe.HTTPGet.Path).
    To(Equal("/startup"))
Expect(container.StartupProbe.FailureThreshold).
    To(Equal(int32(40)))
Expect(container.StartupProbe.PeriodSeconds).
    To(Equal(int32(5)))

Expect(container.ReadinessProbe.HTTPGet.Path).
    To(Equal("/ready"))

Expect(container.LivenessProbe.HTTPGet.Path).
    To(Equal("/live"))
```

Update tests change paths and timing values in the `ModelService`, reconcile again, and assert the Deployment is updated.

---

## 28. Security assertions

Pod-level assertions:

```go
podSecurityContext := deployment.Spec.Template.Spec.SecurityContext

Expect(podSecurityContext).NotTo(BeNil())
Expect(*podSecurityContext.RunAsNonRoot).To(BeTrue())
Expect(*podSecurityContext.RunAsUser).To(Equal(int64(101)))
Expect(*podSecurityContext.RunAsGroup).To(Equal(int64(101)))
Expect(*podSecurityContext.FSGroup).To(Equal(int64(101)))
Expect(podSecurityContext.SeccompProfile.Type).
    To(Equal(corev1.SeccompProfileTypeRuntimeDefault))
```

Container-level assertions:

```go
containerSecurityContext := container.SecurityContext

Expect(*containerSecurityContext.AllowPrivilegeEscalation).
    To(BeFalse())
Expect(*containerSecurityContext.ReadOnlyRootFilesystem).
    To(BeTrue())
Expect(containerSecurityContext.Capabilities.Drop).
    To(ContainElement(corev1.Capability("ALL")))
```

---

## 29. Runtime-volume assertions

Do not rely on array order. Search volumes and mounts by name:

```go
var runtimeTmpMount *corev1.VolumeMount
var modelStorageMount *corev1.VolumeMount

for index := range container.VolumeMounts {
    volumeMount := &container.VolumeMounts[index]

    switch volumeMount.Name {
    case runtimeTmpVolumeName:
        runtimeTmpMount = volumeMount
    case modelStorageName:
        modelStorageMount = volumeMount
    }
}
```

Assertions:

```go
Expect(runtimeTmpMount).NotTo(BeNil())
Expect(runtimeTmpMount.MountPath).To(Equal("/tmp"))
Expect(modelStorageMount.MountPath).To(Equal("/models"))
```

The same name-based approach should be used for Pod volumes.

---

## 30. Rollout assertions

```go
Expect(deployment.Spec.Strategy.Type).
    To(Equal(appsv1.RollingUpdateDeploymentStrategyType))

Expect(deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.String()).
    To(Equal("0"))

Expect(deployment.Spec.Strategy.RollingUpdate.MaxSurge.String()).
    To(Equal("2"))

Expect(deployment.Spec.MinReadySeconds).
    To(Equal(int32(10)))

Expect(*deployment.Spec.ProgressDeadlineSeconds).
    To(Equal(int32(900)))
```

Termination assertions:

```go
Expect(*deployment.Spec.Template.Spec.TerminationGracePeriodSeconds).
    To(Equal(int64(45)))

Expect(container.Lifecycle.PreStop.Exec.Command).
    To(Equal([]string{
        "/bin/sh",
        "-c",
        "sleep 7",
    }))
```

---

## 31. PodDisruptionBudget assertions

Creation:

```go
pdb := &policyv1.PodDisruptionBudget{}

Eventually(func() error {
    return k8sClient.Get(ctx, pdbKey, pdb)
}).Should(Succeed())
```

Specification:

```go
Expect(pdb.Spec.Selector.MatchLabels).
    To(Equal(labelsForModelService(modelService)))

Expect(pdb.Spec.MinAvailable).To(BeNil())
Expect(pdb.Spec.MaxUnavailable).NotTo(BeNil())
Expect(pdb.Spec.MaxUnavailable.String()).To(Equal("1"))
```

Ownership:

```go
Expect(metav1.IsControlledBy(pdb, modelService)).
    To(BeTrue())
```

Disable/delete behavior:

```go
currentModelService.Spec.PodDisruptionBudget.Enabled = false
Expect(k8sClient.Update(ctx, currentModelService)).To(Succeed())

Eventually(func() bool {
    err := k8sClient.Get(ctx, pdbKey, &policyv1.PodDisruptionBudget{})
    return apierrors.IsNotFound(err)
}).Should(BeTrue())
```

---

## 32. Build and test workflow

```bash
cd /mnt/data/ai-platform-operator

gofmt -w \
  api/v1alpha1/modelservice_types.go \
  internal/controller/modelservice_controller.go \
  internal/controller/modelservice_controller_test.go

make generate
make manifests
make build
make test
```

Expected outcome:

```text
controller tests pass
API deepcopy code regenerated
CRD schema regenerated
RBAC regenerated
controller binary builds successfully
```

Because the API schema changes, install the latest CRD:

```bash
make install
```

For local execution:

```bash
make run
```

---

# Part V — Live Cluster Validation

## 33. Apply the sample

```bash
kubectl apply \
  -f config/samples/platform_v1alpha1_modelservice.yaml
```

Watch the rollout:

```bash
kubectl rollout status deployment/fraud-model \
  -n ai-platform \
  --timeout=180s
```

```bash
kubectl get pods -n ai-platform -w
```

---

## 34. Verify requests and limits

```bash
kubectl get deployment fraud-model \
  -n ai-platform \
  -o jsonpath='{.spec.template.spec.containers[0].resources}{"\n"}'
```

Expected conceptually:

```text
requests: cpu=100m, memory=128Mi
limits:   cpu=500m, memory=512Mi
```

---

## 35. Verify probes

```bash
kubectl get deployment fraud-model \
  -n ai-platform \
  -o jsonpath='startup={.spec.template.spec.containers[0].startupProbe.httpGet.path}{"\n"}readiness={.spec.template.spec.containers[0].readinessProbe.httpGet.path}{"\n"}liveness={.spec.template.spec.containers[0].livenessProbe.httpGet.path}{"\n"}'
```

Verify startup timing:

```bash
kubectl get deployment fraud-model \
  -n ai-platform \
  -o jsonpath='period={.spec.template.spec.containers[0].startupProbe.periodSeconds}{"\n"}failureThreshold={.spec.template.spec.containers[0].startupProbe.failureThreshold}{"\n"}'
```

---

## 36. Verify Pod and container security

Pod-level:

```bash
kubectl get deployment fraud-model \
  -n ai-platform \
  -o jsonpath='{.spec.template.spec.securityContext}{"\n"}'
```

Container-level:

```bash
kubectl get deployment fraud-model \
  -n ai-platform \
  -o jsonpath='{.spec.template.spec.containers[0].securityContext}{"\n"}'
```

Expected conceptually:

```text
runAsNonRoot=true
runAsUser=101
runAsGroup=101
fsGroup=101
seccompProfile=RuntimeDefault
allowPrivilegeEscalation=false
readOnlyRootFilesystem=true
capabilities.drop=[ALL]
```

---

## 37. Runtime security tests

Select a Pod:

```bash
POD=$(kubectl get pods \
  -n ai-platform \
  -l app.kubernetes.io/instance=fraud-model \
  -o jsonpath='{.items[0].metadata.name}')
```

Confirm UID and GID:

```bash
kubectl exec -n ai-platform "$POD" -- id
```

Expected:

```text
uid=101
```

Test that the root filesystem is read-only:

```bash
kubectl exec -n ai-platform "$POD" -- \
  sh -c 'touch /should-fail'
```

Expected:

```text
Read-only file system
```

Verify writable temporary storage:

```bash
kubectl exec -n ai-platform "$POD" -- \
  sh -c 'echo working > /tmp/runtime-test && cat /tmp/runtime-test'
```

Expected:

```text
working
```

Verify PVC write access:

```bash
kubectl exec -n ai-platform "$POD" -- \
  sh -c 'echo model-storage-working > /models/security-test.txt && cat /models/security-test.txt'
```

---

## 38. Verify rollout and termination configuration

```bash
kubectl get deployment fraud-model \
  -n ai-platform \
  -o jsonpath='strategy={.spec.strategy}{"\n"}minReadySeconds={.spec.minReadySeconds}{"\n"}progressDeadlineSeconds={.spec.progressDeadlineSeconds}{"\n"}terminationGracePeriodSeconds={.spec.template.spec.terminationGracePeriodSeconds}{"\n"}preStop={.spec.template.spec.containers[0].lifecycle.preStop.exec.command}{"\n"}'
```

Expected conceptually:

```text
strategy.maxUnavailable=0
strategy.maxSurge=1
minReadySeconds=5
progressDeadlineSeconds=600
terminationGracePeriodSeconds=30
preStop=[/bin/sh -c sleep 5]
```

Test termination behavior:

```bash
POD=$(kubectl get pods \
  -n ai-platform \
  -l app.kubernetes.io/instance=fraud-model \
  -o jsonpath='{.items[0].metadata.name}')

time kubectl delete pod "$POD" -n ai-platform
```

The Pod should remain terminating for approximately the configured pre-stop delay before the container exits, while the Deployment creates a replacement.

---

## 39. Verify PodDisruptionBudget

```bash
kubectl get pdb -n ai-platform
```

```bash
kubectl describe pdb fraud-model -n ai-platform
```

```bash
kubectl get pdb fraud-model \
  -n ai-platform \
  -o yaml
```

Expected specification:

```yaml
spec:
  maxUnavailable: 1
  selector:
    matchLabels:
      app.kubernetes.io/instance: fraud-model
      app.kubernetes.io/managed-by: ai-platform-operator
      app.kubernetes.io/name: modelservice
```

Verify ownership:

```bash
kubectl get pdb fraud-model \
  -n ai-platform \
  -o jsonpath='{.metadata.ownerReferences[0].kind}{" / "}{.metadata.ownerReferences[0].name}{"\n"}'
```

Expected:

```text
ModelService / fraud-model
```

### Disable and recreate test

Set:

```yaml
podDisruptionBudget:
  enabled: false
  maxUnavailable: "1"
```

Apply and verify deletion:

```bash
kubectl apply -f config/samples/platform_v1alpha1_modelservice.yaml
kubectl get pdb fraud-model -n ai-platform
```

Set `enabled: true`, apply again, and verify the PDB is recreated.

---

# Part VI — Troubleshooting and Lessons Learned

## 40. CrashLoopBackOff after adding security

A live rollout failed when the existing sample still used:

```yaml
image: nginx:1.29-alpine
port: 80
```

while the new Pod template enforced non-root execution and dropped all Linux capabilities.

Symptoms:

```text
new Pod: CrashLoopBackOff
old Pods: Running
Deployment: desired replicas available, but only one new replica up-to-date
```

This occurred because the new ReplicaSet used the hardened Pod template while older Pods remained on the previous template.

Potential failure causes included:

```text
non-root process attempting to bind port 80
missing NET_BIND_SERVICE after dropping all capabilities
write attempts to /var/cache/nginx or /var/run
read-only root filesystem
forced UID incompatible with the image
```

Useful commands:

```bash
kubectl logs <pod> -n ai-platform --previous
```

```bash
kubectl describe pod <pod> -n ai-platform
```

```bash
kubectl get pod <pod> -n ai-platform \
  -o jsonpath='{.spec.securityContext}{"\n"}{.spec.containers[0].securityContext}{"\n"}'
```

### Resolution

The runtime sample was changed to an unprivileged NGINX image and non-privileged port:

```yaml
image: nginxinc/nginx-unprivileged:1.31-alpine
port: 8080
```

A writable `/tmp` `emptyDir` was added so strict read-only-root operation could succeed.

### Important testing lesson

`envtest` validates API behavior and generated Kubernetes objects, but it does not run the selected container image. A controller test can pass while a real workload fails at runtime.

Both are required:

```text
controller/envtest validation
+
live Kubernetes runtime validation
```

---

## 41. Image contract

The controller is image-agnostic, but each selected image must satisfy the declared contract:

```text
listen on spec.port
serve the configured startup path
serve the configured readiness path
serve the configured liveness path
run under the configured UID/GID
work with dropped capabilities
work with a read-only root filesystem
write only to explicitly mounted writable paths
respond correctly to termination
```

A real model-serving image should document these expectations.

---

## 42. Hard-coded values still present

Some values remain controller defaults rather than API fields:

```text
probe timeout values
readiness probe timing
liveness probe timing
imagePullPolicy
Service type ClusterIP
PVC access mode ReadWriteOnce
container name
named port
runtime /tmp mount
```

These are reasonable secure defaults for the current operator, but they can become configurable in later iterations if there is a concrete platform requirement.

---

## 43. Validation limitation

Individual Kubebuilder validation markers cannot enforce this cross-field rule:

```text
terminationGracePeriodSeconds > preStopDelaySeconds
```

That relationship should eventually be enforced using one of:

```text
CEL validation on the CRD
admission webhook validation
controller-side validation with a status condition
```

---

# Part VII — Completion Checklist

## 44. Implemented capabilities

```text
[✓] CPU requests
[✓] Memory requests
[✓] CPU limits
[✓] Memory limits
[✓] Configurable startup path
[✓] Configurable startup failure threshold
[✓] Configurable startup period
[✓] Configurable readiness path
[✓] Configurable liveness path
[✓] Startup probe
[✓] Readiness probe
[✓] Liveness probe
[✓] Container security context
[✓] Pod security context
[✓] Run as non-root
[✓] Explicit runtime UID
[✓] Explicit runtime GID
[✓] Volume fsGroup
[✓] Read-only root filesystem
[✓] Disable privilege escalation
[✓] Drop all Linux capabilities
[✓] RuntimeDefault seccomp profile
[✓] Writable /tmp emptyDir
[✓] Persistent model-storage mount
[✓] Termination grace period
[✓] preStop graceful-shutdown delay
[✓] RollingUpdate strategy
[✓] maxUnavailable
[✓] maxSurge
[✓] minReadySeconds
[✓] progressDeadlineSeconds
[✓] PodDisruptionBudget API
[✓] PodDisruptionBudget reconciliation
[✓] PodDisruptionBudget RBAC
[✓] PodDisruptionBudget owner reference
[✓] PDB deletion when disabled
[✓] Tests for create and update behavior
[✓] Live runtime validation
```

---

## 45. Final operational outcome

The `ModelService` operator now converts a concise platform-facing resource into a hardened and availability-aware Kubernetes workload.

The implementation protects against:

```text
unscheduled resource consumption
slow-starting model containers
incorrect health endpoint assumptions
root container execution
writable image filesystems
Linux capability abuse
privilege escalation
unfiltered syscall access
abrupt termination
unsafe rolling-update settings
planned voluntary disruptions
configuration drift in owned resources
```

It also preserves extensibility: platform users provide image-specific settings, while the operator consistently applies Kubernetes production controls.
