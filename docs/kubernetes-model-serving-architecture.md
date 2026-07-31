# Kubernetes-Based Model Serving Platform

## 1. Purpose of the Architecture

This architecture provides a self-service platform for deploying trained machine-learning models as secure, scalable, and observable inference services on Kubernetes.

The main user experience is:

```text
Select or register a model
→ request deployment
→ platform validates and provisions it
→ receive a prediction endpoint
→ monitor, update, scale, and roll back the model
```

The platform hides most of the Kubernetes complexity from data scientists and ML engineers. They do not need to manually create Deployments, Services, HTTPRoutes, autoscaling rules, security policies, or monitoring configuration.

The architecture separates two different responsibilities:

- **Platform control plane** — manages model deployment and lifecycle.
- **Inference data plane** — handles live prediction requests.

---

# 2. Correct Architecture Sketch

```text
                         PLATFORM USERS
                Data Scientists / ML Engineers
                              │
                              │ HTTPS
                              ▼

┌────────────────────────── KUBERNETES CLUSTER ──────────────────────────┐
│                                                                       │
│                      PLATFORM CONTROL PLANE                           │
│                                                                       │
│  Ingress                                                              │
│     │                                                                 │
│     ▼                                                                 │
│  AuthN / AuthZ ────────────────────────────────────────────────────────┼──► OIDC /
│     │                                                                 │    Identity Provider
│     ▼                                                                 │
│  REST API                                                             │
│     │                                                                 │
│     │ create / update ModelService                                    │
│     ▼                                                                 │
│  Kubernetes API Server                                                │
│     │                                                                 │
│     ├──────────────────────────────► ModelService CR                   │
│     │                                  desired state                  │
│     │                                                                 │
│     ◄──────────────────────────────► Go Operator                       │
│          watch / reconcile / update status                            │
│                                         │                             │
│                                         ├─────────────────────────────┼──► MLflow
│                                         │                                 Model Registry
│                                         │ resolve model/version       │
│                                         │                             │
│                                         ▼                             │
│                               KServe InferenceService                 │
│                                         │                             │
│                                         │ KServe provisions           │
│                                         ▼                             │
│                                                                       │
│                        INFERENCE DATA PLANE                           │
│                                                                       │
│                         Inference Gateway / HTTPRoute                 │
│                                         │                             │
│                                         ▼                             │
│                              Predictor Service                        │
│                                         │                             │
│                                         ▼                             │
│                               Predictor Pods                          │
│                                         │                             │
│                                         ├─────────────────────────────┼──► Object Storage
│                                         │                                 model artifacts
│                                         │                             │
│  Kubernetes node / container runtime ──┼─────────────────────────────┼──► Container Registry
│                                                                           runtime image
│                                                                       │
└───────────────────────────────────────────────────────────────────────┘
                              ▲
                              │ HTTPS prediction request
                              │
                    APPLICATION CLIENTS
```

---

# 3. Main Architectural Principles

## 3.1 Declarative desired state

The platform does not deploy models through a long sequence of imperative scripts.

Instead, it creates a Kubernetes custom resource called `ModelService`.

Example:

```yaml
apiVersion: platform.example.com/v1alpha1
kind: ModelService
metadata:
  name: fraud-detector
  namespace: ml-team
spec:
  model:
    registry: mlflow
    name: fraud-detector
    version: "12"

  resources:
    requests:
      cpu: "500m"
      memory: "1Gi"
    limits:
      cpu: "2"
      memory: "4Gi"

  scaling:
    minReplicas: 1
    maxReplicas: 5

  exposure:
    visibility: internal
```

This resource expresses what the user wants.

```text
ModelService.spec = desired state
ModelService.status = observed state
```

The operator continuously compares the desired state with the real cluster state and corrects differences.

---

## 3.2 Reconciliation instead of one-time deployment

The Go operator does not run once and stop.

It continuously performs a reconciliation loop:

```text
Observe current state
→ compare with desired state
→ create or update missing resources
→ check readiness
→ update status
→ repeat when something changes
```

This provides self-healing behavior.

For example:

```text
Predictor Service is deleted manually
→ operator or KServe detects drift
→ Service is recreated
```

This is one of the key differences between an operator and a simple deployment script.

---

