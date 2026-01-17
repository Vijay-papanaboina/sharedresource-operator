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

var _ = Describe("Drift Correction", func() {
	ctx := context.Background()

	It("should correct tampered target Secret", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("drift-src-%d", suffix)
		targetNSName := fmt.Sprintf("drift-tgt-%d", suffix)

		// Create namespaces
		sourceNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: sourceNSName}}
		Expect(k8sClient.Create(ctx, sourceNS)).To(Succeed())
		defer k8sClient.Delete(ctx, sourceNS)

		targetNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNSName}}
		Expect(k8sClient.Create(ctx, targetNS)).To(Succeed())
		defer k8sClient.Delete(ctx, targetNS)

		// Create source
		source := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "drift-secret", Namespace: sourceNSName},
			Data:       map[string][]byte{"original": []byte("correct")},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())

		// Create SharedResource
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-drift", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source:  platformv1alpha1.SourceSpec{Kind: "Secret", Name: "drift-secret"},
				Targets: []platformv1alpha1.TargetSpec{{Namespace: targetNSName}},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for target to be created
		target := &corev1.Secret{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "drift-secret", Namespace: targetNSName}, target)
		}, time.Second*10, time.Millisecond*250).Should(Succeed())

		// Verify initial sync
		Expect(target.Data["original"]).To(Equal([]byte("correct")))

		// Tamper with target - retry on conflict by getting fresh copy
		Eventually(func() error {
			freshTarget := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "drift-secret", Namespace: targetNSName}, freshTarget); err != nil {
				return err
			}
			freshTarget.Data["original"] = []byte("tampered")
			return k8sClient.Update(ctx, freshTarget)
		}, time.Second*5, time.Millisecond*500).Should(Succeed())

		// Wait for controller to detect and fix drift
		Eventually(func() string {
			freshTarget := &corev1.Secret{}
			k8sClient.Get(ctx, types.NamespacedName{Name: "drift-secret", Namespace: targetNSName}, freshTarget)
			return string(freshTarget.Data["original"])
		}, time.Second*10, time.Millisecond*250).Should(Equal("correct"))
	})
})
