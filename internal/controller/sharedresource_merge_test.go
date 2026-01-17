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

var _ = Describe("Merge Mode", func() {
	ctx := context.Background()

	It("should preserve extra keys in target", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("merge-src-%d", suffix)
		targetNSName := fmt.Sprintf("merge-tgt-%d", suffix)

		// Create namespaces
		sourceNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: sourceNSName}}
		Expect(k8sClient.Create(ctx, sourceNS)).To(Succeed())
		defer k8sClient.Delete(ctx, sourceNS)

		targetNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNSName}}
		Expect(k8sClient.Create(ctx, targetNS)).To(Succeed())
		defer k8sClient.Delete(ctx, targetNS)

		// Create source
		source := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "merge-secret", Namespace: sourceNSName},
			Data:       map[string][]byte{"source-key": []byte("source-value")},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())

		// Create SharedResource with merge mode
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-merge", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source: platformv1alpha1.SourceSpec{Kind: "Secret", Name: "merge-secret"},
				SyncPolicy: &platformv1alpha1.SyncPolicySpec{
					Mode: "merge",
				},
				Targets: []platformv1alpha1.TargetSpec{{Namespace: targetNSName}},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for initial sync
		target := &corev1.Secret{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "merge-secret", Namespace: targetNSName}, target)
		}, time.Second*10, time.Millisecond*250).Should(Succeed())

		// Add local key - retry on conflict with fresh copy
		Eventually(func() error {
			freshTarget := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "merge-secret", Namespace: targetNSName}, freshTarget); err != nil {
				return err
			}
			freshTarget.Data["local-key"] = []byte("local-value")
			return k8sClient.Update(ctx, freshTarget)
		}, time.Second*5, time.Millisecond*500).Should(Succeed())

		// Wait for watch to trigger reconcile, then verify both keys exist
		Eventually(func() bool {
			freshTarget := &corev1.Secret{}
			k8sClient.Get(ctx, types.NamespacedName{Name: "merge-secret", Namespace: targetNSName}, freshTarget)
			hasSource := string(freshTarget.Data["source-key"]) == "source-value"
			hasLocal := string(freshTarget.Data["local-key"]) == "local-value"
			return hasSource && hasLocal
		}, time.Second*10, time.Millisecond*250).Should(BeTrue())
	})
})
