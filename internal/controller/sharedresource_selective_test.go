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

var _ = Describe("Selective Key Filtering", func() {
	ctx := context.Background()

	It("should sync only included keys", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("include-src-%d", suffix)
		targetNSName := fmt.Sprintf("include-tgt-%d", suffix)

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

		// Create source with multiple keys
		source := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "multi-key-secret", Namespace: sourceNSName},
			Data: map[string][]byte{
				"key-a": []byte("value-a"),
				"key-b": []byte("value-b"),
				"key-c": []byte("value-c"),
			},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())

		// Create SharedResource with include filter
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-include", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source: platformv1alpha1.SourceSpec{Kind: "Secret", Name: "multi-key-secret"},
				SyncPolicy: &platformv1alpha1.SyncPolicySpec{
					Mode: platformv1alpha1.SyncModeSelective,
					Keys: &platformv1alpha1.KeySelector{
						Include: []string{"key-a", "key-b"},
					},
				},
				Targets: []platformv1alpha1.TargetSpec{{Namespace: targetNSName}},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for target
		target := &corev1.Secret{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "multi-key-secret", Namespace: targetNSName}, target)
		}, time.Second*10, time.Millisecond*250).Should(Succeed())

		// Verify only included keys exist
		Expect(target.Data).To(HaveKey("key-a"))
		Expect(target.Data).To(HaveKey("key-b"))
		Expect(target.Data).NotTo(HaveKey("key-c"))
	})

	It("should exclude specified keys", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("exclude-src-%d", suffix)
		targetNSName := fmt.Sprintf("exclude-tgt-%d", suffix)

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

		// Create source
		source := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "exclude-secret", Namespace: sourceNSName},
			Data: map[string][]byte{
				"keep-a":    []byte("value-a"),
				"keep-b":    []byte("value-b"),
				"skip-this": []byte("should-not-sync"),
			},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())

		// Create SharedResource with exclude filter
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-exclude", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source: platformv1alpha1.SourceSpec{Kind: "Secret", Name: "exclude-secret"},
				SyncPolicy: &platformv1alpha1.SyncPolicySpec{
					Mode: platformv1alpha1.SyncModeSelective,
					Keys: &platformv1alpha1.KeySelector{
						Exclude: []string{"skip-this"},
					},
				},
				Targets: []platformv1alpha1.TargetSpec{{Namespace: targetNSName}},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for target
		target := &corev1.Secret{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "exclude-secret", Namespace: targetNSName}, target)
		}, time.Second*10, time.Millisecond*250).Should(Succeed())

		// Verify excluded key is missing
		Expect(target.Data).To(HaveKey("keep-a"))
		Expect(target.Data).To(HaveKey("keep-b"))
		Expect(target.Data).NotTo(HaveKey("skip-this"))
	})

	It("should apply include then exclude filters", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("combo-src-%d", suffix)
		targetNSName := fmt.Sprintf("combo-tgt-%d", suffix)

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

		// Create source
		source := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "combo-secret", Namespace: sourceNSName},
			Data: map[string][]byte{
				"key-a": []byte("value-a"),
				"key-b": []byte("value-b"),
				"key-c": []byte("value-c"),
				"key-d": []byte("value-d"),
			},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())

		// Include a,b,c but exclude c → should have only a,b
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-combo", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source: platformv1alpha1.SourceSpec{Kind: "Secret", Name: "combo-secret"},
				SyncPolicy: &platformv1alpha1.SyncPolicySpec{
					Mode: platformv1alpha1.SyncModeSelective,
					Keys: &platformv1alpha1.KeySelector{
						Include: []string{"key-a", "key-b", "key-c"},
						Exclude: []string{"key-c"},
					},
				},
				Targets: []platformv1alpha1.TargetSpec{{Namespace: targetNSName}},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for target
		target := &corev1.Secret{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "combo-secret", Namespace: targetNSName}, target)
		}, time.Second*10, time.Millisecond*250).Should(Succeed())

		// Verify combo filter result
		Expect(target.Data).To(HaveKey("key-a"))
		Expect(target.Data).To(HaveKey("key-b"))
		Expect(target.Data).NotTo(HaveKey("key-c")) // excluded
		Expect(target.Data).NotTo(HaveKey("key-d")) // not included
	})
})
