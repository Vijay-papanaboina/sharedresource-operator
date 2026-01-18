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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/vijay-papanaboina/sharedresource-operator/api/v1alpha1"
)

// =============================================================================
// Sync operations for Secret and ConfigMap resources.
//
// These methods handle the actual synchronization of resources to target
// namespaces, including creation, updates, and deletion.
// =============================================================================

// fetchSourceResource retrieves the source Secret or ConfigMap.
//
// Returns:
// - data: The key-value data from the source resource
// - secretType: The secret type (only for Secrets, e.g., kubernetes.io/tls)
// - error: Any error encountered
//
// Note: Source must be in the SAME namespace as the SharedResource CR.
func (r *SharedResourceReconciler) fetchSourceResource(ctx context.Context, sr *platformv1alpha1.SharedResource) (map[string][]byte, corev1.SecretType, error) {
	sourceKey := types.NamespacedName{
		Namespace: sr.Namespace, // Source is in same namespace as CR
		Name:      sr.Spec.Source.Name,
	}

	switch sr.Spec.Source.Kind {
	case KindSecret:
		var secret corev1.Secret
		if err := r.Get(ctx, sourceKey, &secret); err != nil {
			return nil, "", err
		}
		return secret.Data, secret.Type, nil

	case KindConfigMap:
		var cm corev1.ConfigMap
		if err := r.Get(ctx, sourceKey, &cm); err != nil {
			return nil, "", err
		}
		// Convert string data to []byte for uniform handling
		data := make(map[string][]byte)
		for k, v := range cm.Data {
			data[k] = []byte(v)
		}
		return data, "", nil

	default:
		return nil, "", fmt.Errorf("unsupported source kind: %s", sr.Spec.Source.Kind)
	}
}

// syncToTarget creates or updates the target resource in the specified namespace.
//
// This is the main entry point for syncing a single target. It:
// 1. Builds the required annotations for tracking
// 2. Delegates to syncSecret or syncConfigMap based on source kind
// 3. Uses syncPolicy.mode to determine sync behavior (copy vs merge)
func (r *SharedResourceReconciler) syncToTarget(
	ctx context.Context,
	sr *platformv1alpha1.SharedResource,
	targetNamespace string,
	targetName string,
	data map[string][]byte,
	secretType corev1.SecretType,
	checksum string,
) error {
	log := logf.FromContext(ctx)

	// Determine sync mode (default to "copy" for strict behavior)
	syncMode := "copy"
	if sr.Spec.SyncPolicy != nil && sr.Spec.SyncPolicy.Mode != "" {
		syncMode = string(sr.Spec.SyncPolicy.Mode)
	}

	// Build annotations for tracking and drift detection
	annotations := map[string]string{
		AnnotationManagedBy:       ManagedByValue,
		AnnotationSourceNamespace: sr.Namespace,
		AnnotationSourceName:      sr.Spec.Source.Name,
		AnnotationSourceCR:        sr.Name,
		AnnotationChecksum:        checksum,
		AnnotationLastSynced:      time.Now().UTC().Format(time.RFC3339),
	}

	targetKey := types.NamespacedName{Namespace: targetNamespace, Name: targetName}

	switch sr.Spec.Source.Kind {
	case KindSecret:
		return r.syncSecret(ctx, targetKey, data, secretType, annotations, syncMode, log)
	case KindConfigMap:
		return r.syncConfigMap(ctx, targetKey, data, annotations, syncMode, log)
	default:
		return fmt.Errorf("unsupported source kind: %s", sr.Spec.Source.Kind)
	}
}

// syncSecret creates or updates a Secret in the target namespace.
//
// Behavior depends on syncMode:
// - "copy": Target data = Source data exactly (overwrites everything)
// - "merge": Source keys are synced, extra target keys are preserved
func (r *SharedResourceReconciler) syncSecret(
	ctx context.Context,
	targetKey types.NamespacedName,
	data map[string][]byte,
	secretType corev1.SecretType,
	annotations map[string]string,
	syncMode string,
	log logr.Logger,
) error {
	var existing corev1.Secret
	err := r.Get(ctx, targetKey, &existing)

	if apierrors.IsNotFound(err) {
		// Create new Secret
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        targetKey.Name,
				Namespace:   targetKey.Namespace,
				Annotations: annotations,
			},
			Type: secretType,
			Data: data,
		}
		log.Info("Creating target Secret", "namespace", targetKey.Namespace, "name", targetKey.Name)
		return r.Create(ctx, secret)
	} else if err != nil {
		return err
	}

	// Secret exists - determine what data to use based on sync mode
	var targetData map[string][]byte
	if syncMode == "merge" {
		// Merge mode: Start with existing data, overlay source data
		targetData = make(map[string][]byte)
		for k, v := range existing.Data {
			targetData[k] = v
		}
		for k, v := range data {
			targetData[k] = v
		}
	} else {
		// Copy mode (default): Target = Source exactly
		targetData = data
	}

	// Check if update is needed by comparing actual data
	existingDataChecksum := computeChecksum(existing.Data)
	newDataChecksum := computeChecksum(targetData)
	if existingDataChecksum == newDataChecksum {
		log.Info("Target Secret already up to date", "namespace", targetKey.Namespace, "name", targetKey.Name, "mode", syncMode)
		return nil
	}

	// Update existing Secret
	existing.Data = targetData
	existing.Type = secretType
	if existing.Annotations == nil {
		existing.Annotations = make(map[string]string)
	}
	for k, v := range annotations {
		existing.Annotations[k] = v
	}

	log.Info("Updating target Secret", "namespace", targetKey.Namespace, "name", targetKey.Name, "mode", syncMode)
	return r.Update(ctx, &existing)
}