## 3.3 Separation of platform control plane and inference data plane

The platform control plane is responsible for management:

```text
deploy
update
validate
authorize
observe
roll back
delete
```

The inference data plane is responsible for live traffic:

```text
receive prediction request
route request
run model inference
return prediction response
```

The REST API does not process model predictions.

The Go operator does not process model predictions.

The predictor Pods do not create Kubernetes resources.

Each component has a clear responsibility.

---

## 3.4 Kubernetes API Server as the central control interface

The REST API and the Go operator do not communicate directly.

Both communicate through the Kubernetes API Server.

```text
REST API
→ Kubernetes API Server
→ ModelService CR
```

```text
Go Operator
↔ Kubernetes API Server
```

The operator uses the API Server to:

- watch `ModelService` resources;
- read desired configuration;
- create a KServe `InferenceService`;
- create or manage supporting resources;
- read KServe readiness;
- update `ModelService.status`.

This follows the standard Kubernetes controller pattern.

---

## 3.5 Platform abstraction over KServe

KServe already provides model-serving functionality.

The platform should not unnecessarily duplicate it.

The responsibility split is:

| Component | Main responsibility |
|---|---|
| REST API | User-friendly platform interface |
| ModelService CR | Platform-level desired state |
| Go Operator | Reconciliation, governance, orchestration |
| MLflow | Model registry and metadata |
| KServe | Model-serving lifecycle |
| Kubernetes | Scheduling, networking, recovery |
| Object Storage | Model artifact storage |
| Container Registry | Runtime image storage |

The Go operator creates a KServe `InferenceService`.

KServe then manages lower-level serving resources such as:

- predictor Pods;
- predictor Service;
- readiness;
- routing integration;
- autoscaling integration;
- model runtime lifecycle.

---

## 3.6 Externalized model artifacts and runtime images

The trained model and the serving runtime are different things.

### Model artifact

Examples:

```text
model.pkl
model.onnx
weights.pt
SavedModel
MLmodel
config.json
```

These are stored in object storage.

### Runtime image

The runtime image contains:

- Python or another runtime;
- ML framework;
- KServe model server;
- dependencies;
- health endpoints;
- inference logic.

The runtime image is stored in the container registry.

At startup:

```text
Kubernetes node pulls runtime image
Predictor Pod starts
Predictor Pod downloads model artifact
Predictor Pod loads model into memory
Predictor becomes Ready
```

---

# 4. Component-by-Component Explanation

## 4.1 Platform Users

Typical platform users are:

- data scientists;
- ML engineers;
- platform engineers;
- authorized application teams.

They use the platform to:

- deploy a model;
- update a version;
- inspect deployment status;
- retrieve the inference endpoint;
- roll back a failed release;
- delete a model service.

They are outside the Kubernetes cluster.

---

## 4.2 Ingress

Ingress exposes the platform REST API over HTTPS.

Example:

```text
https://platform.example.com/api/v1/model-services
```

It routes incoming platform-management requests to the REST API.

It may also terminate TLS, depending on the chosen ingress or gateway implementation.

The platform Ingress is part of the control path.

---

## 4.3 AuthN / AuthZ

Authentication and authorization are separate concerns.

### Authentication

Authentication answers:

```text
Who is the caller?
```

### Authorization

Authorization answers:

```text
What is the caller allowed to do?
```

Example permissions:

```text
Data scientist:
- create ModelService in own namespace
- update own model versions
- view status

Platform administrator:
- manage cluster-wide policies
- approve runtimes
- set quotas
- manage namespaces
```

The authentication component communicates with an external OIDC identity provider.

Examples include:

- Keycloak;
- Okta;
- Microsoft Entra ID;
- Google Identity;
- another OpenID Connect provider.

---

## 4.4 REST API

The REST API is the user-facing platform control plane.

Example endpoints:

```http
POST   /api/v1/model-services
GET    /api/v1/model-services
GET    /api/v1/model-services/{name}
PATCH  /api/v1/model-services/{name}
DELETE /api/v1/model-services/{name}
POST   /api/v1/model-services/{name}/rollback
```

The REST API should perform:

- request validation;
- authentication context handling;
- authorization checks;
- namespace or tenant mapping;
- policy checks;
- audit logging;
- creation and update of `ModelService` resources;
- retrieval of `ModelService.status`.

