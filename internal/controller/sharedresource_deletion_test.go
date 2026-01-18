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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	platformv1alpha1 "github.com/vijay-papanaboina/sharedresource-operator/api/v1alpha1"
)

var _ = Describe("Deletion Policy", func() {
	ctx := context.Background()

	It("should orphan targets when deletionPolicy is orphan (default)", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("orphan-src-%d", suffix)
		targetNSName := fmt.Sprintf("orphan-tgt-%d", suffix)

		// Create namespaces
		sourceNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: sourceNSName}}
		Expect(k8sClient.Create(ctx, sourceNS)).To(Succeed())
		defer k8sClient.Delete(ctx, sourceNS)

		targetNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNSName}}
		Expect(k8sClient.Create(ctx, targetNS)).To(Succeed())
		defer k8sClient.Delete(ctx, targetNS)

		// Create source
		source := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "orphan-secret", Namespace: sourceNSName},
			Data:       map[string][]byte{"key": []byte("value")},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())

		// Create SharedResource with default (orphan) policy
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-orphan", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source:  platformv1alpha1.SourceSpec{Kind: "Secret", Name: "orphan-secret"},
				Targets: []platformv1alpha1.TargetSpec{{Namespace: targetNSName}},
				// DeletionPolicy defaults to "orphan"
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for target to be created
		target := &corev1.Secret{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "orphan-secret", Namespace: targetNSName}, target)
		}, time.Second*10, time.Millisecond*250).Should(Succeed())

		// Delete the SharedResource
		Expect(k8sClient.Delete(ctx, sr)).To(Succeed())

		// Wait for SharedResource to be deleted
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "sync-orphan", Namespace: sourceNSName}, &platformv1alpha1.SharedResource{})
			return apierrors.IsNotFound(err)
		}, time.Second*10, time.Millisecond*250).Should(BeTrue())

		// Target should still exist (orphaned)
		Consistently(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "orphan-secret", Namespace: targetNSName}, &corev1.Secret{})
		}, time.Second*3, time.Millisecond*500).Should(Succeed())
	})

	It("should delete targets when deletionPolicy is delete", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("delete-src-%d", suffix)
		targetNSName := fmt.Sprintf("delete-tgt-%d", suffix)

		// Create namespaces
		sourceNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: sourceNSName}}
		Expect(k8sClient.Create(ctx, sourceNS)).To(Succeed())
		defer k8sClient.Delete(ctx, sourceNS)

		targetNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNSName}}
		Expect(k8sClient.Create(ctx, targetNS)).To(Succeed())
		defer k8sClient.Delete(ctx, targetNS)

		// Create source
		source := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "delete-secret", Namespace: sourceNSName},
			Data:       map[string][]byte{"key": []byte("value")},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())

		// Create SharedResource with delete policy
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-delete", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source:         platformv1alpha1.SourceSpec{Kind: "Secret", Name: "delete-secret"},
				Targets:        []platformv1alpha1.TargetSpec{{Namespace: targetNSName}},
				DeletionPolicy: platformv1alpha1.DeletionPolicyDelete,
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for target to be created
		target := &corev1.Secret{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "delete-secret", Namespace: targetNSName}, target)
		}, time.Second*10, time.Millisecond*250).Should(Succeed())

		// Delete the SharedResource
		Expect(k8sClient.Delete(ctx, sr)).To(Succeed())

		// Wait for target to be deleted
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "delete-secret", Namespace: targetNSName}, &corev1.Secret{})
			return apierrors.IsNotFound(err)
		}, time.Second*10, time.Millisecond*250).Should(BeTrue())
	})
})
