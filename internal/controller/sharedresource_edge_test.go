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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	platformv1alpha1 "github.com/vijay-papanaboina/sharedresource-operator/api/v1alpha1"
)

var _ = Describe("Edge Cases", func() {
	ctx := context.Background()

	It("should preserve TLS secret type", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("tls-src-%d", suffix)
		targetNSName := fmt.Sprintf("tls-tgt-%d", suffix)

		// Create namespaces
		sourceNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: sourceNSName}}
		Expect(k8sClient.Create(ctx, sourceNS)).To(Succeed())
		defer func(name string) {
			_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})
		}(sourceNSName)

		targetNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNSName}}
		Expect(k8sClient.Create(ctx, targetNS)).To(Succeed())
		defer func(name string) {
			_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})
		}(targetNSName)

		// Create TLS secret
		source := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "tls-cert", Namespace: sourceNSName},
			Type:       corev1.SecretTypeTLS,
			Data: map[string][]byte{
				"tls.crt": []byte("-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----"),
				"tls.key": []byte("-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----"),
			},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())

		// Create SharedResource
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-tls", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source:  platformv1alpha1.SourceSpec{Kind: "Secret", Name: "tls-cert"},
				Targets: []platformv1alpha1.TargetSpec{{Namespace: targetNSName}},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for target
		target := &corev1.Secret{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "tls-cert", Namespace: targetNSName}, target)
		}, time.Second*10, time.Millisecond*250).Should(Succeed())

		// Verify type is preserved
		Expect(target.Type).To(Equal(corev1.SecretTypeTLS))
	})

	It("should set SourceNotFound condition when source doesn't exist", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("nosrc-ns-%d", suffix)
		targetNSName := fmt.Sprintf("nosrc-tgt-%d", suffix)

		// Create namespaces
		sourceNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: sourceNSName}}
		Expect(k8sClient.Create(ctx, sourceNS)).To(Succeed())
		defer func(name string) {
			_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})
		}(sourceNSName)

		targetNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNSName}}
		Expect(k8sClient.Create(ctx, targetNS)).To(Succeed())
		defer func(name string) {
			_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})
		}(targetNSName)

		// Create SharedResource WITHOUT creating source first
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-nosource", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source:  platformv1alpha1.SourceSpec{Kind: "Secret", Name: "nonexistent-secret"},
				Targets: []platformv1alpha1.TargetSpec{{Namespace: targetNSName}},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for SourceFound condition to be False
		Eventually(func() bool {
			freshSR := &platformv1alpha1.SharedResource{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "sync-nosource", Namespace: sourceNSName}, freshSR); err != nil {
				return false
			}
			for _, c := range freshSR.Status.Conditions {
				if c.Type == "SourceFound" && c.Status == metav1.ConditionFalse {
					return true
				}
			}
			return false
		}, time.Second*10, time.Millisecond*250).Should(BeTrue())
	})

})