The REST API should not directly create raw Deployments or Pods.

Its main output is the `ModelService` custom resource.

---

## 4.5 Kubernetes API Server

The Kubernetes API Server is the authoritative interface for cluster state.

The REST API writes desired state through it.

The Go operator watches and modifies resources through it.

The KServe controller also watches resources through it.

This gives a consistent event-driven workflow.

---

## 4.6 ModelService Custom Resource

`ModelService` is the platform abstraction presented to users.

It may contain:

```text
model reference
model version
resource requirements
scaling limits
visibility
runtime type
ownership
environment
security settings
```

Example status:

```yaml
status:
  phase: Ready
  endpoint: https://fraud-detector.models.example.com
  activeVersion: "12"
  readyReplicas: 2
  observedGeneration: 4

  conditions:
    - type: ModelResolved
      status: "True"

    - type: RuntimeReady
      status: "True"

    - type: EndpointReady
      status: "True"

    - type: Ready
      status: "True"
```

`Status / Conditions` is not a separate workload.

It is stored under `ModelService.status`.

---

## 4.7 Go Operator

The Go operator is the main orchestration component.

Its reconciliation logic is approximately:

```text
Read ModelService
→ validate desired state
→ resolve model in MLflow
→ obtain artifact URI and model metadata
→ build desired KServe InferenceService
→ create or update it through Kubernetes API Server
→ inspect readiness
→ update ModelService.status
```

The operator should be:

- idempotent;
- retry-safe;
- event-driven;
- status-aware;
- able to handle deletion;
- able to correct drift.

Important operator concepts include:

- owner references;
- finalizers;
- status conditions;
- observed generation;
- exponential backoff;
- conflict handling;
- reconciliation retries.

---

## 4.8 MLflow Model Registry

MLflow stores model metadata and lifecycle information.

The operator may query MLflow for:

- model name;
- model version;
- alias such as `champion`;
- framework;
- model signature;
- artifact URI;
- training metrics;
- model tags.

Example:

```text
Model: fraud-detector
Version: 12
Alias: champion
Framework: sklearn
Artifact URI: s3://mlflow-artifacts/fraud-detector/12/model
```

The operator uses this information to create the KServe resource.

---

## 4.9 KServe InferenceService

The KServe `InferenceService` describes how a model should be served.

Example:

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: fraud-detector
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      storageUri: s3://mlflow-artifacts/fraud-detector/12/model
      resources:
        requests:
          cpu: "500m"
          memory: "1Gi"
