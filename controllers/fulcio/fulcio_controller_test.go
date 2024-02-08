/*
Copyright 2023.

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

package fulcio

import (
	"context"
	"fmt"
	"time"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/fulcio/actions"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Fulcio controller", func() {
	Context("Fulcio controller test", func() {

		const (
			Name      = "test-fulcio"
			Namespace = "default"
		)

		ctx := context.Background()

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      Name,
				Namespace: Namespace,
			},
		}

		typeNamespaceName := types.NamespacedName{Name: Name, Namespace: Namespace}
		instance := &v1alpha1.Fulcio{}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func() {
			By("removing the custom resource for the Kind Fulcio")
			found := &v1alpha1.Fulcio{}
			err := k8sClient.Get(ctx, typeNamespaceName, found)
			Expect(err).To(Not(HaveOccurred()))

			Eventually(func() error {
				return k8sClient.Delete(context.TODO(), found)
			}, 2*time.Minute, time.Second).Should(Succeed())

			// TODO(user): Attention if you improve this code by adding other context test you MUST
			// be aware of the current delete namespace limitations.
			// More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
			By("Deleting the Namespace to perform the tests")
			_ = k8sClient.Delete(ctx, namespace)
		})

		It("should successfully reconcile a custom resource for Fulcio", func() {
			By("creating the custom resource for the Kind Fulcio")
			err := k8sClient.Get(ctx, typeNamespaceName, instance)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				instance := &v1alpha1.Fulcio{
					ObjectMeta: metav1.ObjectMeta{
						Name:      Name,
						Namespace: Namespace,
					},
					Spec: v1alpha1.FulcioSpec{
						ExternalAccess: v1alpha1.ExternalAccess{
							Host:    "fulcio.localhost",
							Enabled: true,
						},
						Config: v1alpha1.FulcioConfig{
							OIDCIssuers: map[string]v1alpha1.OIDCIssuer{
								"test": {
									IssuerURL: "test",
									ClientID:  "test",
									Type:      "email",
								},
							},
						},
						Certificate: v1alpha1.FulcioCert{
							OrganizationName:  "MyOrg",
							OrganizationEmail: "my@email.com",
							CommonName:        "local",
							PrivateKeyPasswordRef: &v1alpha1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "password-secret",
								},
								Key: "password",
							},
						},
						Monitoring: v1alpha1.MonitoringConfig{Enabled: false},
					},
				}
				err = k8sClient.Create(ctx, instance)
				Expect(err).To(Not(HaveOccurred()))
			}

			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				found := &v1alpha1.Fulcio{}
				return k8sClient.Get(ctx, typeNamespaceName, found)
			}, time.Minute, time.Second).Should(Succeed())

			By("Pending phase until password key is resolved")
			Eventually(func() v1alpha1.Phase {
				found := &v1alpha1.Fulcio{}
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Phase
			}, time.Minute, time.Second).Should(Equal(v1alpha1.PhasePending))

			By("Creating password secret with cert password")
			Expect(k8sClient.Create(ctx, kubernetes.CreateSecret("password-secret", typeNamespaceName.Namespace, map[string][]byte{
				"password": []byte("secret"),
			}, constants.LabelsForComponent(actions.ComponentName, instance.Name)))).To(Succeed())

			By("Secrets are resolved")
			Eventually(func() error {
				found := &corev1.Secret{}
				return k8sClient.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("fulcio-%s-cert", Name), Namespace: Namespace}, found)
			}, time.Minute, time.Second).Should(Not(HaveOccurred()))

			Eventually(func() string {
				found := &v1alpha1.Fulcio{}
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Spec.Certificate.CARef.Name
			}, time.Minute, time.Second).Should(Equal(fmt.Sprintf("fulcio-%s-cert", Name)))
			Eventually(func() string {
				found := &v1alpha1.Fulcio{}
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Spec.Certificate.PrivateKeyRef.Name
			}, time.Minute, time.Second).Should(Equal(fmt.Sprintf("fulcio-%s-cert", Name)))
			Eventually(func() string {
				found := &v1alpha1.Fulcio{}
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Spec.Certificate.PrivateKeyPasswordRef.Name
			}, time.Minute, time.Second).Should(Equal("password-secret"))

			Eventually(func() map[string][]byte {
				found := &corev1.Secret{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: Namespace,
					Name:      fmt.Sprintf("fulcio-%s-cert", Name),
				}, found)).Should(Succeed())
				return found.Data
			}, time.Minute, time.Second).Should(And(HaveKey("private"), HaveKey("cert")))

			deployment := &appsv1.Deployment{}
			By("Checking if Deployment was successfully created in the reconciliation")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)
			}, time.Minute, time.Second).Should(Succeed())

			By("Move to Ready phase")
			// Workaround to succeed condition for Ready phase
			deployment.Status.Replicas = *deployment.Spec.Replicas
			deployment.Status.ReadyReplicas = *deployment.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, deployment)).Should(Succeed())

			By("Waiting until Fulcio instance is Ready")
			Eventually(func() v1alpha1.Phase {
				found := &v1alpha1.Fulcio{}
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Phase
			}, time.Minute, time.Second).Should(Equal(v1alpha1.PhaseReady))

			By("Checking if Service was successfully created in the reconciliation")
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, service)
			}, time.Minute, time.Second).Should(Succeed())
			Expect(service.Spec.Ports[0].Port).Should(Equal(int32(2112)))
			Expect(service.Spec.Ports[1].Port).Should(Equal(int32(5554)))
			Expect(service.Spec.Ports[2].Port).Should(Equal(int32(80)))

			By("Checking if Ingress was successfully created in the reconciliation")
			ingress := &v1.Ingress{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, ingress)
			}, time.Minute, time.Second).Should(Succeed())
			Expect(ingress.Spec.Rules[0].Host).Should(Equal("fulcio.localhost"))
			Expect(ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Name).Should(Equal(service.Name))
			Expect(ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Port.Name).Should(Equal("80-tcp"))

			By("Checking if controller will return deployment to desired state")
			deployment = &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)
			}, time.Minute, time.Second).Should(Succeed())
			replicas := int32(99)
			deployment.Spec.Replicas = &replicas
			Expect(k8sClient.Status().Update(ctx, deployment)).Should(Succeed())
			Eventually(func() int32 {
				deployment = &appsv1.Deployment{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)).Should(Succeed())
				return *deployment.Spec.Replicas
			}, time.Minute, time.Second).Should(Equal(int32(1)))
		})
	})
})
