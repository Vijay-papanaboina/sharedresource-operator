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

var _ = Describe("Source Updates", func() {
	ctx := context.Background()

	It("should propagate source data changes to target", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("update-src-%d", suffix)
		targetNSName := fmt.Sprintf("update-tgt-%d", suffix)

		// Create namespaces
		sourceNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: sourceNSName}}
		Expect(k8sClient.Create(ctx, sourceNS)).To(Succeed())
		defer k8sClient.Delete(ctx, sourceNS)

		targetNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNSName}}
		Expect(k8sClient.Create(ctx, targetNS)).To(Succeed())
		defer k8sClient.Delete(ctx, targetNS)

		// Create source
		source := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "update-secret", Namespace: sourceNSName},
			Data:       map[string][]byte{"key": []byte("original")},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())

		// Create SharedResource
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-update", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source:  platformv1alpha1.SourceSpec{Kind: "Secret", Name: "update-secret"},
				Targets: []platformv1alpha1.TargetSpec{{Namespace: targetNSName}},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for initial sync
		target := &corev1.Secret{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "update-secret", Namespace: targetNSName}, target)
		}, time.Second*10, time.Millisecond*250).Should(Succeed())
		Expect(target.Data["key"]).To(Equal([]byte("original")))

		// Update source
		Eventually(func() error {
			freshSource := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "update-secret", Namespace: sourceNSName}, freshSource); err != nil {
				return err
			}
			freshSource.Data["key"] = []byte("updated")
			return k8sClient.Update(ctx, freshSource)
		}, time.Second*5, time.Millisecond*500).Should(Succeed())

		// Wait for target to be updated
		Eventually(func() string {
			freshTarget := &corev1.Secret{}
			k8sClient.Get(ctx, types.NamespacedName{Name: "update-secret", Namespace: targetNSName}, freshTarget)
			return string(freshTarget.Data["key"])
		}, time.Second*10, time.Millisecond*250).Should(Equal("updated"))
	})

	It("should add new keys from source to target", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("addkey-src-%d", suffix)
		targetNSName := fmt.Sprintf("addkey-tgt-%d", suffix)

		// Create namespaces
		sourceNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: sourceNSName}}
		Expect(k8sClient.Create(ctx, sourceNS)).To(Succeed())
		defer k8sClient.Delete(ctx, sourceNS)

		targetNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNSName}}
		Expect(k8sClient.Create(ctx, targetNS)).To(Succeed())
		defer k8sClient.Delete(ctx, targetNS)

		// Create source with one key
		source := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "addkey-secret", Namespace: sourceNSName},
			Data:       map[string][]byte{"existing": []byte("value")},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())

		// Create SharedResource
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-addkey", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source:  platformv1alpha1.SourceSpec{Kind: "Secret", Name: "addkey-secret"},
				Targets: []platformv1alpha1.TargetSpec{{Namespace: targetNSName}},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for initial sync
		Eventually(func() error {
			target := &corev1.Secret{}
			return k8sClient.Get(ctx, types.NamespacedName{Name: "addkey-secret", Namespace: targetNSName}, target)
		}, time.Second*10, time.Millisecond*250).Should(Succeed())

		// Add new key to source
		Eventually(func() error {
			freshSource := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "addkey-secret", Namespace: sourceNSName}, freshSource); err != nil {
				return err
			}
			freshSource.Data["newkey"] = []byte("newvalue")
			return k8sClient.Update(ctx, freshSource)
		}, time.Second*5, time.Millisecond*500).Should(Succeed())

		// Wait for new key to appear in target
		Eventually(func() bool {
			freshTarget := &corev1.Secret{}
			k8sClient.Get(ctx, types.NamespacedName{Name: "addkey-secret", Namespace: targetNSName}, freshTarget)
			_, exists := freshTarget.Data["newkey"]
			return exists
		}, time.Second*10, time.Millisecond*250).Should(BeTrue())
	})

	It("should remove keys from target when removed from source (copy mode)", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("rmkey-src-%d", suffix)
		targetNSName := fmt.Sprintf("rmkey-tgt-%d", suffix)

		// Create namespaces
		sourceNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: sourceNSName}}
		Expect(k8sClient.Create(ctx, sourceNS)).To(Succeed())
		defer k8sClient.Delete(ctx, sourceNS)

		targetNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNSName}}
		Expect(k8sClient.Create(ctx, targetNS)).To(Succeed())
		defer k8sClient.Delete(ctx, targetNS)

		// Create source with two keys
		source := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "rmkey-secret", Namespace: sourceNSName},
			Data: map[string][]byte{
				"keep":   []byte("value"),
				"remove": []byte("will-be-removed"),
			},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())

		// Create SharedResource (copy mode is default)
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-rmkey", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source:  platformv1alpha1.SourceSpec{Kind: "Secret", Name: "rmkey-secret"},
				Targets: []platformv1alpha1.TargetSpec{{Namespace: targetNSName}},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for initial sync with both keys
		Eventually(func() bool {
			target := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "rmkey-secret", Namespace: targetNSName}, target); err != nil {
				return false
			}
			_, hasRemove := target.Data["remove"]
			return hasRemove
		}, time.Second*10, time.Millisecond*250).Should(BeTrue())

		// Remove key from source
		Eventually(func() error {
			freshSource := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "rmkey-secret", Namespace: sourceNSName}, freshSource); err != nil {
				return err
			}
			delete(freshSource.Data, "remove")
			return k8sClient.Update(ctx, freshSource)
		}, time.Second*5, time.Millisecond*500).Should(Succeed())

		// Wait for key to be removed from target
		Eventually(func() bool {
			freshTarget := &corev1.Secret{}
			k8sClient.Get(ctx, types.NamespacedName{Name: "rmkey-secret", Namespace: targetNSName}, freshTarget)
			_, exists := freshTarget.Data["remove"]
			return exists
		}, time.Second*10, time.Millisecond*250).Should(BeFalse())
	})

	It("should sync to new target when added to CR", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("addtgt-src-%d", suffix)
		target1NSName := fmt.Sprintf("addtgt-tgt1-%d", suffix)
		target2NSName := fmt.Sprintf("addtgt-tgt2-%d", suffix)

		// Create namespaces
		for _, ns := range []string{sourceNSName, target1NSName, target2NSName} {
			Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
			defer k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
		}

		// Create source
		source := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "addtgt-secret", Namespace: sourceNSName},
			Data:       map[string][]byte{"key": []byte("value")},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())

		// Create SharedResource with one target
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-addtgt", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source:  platformv1alpha1.SourceSpec{Kind: "Secret", Name: "addtgt-secret"},
				Targets: []platformv1alpha1.TargetSpec{{Namespace: target1NSName}},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for first target
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "addtgt-secret", Namespace: target1NSName}, &corev1.Secret{})
		}, time.Second*10, time.Millisecond*250).Should(Succeed())

		// Update CR to add second target
		Eventually(func() error {
			freshSR := &platformv1alpha1.SharedResource{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "sync-addtgt", Namespace: sourceNSName}, freshSR); err != nil {
				return err
			}
			freshSR.Spec.Targets = append(freshSR.Spec.Targets, platformv1alpha1.TargetSpec{Namespace: target2NSName})
			return k8sClient.Update(ctx, freshSR)
		}, time.Second*5, time.Millisecond*500).Should(Succeed())

		// Wait for second target
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "addtgt-secret", Namespace: target2NSName}, &corev1.Secret{})
		}, time.Second*10, time.Millisecond*250).Should(Succeed())
	})

	It("should update existing target when renamed in CR", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("crrename-src-%d", suffix)
		targetNSName := fmt.Sprintf("crrename-tgt-%d", suffix)

		// Create namespaces
		for _, ns := range []string{sourceNSName, targetNSName} {
			Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
			defer k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
		}

		// Create source
		source := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "crrename-secret", Namespace: sourceNSName},
			Data:       map[string][]byte{"key": []byte("value")},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())

		// Create SharedResource with original name
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-crrename", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source:  platformv1alpha1.SourceSpec{Kind: "Secret", Name: "crrename-secret"},
				Targets: []platformv1alpha1.TargetSpec{{Namespace: targetNSName, Name: "old-name"}},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for old name
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "old-name", Namespace: targetNSName}, &corev1.Secret{})
		}, time.Second*10, time.Millisecond*250).Should(Succeed())

		// Update CR to rename target
		Eventually(func() error {
			freshSR := &platformv1alpha1.SharedResource{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "sync-crrename", Namespace: sourceNSName}, freshSR); err != nil {
				return err
			}
			freshSR.Spec.Targets[0].Name = "new-name"
			return k8sClient.Update(ctx, freshSR)
		}, time.Second*5, time.Millisecond*500).Should(Succeed())

		// Wait for new name
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "new-name", Namespace: targetNSName}, &corev1.Secret{})
		}, time.Second*10, time.Millisecond*250).Should(Succeed())
	})

	It("should change sync mode from copy to merge", func() {
		suffix := time.Now().UnixNano() % 100000
		sourceNSName := fmt.Sprintf("mode-src-%d", suffix)
		targetNSName := fmt.Sprintf("mode-tgt-%d", suffix)

		// Create namespaces
		for _, ns := range []string{sourceNSName, targetNSName} {
			Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
			defer k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
		}

		// Create source
		source := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "mode-secret", Namespace: sourceNSName},
			Data:       map[string][]byte{"source": []byte("val")},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())

		// Create SharedResource in copy mode
		sr := &platformv1alpha1.SharedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "sync-mode", Namespace: sourceNSName},
			Spec: platformv1alpha1.SharedResourceSpec{
				Source: platformv1alpha1.SourceSpec{Kind: "Secret", Name: "mode-secret"},
				SyncPolicy: &platformv1alpha1.SyncPolicySpec{
					Mode: platformv1alpha1.SyncModeCopy,
				},
				Targets: []platformv1alpha1.TargetSpec{{Namespace: targetNSName}},
			},
		}
		Expect(k8sClient.Create(ctx, sr)).To(Succeed())

		// Wait for sync
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "mode-secret", Namespace: targetNSName}, &corev1.Secret{})
		}, time.Second*10, time.Millisecond*250).Should(Succeed())

		// Manually add local key (should be wiped in copy mode on next sync/drift check)
		Eventually(func() error {
			target := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "mode-secret", Namespace: targetNSName}, target); err != nil {
				return err
			}
			target.Data["local"] = []byte("val")
			return k8sClient.Update(ctx, target)
		}, time.Second*5, time.Millisecond*500).Should(Succeed())

		// Switch to merge mode
		Eventually(func() error {
			freshSR := &platformv1alpha1.SharedResource{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "sync-mode", Namespace: sourceNSName}, freshSR); err != nil {
				return err
			}
			freshSR.Spec.SyncPolicy.Mode = platformv1alpha1.SyncModeMerge
			return k8sClient.Update(ctx, freshSR)
		}, time.Second*5, time.Millisecond*500).Should(Succeed())

		// Add local key again (it might have been wiped before switch, or we just add it now)
		Eventually(func() error {
			target := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "mode-secret", Namespace: targetNSName}, target); err != nil {
				return err
			}
			target.Data["local"] = []byte("val")
			return k8sClient.Update(ctx, target)
		}, time.Second*5, time.Millisecond*500).Should(Succeed())

		// Verify local key persists in merge mode
		Consistently(func() bool {
			target := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "mode-secret", Namespace: targetNSName}, target); err != nil {
				return false
			}
			_, hasLocal := target.Data["local"]
			return hasLocal
		}, time.Second*3, time.Millisecond*500).Should(BeTrue())
	})
})
