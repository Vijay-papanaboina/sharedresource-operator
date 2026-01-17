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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	platformv1alpha1 "github.com/vijay-papanaboina/sharedresource-operator/api/v1alpha1"
)

var _ = Describe("SharedResource Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		sharedresource := &platformv1alpha1.SharedResource{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind SharedResource")
			// Create source secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"key": []byte("value"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			err := k8sClient.Get(ctx, typeNamespacedName, sharedresource)
			if err != nil && errors.IsNotFound(err) {
				resource := &platformv1alpha1.SharedResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: platformv1alpha1.SharedResourceSpec{
						Source: platformv1alpha1.SourceSpec{
							Kind: "Secret",
							Name: "test-source-secret",
						},
						Targets: []platformv1alpha1.TargetSpec{
							{Namespace: "default", Name: "target-secret"},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &platformv1alpha1.SharedResource{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance SharedResource")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			By("Cleanup the source secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source-secret",
					Namespace: "default",
				},
			}
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")

			// The controller is running in the background (started in suite_test.go),
			// so we just need to wait for the reconciliation to happen.

			targetSecret := &corev1.Secret{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "target-secret",
					Namespace: "default",
				}, targetSecret)
			}, time.Second*10, time.Millisecond*250).Should(Succeed())

			Expect(targetSecret.Data["key"]).To(Equal([]byte("value")))
		})
	})
})