// syncConfigMap creates or updates a ConfigMap in the target namespace.
//
// Behavior depends on syncMode:
// - "copy": Target data = Source data exactly (overwrites everything)
// - "merge": Source keys are synced, extra target keys are preserved
func (r *SharedResourceReconciler) syncConfigMap(
	ctx context.Context,
	targetKey types.NamespacedName,
	data map[string][]byte,
	annotations map[string]string,
	syncMode string,
	log logr.Logger,
) error {
	// Convert []byte back to string for ConfigMap
	stringData := make(map[string]string)
	for k, v := range data {
		stringData[k] = string(v)
	}

	var existing corev1.ConfigMap
	err := r.Get(ctx, targetKey, &existing)

	if apierrors.IsNotFound(err) {
		// Create new ConfigMap
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:        targetKey.Name,
				Namespace:   targetKey.Namespace,
				Annotations: annotations,
			},
			Data: stringData,
		}
		log.Info("Creating target ConfigMap", "namespace", targetKey.Namespace, "name", targetKey.Name)
		return r.Create(ctx, cm)
	} else if err != nil {
		return err
	}

	// ConfigMap exists - determine what data to use based on sync mode
	var targetData map[string]string
	if syncMode == "merge" {
		// Merge mode: Start with existing data, overlay source data
		targetData = make(map[string]string)
		for k, v := range existing.Data {
			targetData[k] = v
		}
		for k, v := range stringData {
			targetData[k] = v
		}
	} else {
		// Copy mode (default): Target = Source exactly
		targetData = stringData
	}

	// Check if update is needed by comparing actual data
	existingByteData := make(map[string][]byte)
	for k, v := range existing.Data {
		existingByteData[k] = []byte(v)
	}
	targetByteData := make(map[string][]byte)
	for k, v := range targetData {
		targetByteData[k] = []byte(v)
	}
	existingDataChecksum := computeChecksum(existingByteData)
	newDataChecksum := computeChecksum(targetByteData)
	if existingDataChecksum == newDataChecksum {
		log.Info("Target ConfigMap already up to date", "namespace", targetKey.Namespace, "name", targetKey.Name, "mode", syncMode)
		return nil
	}

	// Update existing ConfigMap
	existing.Data = targetData
	if existing.Annotations == nil {
		existing.Annotations = make(map[string]string)
	}
	for k, v := range annotations {
		existing.Annotations[k] = v
	}

	log.Info("Updating target ConfigMap", "namespace", targetKey.Namespace, "name", targetKey.Name, "mode", syncMode)
	return r.Update(ctx, &existing)
}

// deleteTargetResources removes all synced resources when DeletionPolicy is "delete".
//
// Safety checks:
// - Only deletes resources with our managed-by annotation
// - Continues on NotFound errors (idempotent)
func (r *SharedResourceReconciler) deleteTargetResources(ctx context.Context, sr *platformv1alpha1.SharedResource) error {
	log := logf.FromContext(ctx)

	for _, target := range sr.Spec.Targets {
		targetName := target.Name
		if targetName == "" {
			targetName = sr.Spec.Source.Name
		}

		targetKey := types.NamespacedName{Namespace: target.Namespace, Name: targetName}

		switch sr.Spec.Source.Kind {
		case KindSecret:
			var secret corev1.Secret
			if err := r.Get(ctx, targetKey, &secret); err != nil {
				if apierrors.IsNotFound(err) {
					continue // Already deleted
				}
				return err
			}
			// Only delete if managed by us (safety check)
			if secret.Annotations[AnnotationManagedBy] == ManagedByValue {
				log.Info("Deleting target Secret", "namespace", target.Namespace, "name", targetName)
				if err := r.Delete(ctx, &secret); err != nil && !apierrors.IsNotFound(err) {
					return err
				}
			}

		case KindConfigMap:
			var cm corev1.ConfigMap
			if err := r.Get(ctx, targetKey, &cm); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return err
			}
			if cm.Annotations[AnnotationManagedBy] == ManagedByValue {
				log.Info("Deleting target ConfigMap", "namespace", target.Namespace, "name", targetName)
				if err := r.Delete(ctx, &cm); err != nil && !apierrors.IsNotFound(err) {
					return err
				}
			}
		}
	}

	return nil
}