```

KServe then provisions and manages the serving workload.

Conceptually:

```text
InferenceService
→ Predictor Service
→ Predictor Pods
→ prediction endpoint
```

KServe may also integrate with:

- autoscaling;
- revisions;
- traffic splitting;
- serverless serving;
- standard inference protocols;
- readiness and health reporting.

---

## 4.10 Inference Gateway / HTTPRoute

The inference gateway is the entry point for prediction traffic.

Example:

```text
https://models.example.com/fraud-detector
```

The route maps the external model endpoint to the correct predictor Service.

The runtime path is:

```text
Application Client
→ Inference Gateway / HTTPRoute
→ Predictor Service
→ Predictor Pod
```

The prediction response returns along the reverse path.

The inference gateway should be separated from the platform-management REST API because they serve different traffic patterns and security requirements.

---

## 4.11 Predictor Service

The Kubernetes Service provides a stable network identity for the predictor Pods.

It selects the correct Pods using labels.

Example conceptually:

```text
Service
→ Pod A
→ Pod B
→ Pod C
```

The Service is not the model runtime itself.

It routes traffic to healthy predictor Pods.

---

## 4.12 Predictor Pods

Predictor Pods perform the actual model inference.

At startup they:

```text
start serving runtime
→ obtain object-storage credentials
→ download model artifact
→ deserialize or load model
→ perform warm-up
→ report readiness
```

At runtime they:

```text
receive request
→ validate input
→ run inference
→ return prediction
→ expose metrics
```

Useful endpoints may include:

```text
/live
/ready
/metrics
/v1/models
/predict
```

---

## 4.13 Object Storage

Object storage holds the model artifact.

Examples include:

- Amazon S3;
- Google Cloud Storage through compatible integration;
- MinIO;
- Ceph Object Gateway;
- another S3-compatible service.

The predictor workload reads the model artifact from object storage.

Access should normally use:

- workload identity;
- service account permissions;
- short-lived credentials;
- secret references.

Credentials should not be hard-coded into the `ModelService`.

---

## 4.14 Container Registry

The container registry holds runtime images.

Examples include:

- Docker Hub;
- GitHub Container Registry;
- GitLab Container Registry;
- Google Artifact Registry;
- Amazon ECR;
- Harbor.

The exact image-pull flow is:

```text
Kubernetes node container runtime
→ Container Registry
→ pull image
→ start Predictor Pod
```

In a simplified diagram, this may be shown as:

```text
Predictor Pods → Container Registry
```

But technically the node's container runtime performs the pull.

---

## 4.15 Application Clients

Application clients consume the deployed model.

Examples include:

- another microservice;
- a web backend;
- a mobile backend;
- a fraud-detection service;
- an IoT backend;
- a test client using `curl`.

They are outside the Kubernetes cluster.

They send prediction traffic to the inference gateway.

Example:

```http
POST /v1/models/fraud-detector:predict
Content-Type: application/json
Authorization: Bearer <token>
```

```json
{
  "instances": [
    {
      "amount": 1500,
      "country": "DE"
    }
  ]
}
```

---

# 5. End-to-End Deployment Flow

## Step 1: User requests deployment

The user selects:

```text
model name
model version
CPU and memory
minimum and maximum replicas
visibility
```

The request enters through the platform Ingress.

---

## Step 2: Authentication and authorization

The platform validates the user through the OIDC provider.

It then checks whether the user is permitted to deploy into the target namespace.

---

## Step 3: REST API creates ModelService

The REST API validates the request and creates a `ModelService` through the Kubernetes API Server.

---

## Step 4: Operator detects the new resource

The Go operator receives a watch event from the Kubernetes API Server.

It begins reconciliation.

---

## Step 5: Operator resolves model metadata

The operator queries MLflow to identify:

```text
exact model version
framework
artifact URI
model metadata
```

---

## Step 6: Operator creates KServe InferenceService

The operator translates the platform-level `ModelService` into a KServe `InferenceService`.

---

## Step 7: KServe provisions the serving resources

KServe creates or manages:

```text
Predictor Service
Predictor Pods
routing integration
readiness state
autoscaling integration
```

---

## Step 8: Runtime image is pulled

The Kubernetes node pulls the runtime image from the container registry.

---

## Step 9: Model artifact is loaded

The predictor Pod downloads the model artifact from object storage and loads it.

---

## Step 10: Readiness is reported

When the runtime and model are ready, KServe updates the `InferenceService` status.

---

## Step 11: Operator updates ModelService status

The operator translates KServe status into user-facing platform status.

Example:

```text
Pending
→ ResolvingModel
→ Provisioning
→ LoadingModel
→ Ready
```

---

## Step 12: User receives endpoint

The REST API reads `ModelService.status` and returns:

```json
{
  "name": "fraud-detector",
  "phase": "Ready",
  "endpoint": "https://fraud-detector.models.example.com",
  "activeVersion": "12"
}
```

---

# 6. End-to-End Prediction Flow

The prediction path is separate from the deployment path.

```text
Application Client
→ HTTPS
→ Inference Gateway / HTTPRoute
→ Predictor Service
→ Predictor Pod
→ Model inference
→ prediction response
```

The REST API and Go operator are not part of the live prediction request path.

This prevents management components from becoming a bottleneck for inference traffic.

---

# 7. Status and Lifecycle Management

A useful lifecycle model is:

```text
Pending
→ ResolvingModel
→ Provisioning
→ LoadingModel
→ Ready
```

Failure states may include:

```text
Degraded
Failed
Unknown
```

Example conditions:

```yaml
conditions:
  - type: ModelResolved
    status: "True"

  - type: RuntimeReady
    status: "True"

  - type: EndpointReady
    status: "True"

  - type: Ready
    status: "True"
```

The operator should set `observedGeneration` so users know whether status corresponds to the latest specification.

---

# 8. Security Principles

## 8.1 Authentication and authorization

Use OIDC for user authentication and RBAC or platform policies for authorization.

## 8.2 Least privilege

Each component should have only the permissions it needs.

Examples:

```text
REST API:
- create and read ModelService resources

