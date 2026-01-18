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

var _ = Describe("ConfigMap Sync", func() {
	ctx := context.Background()

	It("should sync a ConfigMap to target namespace", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("cm-src-%d", suffix)
		targetNSName := fmt.Sprintf("cm-tgt-%d", suffix)

		// Create namespaces
		sourceNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: sourceNSName}}
		Expect(k8sClient.Create(ctx, sourceNS)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, sourceNS) }()

		targetNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNSName}}
		Expect(k8sClient.Create(ctx, targetNS)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, targetNS) }()

		// Create source ConfigMap
		source := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "my-config", Namespace: sourceNSName},
			Data: map[string]string{
				"config.yaml": "key: value",
				"settings":    "debug=true",
			},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())

		// Create SharedResource for ConfigMap
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-configmap", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source:  platformv1alpha1.SourceSpec{Kind: "ConfigMap", Name: "my-config"},
				Targets: []platformv1alpha1.TargetSpec{{Namespace: targetNSName}},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for target ConfigMap
		target := &corev1.ConfigMap{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "my-config", Namespace: targetNSName}, target)
		}, time.Second*10, time.Millisecond*250).Should(Succeed())

		// Verify data
		Expect(target.Data["config.yaml"]).To(Equal("key: value"))
		Expect(target.Data["settings"]).To(Equal("debug=true"))
	})
})

var _ = Describe("Multi-Target Sync", func() {
	ctx := context.Background()

	It("should sync to multiple target namespaces", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("multi-src-%d", suffix)
		target1NSName := fmt.Sprintf("multi-tgt1-%d", suffix)
		target2NSName := fmt.Sprintf("multi-tgt2-%d", suffix)
		target3NSName := fmt.Sprintf("multi-tgt3-%d", suffix)

		// Create all namespaces
		sourceNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: sourceNSName}}
		Expect(k8sClient.Create(ctx, sourceNS)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, sourceNS) }()

		target1NS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: target1NSName}}
		Expect(k8sClient.Create(ctx, target1NS)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, target1NS) }()

		target2NS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: target2NSName}}
		Expect(k8sClient.Create(ctx, target2NS)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, target2NS) }()

		target3NS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: target3NSName}}
		Expect(k8sClient.Create(ctx, target3NS)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, target3NS) }()

		// Create source
		source := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "shared-creds", Namespace: sourceNSName},
			Data:       map[string][]byte{"password": []byte("secret123")},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())

		// Create SharedResource with multiple targets
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-multi", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source: platformv1alpha1.SourceSpec{Kind: "Secret", Name: "shared-creds"},
				Targets: []platformv1alpha1.TargetSpec{
					{Namespace: target1NSName},
					{Namespace: target2NSName},
					{Namespace: target3NSName},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for all targets
		for _, ns := range []string{target1NSName, target2NSName, target3NSName} {
			target := &corev1.Secret{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "shared-creds", Namespace: ns}, target)
			}, time.Second*10, time.Millisecond*250).Should(Succeed())
			Expect(target.Data["password"]).To(Equal([]byte("secret123")))
		}
	})
})
