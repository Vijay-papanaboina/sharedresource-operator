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

// Simple test: Create a source Secret, create a SharedResource, reconcile, verify target exists
var _ = Describe("SharedResource Simple Sync", func() {
	ctx := context.Background()

	It("should sync a Secret to target namespace", func() {
		// Create unique namespaces to avoid conflicts
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("source-ns-%d", suffix)
		targetNSName := fmt.Sprintf("target-ns-%d", suffix)

		sourceNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: sourceNSName},
		}
		Expect(k8sClient.Create(ctx, sourceNS)).To(Succeed())
		defer k8sClient.Delete(ctx, sourceNS)

		targetNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: targetNSName},
		}
		Expect(k8sClient.Create(ctx, targetNS)).To(Succeed())
		defer k8sClient.Delete(ctx, targetNS)

		// Create source Secret
		sourceSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-secret",
				Namespace: sourceNSName,
			},
			Data: map[string][]byte{"key": []byte("value")},
		}
		Expect(k8sClient.Create(ctx, sourceSecret)).To(Succeed())

		// Create SharedResource - Manager will automatically reconcile it
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sync-secret",
				Namespace: sourceNSName,
			},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source: platformv1alpha1.SourceSpec{
					Kind: "Secret",
					Name: "my-secret",
				},
				Targets: []platformv1alpha1.TargetSpec{
					{Namespace: targetNSName},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for target Secret to be created by the controller
		targetSecret := &corev1.Secret{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      "my-secret",
				Namespace: targetNSName,
			}, targetSecret)
		}, time.Second*10, time.Millisecond*250).Should(Succeed())

		// Verify data
		Expect(targetSecret.Data["key"]).To(Equal([]byte("value")))
	})
})