Go Operator:
- watch ModelService
- manage InferenceService
- update status

Predictor Pod:
- read only its model artifact
```

## 8.3 Secure workloads

Predictor Pods should use secure defaults:

```yaml
securityContext:
  runAsNonRoot: true
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop:
      - ALL
```

## 8.4 Secret management

Use:

- External Secrets Operator;
- Vault;
- cloud secret manager;
- workload identity.

Avoid embedding credentials in custom resources.

## 8.5 Network isolation

Use NetworkPolicies to restrict:

```text
who may call predictor Pods
which external services Pods may access
whether models are internal or external
```

## 8.6 Image trust

Use:

- approved registries;
- image scanning;
- signed images;
- immutable image digests;
- admission policies.

---

# 9. Reliability Principles

## 9.1 Health probes

The runtime should distinguish:

```text
process alive
model loaded
service ready
```

## 9.2 Autoscaling

KServe or Kubernetes can scale based on:

- CPU;
- memory;
- request concurrency;
- request rate;
- custom metrics.

## 9.3 Self-healing

Kubernetes recreates failed Pods.

The operator and KServe restore missing managed resources.

## 9.4 Safe rollout

A new model version should be deployed without immediately replacing the last working version.

Possible strategy:

```text
deploy candidate
→ verify readiness
→ send limited traffic
→ observe errors and latency
→ promote or roll back
```

## 9.5 Idempotency

Repeated reconciliation should not create duplicate or unnecessary updates.

---

# 10. Observability Principles

The platform should expose:

## Infrastructure metrics

```text
replica count
CPU
memory
Pod restarts
pending Pods
model load duration
```

## Serving metrics

```text
request rate
error rate
p50 latency
p95 latency
p99 latency
inference duration
queue depth
```

## Model-service status

```text
model version
endpoint
ready replicas
deployment phase
last successful rollout
```

Useful tools may include:

- Prometheus;
- Grafana;
- OpenTelemetry;
- Loki or another logging backend;
- Alertmanager.

---

# 11. Failure Scenarios

## MLflow unavailable

```text
Operator cannot resolve model
→ ModelService remains not ready
→ status reports ModelResolutionFailed
→ operator retries
```

## Object storage unavailable

```text
Predictor cannot download model
→ readiness fails
→ traffic is not sent
→ status reports ModelLoadFailed
```

## Container image unavailable

```text
Pod enters ImagePullBackOff
→ KServe is not ready
→ operator reports runtime provisioning failure
```

## New model version fails

```text
candidate does not become ready
→ old version remains active
→ candidate is marked failed
→ rollback or retry occurs
```

## Predictor Pod crashes

```text
Kubernetes restarts or replaces Pod
→ Service routes only to healthy Pods
```

---

# 12. Product Value

The platform solves the gap between a trained model and a production-ready service.

Without the platform:

```text
build serving API
write Dockerfile
write Deployment
write Service
configure route
configure TLS
configure autoscaling
configure monitoring
configure secrets
configure rollback
```

With the platform:

```text
select model
→ submit ModelService
→ receive secure inference endpoint
```

The user gets:

- self-service deployment;
- standardized security;
- consistent observability;
- automatic scaling;
- model-version tracking;
- status visibility;
- rollback support;
- reduced dependency on platform engineers.

---

# 13. Final Summary

The architecture follows a clean Kubernetes-native pattern:

```text
Platform User
→ REST API
→ ModelService
→ Go Operator
→ MLflow
→ KServe
→ Predictor Service and Pods
```

Prediction traffic follows a separate path:

```text
Application Client
→ Inference Gateway
→ Predictor Service
→ Predictor Pod
→ Prediction Response
```

The key principles are:

- declarative desired state;
- continuous reconciliation;
- separation of management and runtime traffic;
- Kubernetes API Server as the central control interface;
- KServe for model-serving lifecycle;
- MLflow for model metadata;
- object storage for model artifacts;
- container registry for runtime images;
- secure, observable, and scalable defaults.

The final product promise is:

> Register or select a trained model, submit one deployment request, and receive a secure, scalable, observable inference endpoint without manually building Kubernetes infrastructure.
