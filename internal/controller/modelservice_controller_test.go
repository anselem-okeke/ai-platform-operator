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
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("ModelService Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			resourceName      = "test-resource"
			resourceNamespace = "default"
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ModelService")

			modelService := &platformv1alpha1.ModelService{}
			err := k8sClient.Get(
				ctx,
				typeNamespacedName,
				modelService,
			)

			if errors.IsNotFound(err) {
				resource := &platformv1alpha1.ModelService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: resourceNamespace,
					},
					Spec: platformv1alpha1.ModelServiceSpec{
						Image:    "nginx:1.29-alpine",
						Replicas: 1,
						Port:     80,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse(
									"128Mi",
								),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse(
									"512Mi",
								),
							},
						},

						Health: &platformv1alpha1.ModelServiceHealth{
							StartupPath:             "/startup",
							StartupFailureThreshold: 40,
							StartupPeriodSeconds:    5,
							ReadinessPath:           "/ready",
							LivenessPath:            "/live",
						},

						Rollout: &platformv1alpha1.ModelServiceRollout{
							TerminationGracePeriodSeconds: 45,
							PreStopDelaySeconds:           7,
							MinReadySeconds:               10,
							ProgressDeadlineSeconds:       900,
							MaxUnavailable:                "0",
							MaxSurge:                      "2",
						},

						PodDisruptionBudget: &platformv1alpha1.ModelServicePodDisruptionBudget{
							Enabled:        true,
							MaxUnavailable: "1",
						},

						NetworkPolicy: &platformv1alpha1.ModelServiceNetworkPolicy{
							Enabled:                   true,
							AllowSameNamespaceIngress: true,
							AllowDNSEgress:            true,
						},

						Exposure: &platformv1alpha1.ModelServiceExposure{
							Enabled:                   true,
							Hostname:                  "fraud-model.example.test",
							PathPrefix:                "/predict",
							GatewayName:               "shared-gateway",
							GatewayNamespace:          "gateway-system",
							GatewaySectionName:        "http",
							GatewayDataPlaneNamespace: gatewayDataPlaneNamespace,
						},

						Security: &platformv1alpha1.ModelServiceSecurity{
							RunAsNonRoot:           true,
							RunAsUser:              101,
							RunAsGroup:             101,
							FSGroup:                101,
							ReadOnlyRootFilesystem: true,
						},

						Storage: &platformv1alpha1.ModelServiceStorage{
							Enabled:   true,
							Size:      "1Gi",
							MountPath: "/models",
						},
					},
				}

				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		})

		AfterEach(func() {
			By("cleaning up the ModelService")

			modelService := &platformv1alpha1.ModelService{}
			err := k8sClient.Get(
				ctx,
				typeNamespacedName,
				modelService,
			)

			if err == nil {
				Expect(
					k8sClient.Delete(ctx, modelService),
				).To(Succeed())
			} else {
				Expect(errors.IsNotFound(err)).To(BeTrue())
			}

			By("cleaning up the Deployment if it still exists")

			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(
				ctx,
				typeNamespacedName,
				deployment,
			)

			if err == nil {
				Expect(
					k8sClient.Delete(ctx, deployment),
				).To(Succeed())
			} else {
				Expect(errors.IsNotFound(err)).To(BeTrue())
			}

			By("cleaning up the ServiceAccount if it still exists")

			serviceAccount := &corev1.ServiceAccount{}

			err = k8sClient.Get(
				ctx,
				typeNamespacedName,
				serviceAccount,
			)

			if err == nil {
				Expect(
					k8sClient.Delete(ctx, serviceAccount),
				).To(Succeed())
			} else {
				Expect(errors.IsNotFound(err)).To(BeTrue())
			}

			By("cleaning up the NetworkPolicy if it still exists")

			networkPolicy := &networkingv1.NetworkPolicy{}

			err = k8sClient.Get(
				ctx,
				typeNamespacedName,
				networkPolicy,
			)

			if err == nil {
				Expect(
					k8sClient.Delete(ctx, networkPolicy),
				).To(Succeed())
			} else {
				Expect(errors.IsNotFound(err)).To(BeTrue())
			}

			By("cleaning up the PodDisruptionBudget if it still exists")

			pdb := &policyv1.PodDisruptionBudget{}

			err = k8sClient.Get(
				ctx,
				typeNamespacedName,
				pdb,
			)

			if err == nil {
				Expect(
					k8sClient.Delete(ctx, pdb),
				).To(Succeed())
			} else {
				Expect(errors.IsNotFound(err)).To(BeTrue())
			}
		})

		It(
			"should propagate ModelService specification changes to managed resources",
			func() {
				By("performing the initial reconciliation")

				controllerReconciler := &ModelServiceReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				_, err := controllerReconciler.Reconcile(
					ctx,
					reconcile.Request{
						NamespacedName: typeNamespacedName,
					},
				)
				Expect(err).NotTo(HaveOccurred())

				By("updating the ModelService specification")

				modelService := &platformv1alpha1.ModelService{}

				Expect(
					k8sClient.Get(
						ctx,
						typeNamespacedName,
						modelService,
					),
				).To(Succeed())

				modelService.Spec.Image = "nginx:1.29"
				modelService.Spec.Replicas = 3
				modelService.Spec.Port = 8081

				modelService.Spec.Health.StartupPath = "/health/startup"
				modelService.Spec.Health.StartupFailureThreshold = 60
				modelService.Spec.Health.StartupPeriodSeconds = 3
				modelService.Spec.Health.ReadinessPath = "/health/ready"
				modelService.Spec.Health.LivenessPath = "/health/live"

				modelService.Spec.Rollout.
					TerminationGracePeriodSeconds = 60

				modelService.Spec.Rollout.
					PreStopDelaySeconds = 10

				modelService.Spec.Rollout.
					MinReadySeconds = 15

				modelService.Spec.Rollout.
					ProgressDeadlineSeconds = 1200
				modelService.Spec.Rollout.MaxUnavailable = "1"
				modelService.Spec.Rollout.MaxSurge = "25%"

				modelService.Spec.PodDisruptionBudget.
					MaxUnavailable = "25%"

				modelService.Spec.Exposure.Hostname =
					"updated-model.example.test"

				modelService.Spec.Exposure.PathPrefix =
					"/v2/predict"

				modelService.Spec.Exposure.GatewaySectionName =
					"public-http"

				modelService.Spec.Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse(
							"256Mi",
						),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse(
							"1Gi",
						),
					},
				}

				modelService.Spec.Storage.MountPath = "/model-data"

				Expect(
					k8sClient.Update(ctx, modelService),
				).To(Succeed())

				By("reconciling the updated ModelService")

				_, err = controllerReconciler.Reconcile(
					ctx,
					reconcile.Request{
						NamespacedName: typeNamespacedName,
					},
				)
				Expect(err).NotTo(HaveOccurred())

				// -------------------------------------------------------------
				// Verify ServiceAccount
				// -------------------------------------------------------------

				serviceAccount := &corev1.ServiceAccount{}

				Eventually(func() error {
					return k8sClient.Get(
						ctx,
						typeNamespacedName,
						serviceAccount,
					)
				}).Should(Succeed())

				Expect(serviceAccount.Labels).
					To(Equal(labelsForModelService(modelService)))

				Expect(serviceAccount.AutomountServiceAccountToken).
					NotTo(BeNil())

				Expect(*serviceAccount.AutomountServiceAccountToken).
					To(BeFalse())

				Expect(metav1.IsControlledBy(
					serviceAccount,
					modelService,
				)).To(BeTrue())

				// -------------------------------------------------------------
				// Verify HTTPRoute update
				// -------------------------------------------------------------

				httpRoute := &gatewayv1.HTTPRoute{}

				Eventually(func() error {
					return k8sClient.Get(
						ctx,
						typeNamespacedName,
						httpRoute,
					)
				}).Should(Succeed())

				Expect(httpRoute.Spec.ParentRefs).
					To(HaveLen(1))

				parentRef := httpRoute.Spec.ParentRefs[0]

				Expect(string(parentRef.Name)).
					To(Equal("shared-gateway"))

				Expect(parentRef.Namespace).
					NotTo(BeNil())

				Expect(string(*parentRef.Namespace)).
					To(Equal("gateway-system"))

				Expect(parentRef.SectionName).
					NotTo(BeNil())

				Expect(string(*parentRef.SectionName)).
					To(Equal("public-http"))

				Expect(httpRoute.Spec.Hostnames).
					To(Equal([]gatewayv1.Hostname{
						"updated-model.example.test",
					}))

				Expect(httpRoute.Spec.Rules).
					To(HaveLen(1))

				Expect(httpRoute.Spec.Rules[0].Matches).
					To(HaveLen(1))

				pathMatch :=
					httpRoute.Spec.Rules[0].Matches[0].Path

				Expect(pathMatch).
					NotTo(BeNil())

				Expect(pathMatch.Type).
					NotTo(BeNil())

				Expect(*pathMatch.Type).
					To(Equal(gatewayv1.PathMatchPathPrefix))

				Expect(pathMatch.Value).
					NotTo(BeNil())

				Expect(*pathMatch.Value).
					To(Equal("/v2/predict"))

				Expect(httpRoute.Spec.Rules[0].BackendRefs).
					To(HaveLen(1))

				backendRef :=
					httpRoute.Spec.Rules[0].
						BackendRefs[0].
						BackendRef

				Expect(string(backendRef.Name)).
					To(Equal(resourceName))

				Expect(backendRef.Port).
					NotTo(BeNil())

				Expect(*backendRef.Port).
					To(Equal(int32(8081)))

				Expect(metav1.IsControlledBy(
					httpRoute,
					modelService,
				)).To(BeTrue())

				// -------------------------------------------------------------
				// Verify Deployment update
				// -------------------------------------------------------------

				deployment := &appsv1.Deployment{}

				Eventually(func() error {
					return k8sClient.Get(
						ctx,
						typeNamespacedName,
						deployment,
					)
				}).Should(Succeed())

				podSpec := deployment.Spec.Template.Spec

				Expect(podSpec.ServiceAccountName).
					To(Equal(resourceName))

				Expect(podSpec.AutomountServiceAccountToken).
					NotTo(BeNil())

				Expect(*podSpec.AutomountServiceAccountToken).
					To(BeFalse())

				// -------------------------------------------------------------
				// Verify updated rollout configuration
				// -------------------------------------------------------------

				Expect(deployment.Spec.Strategy.Type).
					To(Equal(
						appsv1.RollingUpdateDeploymentStrategyType,
					))

				Expect(deployment.Spec.Strategy.RollingUpdate).
					NotTo(BeNil())

				Expect(
					deployment.Spec.Strategy.
						RollingUpdate.
						MaxUnavailable,
				).NotTo(BeNil())

				Expect(
					deployment.Spec.Strategy.
						RollingUpdate.
						MaxUnavailable.String(),
				).To(Equal("1"))

				Expect(
					deployment.Spec.Strategy.
						RollingUpdate.
						MaxSurge,
				).NotTo(BeNil())

				Expect(
					deployment.Spec.Strategy.
						RollingUpdate.
						MaxSurge.String(),
				).To(Equal("25%"))

				Expect(deployment.Spec.MinReadySeconds).
					To(Equal(int32(15)))

				Expect(deployment.Spec.ProgressDeadlineSeconds).
					NotTo(BeNil())

				Expect(*deployment.Spec.ProgressDeadlineSeconds).
					To(Equal(int32(1200)))

				Expect(
					deployment.Spec.Template.Spec.
						TerminationGracePeriodSeconds,
				).NotTo(BeNil())

				Expect(
					*deployment.Spec.Template.Spec.
						TerminationGracePeriodSeconds,
				).To(Equal(int64(60)))

				Expect(deployment.Spec.Replicas).NotTo(BeNil())
				Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))

				Expect(
					deployment.Spec.Template.Spec.Containers,
				).To(HaveLen(1))

				container :=
					deployment.Spec.Template.Spec.Containers[0]

				Expect(container.Lifecycle).NotTo(BeNil())
				Expect(container.Lifecycle.PreStop).NotTo(BeNil())
				Expect(container.Lifecycle.PreStop.Exec).NotTo(BeNil())

				Expect(container.Lifecycle.PreStop.Exec.Command).
					To(Equal([]string{
						"/bin/sh",
						"-c",
						"sleep 10",
					}))

				Expect(container.Image).To(Equal("nginx:1.29"))

				Expect(container.Ports).To(HaveLen(1))
				Expect(container.Ports[0].ContainerPort).
					To(Equal(int32(8081)))

				Expect(container.StartupProbe).NotTo(BeNil())
				Expect(container.StartupProbe.HTTPGet).NotTo(BeNil())

				Expect(container.StartupProbe.HTTPGet.Path).
					To(Equal("/health/startup"))

				Expect(container.StartupProbe.FailureThreshold).
					To(Equal(int32(60)))

				Expect(container.StartupProbe.PeriodSeconds).
					To(Equal(int32(3)))

				Expect(container.ReadinessProbe).NotTo(BeNil())
				Expect(container.ReadinessProbe.HTTPGet).NotTo(BeNil())

				Expect(container.ReadinessProbe.HTTPGet.Path).
					To(Equal("/health/ready"))

				Expect(container.LivenessProbe).NotTo(BeNil())
				Expect(container.LivenessProbe.HTTPGet).NotTo(BeNil())

				Expect(container.LivenessProbe.HTTPGet.Path).
					To(Equal("/health/live"))

				updatedCPURequest :=
					container.Resources.Requests[corev1.ResourceCPU]

				Expect(updatedCPURequest.String()).
					To(Equal("200m"))

				updatedMemoryRequest :=
					container.Resources.Requests[corev1.ResourceMemory]

				Expect(updatedMemoryRequest.String()).
					To(Equal("256Mi"))

				updatedCPULimit :=
					container.Resources.Limits[corev1.ResourceCPU]

				Expect(updatedCPULimit.String()).
					To(Equal("1"))

				updatedMemoryLimit :=
					container.Resources.Limits[corev1.ResourceMemory]

				Expect(updatedMemoryLimit.String()).
					To(Equal("1Gi"))

				Expect(container.VolumeMounts).To(HaveLen(2))

				var runtimeTmpMount *corev1.VolumeMount
				var modelStorageMount *corev1.VolumeMount

				for index := range container.VolumeMounts {
					mount := &container.VolumeMounts[index]

					switch mount.Name {
					case runtimeTmpVolumeName:
						runtimeTmpMount = mount

					case modelStorageName:
						modelStorageMount = mount
					}
				}

				Expect(runtimeTmpMount).NotTo(BeNil())
				Expect(runtimeTmpMount.MountPath).
					To(Equal(runtimeTmpMountPath))

				Expect(modelStorageMount).NotTo(BeNil())
				Expect(modelStorageMount.MountPath).
					To(Equal("/model-data"))

				Expect(deployment.Spec.Template.Spec.Volumes).
					To(HaveLen(2))

				var runtimeTmpVolume *corev1.Volume
				var modelStorageVolume *corev1.Volume

				volumes := deployment.Spec.Template.Spec.Volumes

				for index := range volumes {
					volume := &volumes[index]

					switch volume.Name {
					case runtimeTmpVolumeName:
						runtimeTmpVolume = volume

					case modelStorageName:
						modelStorageVolume = volume
					}
				}

				Expect(runtimeTmpVolume).NotTo(BeNil())
				Expect(runtimeTmpVolume.EmptyDir).NotTo(BeNil())

				Expect(modelStorageVolume).NotTo(BeNil())
				Expect(modelStorageVolume.PersistentVolumeClaim).
					NotTo(BeNil())

				Expect(
					modelStorageVolume.PersistentVolumeClaim.ClaimName,
				).To(Equal(resourceName))

				// -------------------------------------------------------------
				// Verify Service update
				// -------------------------------------------------------------

				service := &corev1.Service{}

				Eventually(func() error {
					return k8sClient.Get(
						ctx,
						typeNamespacedName,
						service,
					)
				}).Should(Succeed())

				Expect(service.Spec.Ports).To(HaveLen(1))

				Expect(service.Spec.Ports[0].Port).
					To(Equal(int32(8081)))

				Expect(service.Spec.Ports[0].TargetPort.String()).
					To(Equal(httpPortName))

				// -------------------------------------------------------------
				// Verify PVC remains correctly configured
				// -------------------------------------------------------------

				pvc := &corev1.PersistentVolumeClaim{}

				Eventually(func() error {
					return k8sClient.Get(
						ctx,
						typeNamespacedName,
						pvc,
					)
				}).Should(Succeed())

				requestedStorage :=
					pvc.Spec.Resources.Requests[corev1.ResourceStorage]

				Expect(requestedStorage.String()).
					To(Equal("1Gi"))

				Expect(pvc.Spec.AccessModes).
					To(ContainElement(corev1.ReadWriteOnce))

				// -------------------------------------------------------------
				// Verify PodDisruptionBudget update
				// -------------------------------------------------------------

				updatedPDB := &policyv1.PodDisruptionBudget{}

				Eventually(func() error {
					return k8sClient.Get(
						ctx,
						typeNamespacedName,
						updatedPDB,
					)
				}).Should(Succeed())

				Expect(updatedPDB.Spec.Selector).NotTo(BeNil())

				Expect(updatedPDB.Spec.Selector.MatchLabels).
					To(Equal(labelsForModelService(modelService)))

				Expect(updatedPDB.Spec.MinAvailable).
					To(BeNil())

				Expect(updatedPDB.Spec.MaxUnavailable).
					NotTo(BeNil())

				Expect(updatedPDB.Spec.MaxUnavailable.String()).
					To(Equal("25%"))

				Expect(metav1.IsControlledBy(updatedPDB, modelService)).
					To(BeTrue())

				// -------------------------------------------------------------
				// Verify NetworkPolicy update
				// -------------------------------------------------------------

				networkPolicy := &networkingv1.NetworkPolicy{}

				Eventually(func() error {
					return k8sClient.Get(
						ctx,
						typeNamespacedName,
						networkPolicy,
					)
				}).Should(Succeed())

				Expect(networkPolicy.Spec.PodSelector.MatchLabels).
					To(Equal(labelsForModelService(modelService)))

				Expect(networkPolicy.Spec.PolicyTypes).
					To(ConsistOf(
						networkingv1.PolicyTypeIngress,
						networkingv1.PolicyTypeEgress,
					))

				Expect(networkPolicy.Spec.Ingress).
					To(HaveLen(1))

				ingressRule := networkPolicy.Spec.Ingress[0]

				Expect(ingressRule.From).To(HaveLen(2))

				Expect(ingressRule.From[0].PodSelector).
					NotTo(BeNil())

				Expect(ingressRule.From[0].NamespaceSelector).
					To(BeNil())

				Expect(ingressRule.From[1].NamespaceSelector).
					NotTo(BeNil())

				Expect(
					ingressRule.From[1].
						NamespaceSelector.
						MatchLabels,
				).To(Equal(map[string]string{
					namespaceNameLabel: gatewayDataPlaneNamespace,
				}))

				Expect(ingressRule.Ports).To(HaveLen(1))

				Expect(ingressRule.Ports[0].Port).
					NotTo(BeNil())

				Expect(ingressRule.Ports[0].Port.IntVal).
					To(Equal(int32(8081)))

				Expect(ingressRule.Ports[0].Protocol).
					NotTo(BeNil())

				Expect(*ingressRule.Ports[0].Protocol).
					To(Equal(corev1.ProtocolTCP))

				Expect(networkPolicy.Spec.Egress).
					To(HaveLen(1))

				dnsRule := networkPolicy.Spec.Egress[0]

				Expect(dnsRule.To).To(HaveLen(1))

				Expect(dnsRule.To[0].NamespaceSelector).
					NotTo(BeNil())

				Expect(
					dnsRule.To[0].NamespaceSelector.MatchLabels,
				).To(Equal(map[string]string{
					namespaceNameLabel: "kube-system",
				}))

				Expect(dnsRule.To[0].PodSelector).
					NotTo(BeNil())

				Expect(
					dnsRule.To[0].PodSelector.MatchLabels,
				).To(Equal(map[string]string{
					"k8s-app": "kube-dns",
				}))

				Expect(dnsRule.Ports).To(HaveLen(2))

				Expect(metav1.IsControlledBy(
					networkPolicy,
					modelService,
				)).To(BeTrue())

				// -------------------------------------------------------------
				// Verify status reflects the newest generation
				// -------------------------------------------------------------

				updatedModelService :=
					&platformv1alpha1.ModelService{}

				Eventually(func() error {
					return k8sClient.Get(
						ctx,
						typeNamespacedName,
						updatedModelService,
					)
				}).Should(Succeed())

				Expect(
					updatedModelService.Status.ObservedGeneration,
				).To(Equal(updatedModelService.Generation))

				Expect(updatedModelService.Status.Endpoint).
					To(Equal(
						"http://test-resource.default.svc.cluster.local:8081",
					))

				Expect(updatedModelService.Status.Phase).
					To(Equal("Provisioning"))

				Expect(updatedModelService.Status.ReadyReplicas).
					To(Equal(int32(0)))
			},
		)
		It(
			"should delete the owned NetworkPolicy when disabled",
			func() {
				By("performing the initial reconciliation")

				controllerReconciler := &ModelServiceReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				_, err := controllerReconciler.Reconcile(
					ctx,
					reconcile.Request{
						NamespacedName: typeNamespacedName,
					},
				)
				Expect(err).NotTo(HaveOccurred())

				By("verifying that the NetworkPolicy exists")

				Eventually(func() error {
					return k8sClient.Get(
						ctx,
						typeNamespacedName,
						&networkingv1.NetworkPolicy{},
					)
				}).Should(Succeed())

				By("disabling NetworkPolicy in the ModelService specification")

				currentModelService :=
					&platformv1alpha1.ModelService{}

				Expect(
					k8sClient.Get(
						ctx,
						typeNamespacedName,
						currentModelService,
					),
				).To(Succeed())

				Expect(currentModelService.Spec.NetworkPolicy).
					NotTo(BeNil())

				currentModelService.Spec.NetworkPolicy.Enabled = false

				Expect(
					k8sClient.Update(
						ctx,
						currentModelService,
					),
				).To(Succeed())

				By("reconciling the updated ModelService")

				_, err = controllerReconciler.Reconcile(
					ctx,
					reconcile.Request{
						NamespacedName: typeNamespacedName,
					},
				)
				Expect(err).NotTo(HaveOccurred())

				By("verifying that the owned NetworkPolicy was deleted")

				Eventually(func() bool {
					err := k8sClient.Get(
						ctx,
						typeNamespacedName,
						&networkingv1.NetworkPolicy{},
					)

					return errors.IsNotFound(err)
				}).Should(BeTrue())
			},
		)

		It(
			"should delete the owned HTTPRoute when exposure is disabled",
			func() {
				By("performing the initial reconciliation")

				controllerReconciler := &ModelServiceReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				_, err := controllerReconciler.Reconcile(
					ctx,
					reconcile.Request{
						NamespacedName: typeNamespacedName,
					},
				)
				Expect(err).NotTo(HaveOccurred())

				By("verifying that the HTTPRoute exists")

				Eventually(func() error {
					return k8sClient.Get(
						ctx,
						typeNamespacedName,
						&gatewayv1.HTTPRoute{},
					)
				}).Should(Succeed())

				By("disabling exposure in the ModelService specification")

				currentModelService :=
					&platformv1alpha1.ModelService{}

				Expect(
					k8sClient.Get(
						ctx,
						typeNamespacedName,
						currentModelService,
					),
				).To(Succeed())

				Expect(currentModelService.Spec.Exposure).
					NotTo(BeNil())

				currentModelService.Spec.Exposure.Enabled = false

				Expect(
					k8sClient.Update(
						ctx,
						currentModelService,
					),
				).To(Succeed())

				By("reconciling the updated ModelService")

				_, err = controllerReconciler.Reconcile(
					ctx,
					reconcile.Request{
						NamespacedName: typeNamespacedName,
					},
				)
				Expect(err).NotTo(HaveOccurred())

				By("verifying that the owned HTTPRoute was deleted")

				Eventually(func() bool {
					err := k8sClient.Get(
						ctx,
						typeNamespacedName,
						&gatewayv1.HTTPRoute{},
					)

					return errors.IsNotFound(err)
				}).Should(BeTrue())
			},
		)

		It(
			"should assign controller ownership to all managed resources",
			func() {
				By("reconciling the ModelService")

				controllerReconciler := &ModelServiceReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				_, err := controllerReconciler.Reconcile(
					ctx,
					reconcile.Request{
						NamespacedName: typeNamespacedName,
					},
				)
				Expect(err).NotTo(HaveOccurred())

				By("verifying ServiceAccount ownership")

				serviceAccount := &corev1.ServiceAccount{}

				Expect(
					k8sClient.Get(
						ctx,
						typeNamespacedName,
						serviceAccount,
					),
				).To(Succeed())

				Expect(serviceAccount.OwnerReferences).To(HaveLen(1))

				Expect(serviceAccount.OwnerReferences[0].Name).
					To(Equal(resourceName))

				Expect(serviceAccount.OwnerReferences[0].Kind).
					To(Equal("ModelService"))

				Expect(serviceAccount.OwnerReferences[0].Controller).
					NotTo(BeNil())

				Expect(*serviceAccount.OwnerReferences[0].Controller).
					To(BeTrue())

				By("verifying NetworkPolicy ownership")

				modelService := &platformv1alpha1.ModelService{}

				Expect(
					k8sClient.Get(
						ctx,
						typeNamespacedName,
						modelService,
					),
				).To(Succeed())

				networkPolicy := &networkingv1.NetworkPolicy{}

				Expect(
					k8sClient.Get(
						ctx,
						typeNamespacedName,
						networkPolicy,
					),
				).To(Succeed())

				Expect(
					metav1.IsControlledBy(
						networkPolicy,
						modelService,
					),
				).To(BeTrue())

				By("verifying Deployment ownership")

				deployment := &appsv1.Deployment{}

				Expect(
					k8sClient.Get(
						ctx,
						typeNamespacedName,
						deployment,
					),
				).To(Succeed())

				Expect(deployment.OwnerReferences).To(HaveLen(1))
				Expect(deployment.OwnerReferences[0].Name).
					To(Equal(resourceName))
				Expect(deployment.OwnerReferences[0].Kind).
					To(Equal("ModelService"))
				Expect(deployment.OwnerReferences[0].Controller).
					NotTo(BeNil())
				Expect(*deployment.OwnerReferences[0].Controller).
					To(BeTrue())

				By("verifying Service ownership")

				service := &corev1.Service{}

				Expect(
					k8sClient.Get(
						ctx,
						typeNamespacedName,
						service,
					),
				).To(Succeed())

				Expect(service.OwnerReferences).To(HaveLen(1))
				Expect(service.OwnerReferences[0].Name).
					To(Equal(resourceName))
				Expect(service.OwnerReferences[0].Kind).
					To(Equal("ModelService"))
				Expect(service.OwnerReferences[0].Controller).
					NotTo(BeNil())
				Expect(*service.OwnerReferences[0].Controller).
					To(BeTrue())

				By("verifying PVC ownership")

				pvc := &corev1.PersistentVolumeClaim{}

				Expect(
					k8sClient.Get(
						ctx,
						typeNamespacedName,
						pvc,
					),
				).To(Succeed())

				Expect(pvc.OwnerReferences).To(HaveLen(1))
				Expect(pvc.OwnerReferences[0].Name).
					To(Equal(resourceName))
				Expect(pvc.OwnerReferences[0].Kind).
					To(Equal("ModelService"))
				Expect(pvc.OwnerReferences[0].Controller).
					NotTo(BeNil())
				Expect(*pvc.OwnerReferences[0].Controller).
					To(BeTrue())

				By("verifying PodDisruptionBudget ownership")

				pdb := &policyv1.PodDisruptionBudget{}

				Expect(
					k8sClient.Get(
						ctx,
						typeNamespacedName,
						pdb,
					),
				).To(Succeed())

				Expect(pdb.OwnerReferences).To(HaveLen(1))

				Expect(pdb.OwnerReferences[0].Name).
					To(Equal(resourceName))

				Expect(pdb.OwnerReferences[0].Kind).
					To(Equal("ModelService"))

				Expect(pdb.OwnerReferences[0].Controller).
					NotTo(BeNil())

				Expect(*pdb.OwnerReferences[0].Controller).
					To(BeTrue())
				By("verifying HTTPRoute controller ownership")

				httpRoute := &gatewayv1.HTTPRoute{}

				Expect(
					k8sClient.Get(
						ctx,
						typeNamespacedName,
						httpRoute,
					),
				).To(Succeed())

				Expect(
					metav1.IsControlledBy(
						httpRoute,
						modelService,
					),
				).To(BeTrue())
			},
		)
		It(
			"should restore ServiceAccount security configuration drift",
			func() {
				By("performing the initial reconciliation")

				controllerReconciler := &ModelServiceReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				_, err := controllerReconciler.Reconcile(
					ctx,
					reconcile.Request{
						NamespacedName: typeNamespacedName,
					},
				)
				Expect(err).NotTo(HaveOccurred())

				By("changing the managed ServiceAccount configuration")

				serviceAccount := &corev1.ServiceAccount{}

				Eventually(func() error {
					return k8sClient.Get(
						ctx,
						typeNamespacedName,
						serviceAccount,
					)
				}).Should(Succeed())

				serviceAccount.AutomountServiceAccountToken =
					boolPointer(true)

				Expect(
					k8sClient.Update(
						ctx,
						serviceAccount,
					),
				).To(Succeed())

				By("reconciling the ModelService again")

				_, err = controllerReconciler.Reconcile(
					ctx,
					reconcile.Request{
						NamespacedName: typeNamespacedName,
					},
				)
				Expect(err).NotTo(HaveOccurred())

				By("verifying that the secure configuration was restored")

				Eventually(func(g Gomega) {
					reconciledServiceAccount :=
						&corev1.ServiceAccount{}

					g.Expect(
						k8sClient.Get(
							ctx,
							typeNamespacedName,
							reconciledServiceAccount,
						),
					).To(Succeed())

					g.Expect(
						reconciledServiceAccount.
							AutomountServiceAccountToken,
					).NotTo(BeNil())

					g.Expect(
						*reconciledServiceAccount.
							AutomountServiceAccountToken,
					).To(BeFalse())
				}).Should(Succeed())
			},
		)
	})
})
