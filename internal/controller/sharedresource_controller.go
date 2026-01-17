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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/vijay-papanaboina/sharedresource-operator/api/v1alpha1"
)

// =============================================================================
// SharedResourceReconciler reconciles a SharedResource object.
//
// The reconciler's job is to ensure that the declared sync intent (SharedResource CR)
// matches the actual cluster state (target Secrets/ConfigMaps exist with correct data).
//
// Related files:
// - constants.go: Annotation keys, finalizer name, condition types
// - helpers.go: Utility functions (checksum, filtering, conditions)
// - sync.go: Secret/ConfigMap sync operations
// =============================================================================
type SharedResourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// =============================================================================
// RBAC Markers - Generate ClusterRole permissions in config/rbac/role.yaml
//
// Run 'make manifests' after modifying these to regenerate RBAC rules.
// =============================================================================

// +kubebuilder:rbac:groups=platform.platform.dev,resources=sharedresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.platform.dev,resources=sharedresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.platform.dev,resources=sharedresources/finalizers,verbs=update

// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// =============================================================================
// Reconcile is the core reconciliation loop.
//
// This is the heart of the operator. It's called whenever:
// - A SharedResource CR is created, updated, or deleted
// - The operator restarts
//
// The goal: Make actual cluster state match the desired state in the CR.
// =============================================================================

func (r *SharedResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Starting reconciliation", "sharedresource", req.NamespacedName)

	// -------------------------------------------------------------------------
	// Step 1: Fetch the SharedResource CR
	// -------------------------------------------------------------------------
	var sharedResource platformv1alpha1.SharedResource
	if err := r.Get(ctx, req.NamespacedName, &sharedResource); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("SharedResource not found, likely deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to fetch SharedResource")
		return ctrl.Result{}, err
	}

	// -------------------------------------------------------------------------
	// Step 2: Handle deletion with finalizer
	// -------------------------------------------------------------------------
	if !sharedResource.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &sharedResource, log)
	}

	// -------------------------------------------------------------------------
	// Step 3: Add finalizer if not present
	// -------------------------------------------------------------------------
	if !controllerutil.ContainsFinalizer(&sharedResource, FinalizerName) {
		log.Info("Adding finalizer")
		controllerutil.AddFinalizer(&sharedResource, FinalizerName)
		if err := r.Update(ctx, &sharedResource); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// -------------------------------------------------------------------------
	// Step 4: Fetch the source resource
	// -------------------------------------------------------------------------
	sourceData, sourceType, err := r.fetchSourceResource(ctx, &sharedResource)
	if err != nil {
		return r.handleSourceError(ctx, &sharedResource, err, log)
	}

	// Source found - update condition
	setCondition(&sharedResource, ConditionTypeSourceFound, metav1.ConditionTrue, "SourceExists", "Source resource found")

	// -------------------------------------------------------------------------
	// Step 5: Compute checksum for drift detection
	// -------------------------------------------------------------------------
	filteredData := filterData(sourceData, sharedResource.Spec.SyncPolicy)
	checksum := computeChecksum(filteredData)
	log.Info("Computed source checksum", "checksum", checksum)

	// -------------------------------------------------------------------------
	// Step 6: Sync to each target namespace
	// -------------------------------------------------------------------------
	syncedTargets, allSynced := r.syncAllTargets(ctx, &sharedResource, filteredData, sourceType, checksum, log)

	// -------------------------------------------------------------------------
	// Step 7: Update status
	// -------------------------------------------------------------------------
	return r.updateStatus(ctx, &sharedResource, syncedTargets, checksum, allSynced, log)
}

// handleDeletion processes the SharedResource deletion with finalizer cleanup.
func (r *SharedResourceReconciler) handleDeletion(ctx context.Context, sr *platformv1alpha1.SharedResource, log logr.Logger) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(sr, FinalizerName) {
		log.Info("Processing finalizer for deletion")

		// Only delete targets if DeletionPolicy is "delete"
		if sr.Spec.DeletionPolicy == platformv1alpha1.DeletionPolicyDelete {
			if err := r.deleteTargetResources(ctx, sr); err != nil {
				log.Error(err, "Failed to delete target resources")
				return ctrl.Result{}, err
			}
			log.Info("Deleted target resources per DeletionPolicy")
		} else {
			log.Info("Orphaning target resources per DeletionPolicy")
		}

		// Remove finalizer to allow CR deletion to proceed
		controllerutil.RemoveFinalizer(sr, FinalizerName)
		if err := r.Update(ctx, sr); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// handleSourceError updates status when source resource is not found.
func (r *SharedResourceReconciler) handleSourceError(ctx context.Context, sr *platformv1alpha1.SharedResource, err error, log logr.Logger) (ctrl.Result, error) {
	if apierrors.IsNotFound(err) {
		log.Info("Source resource not found", "kind", sr.Spec.Source.Kind, "name", sr.Spec.Source.Name)

		setCondition(sr, ConditionTypeSourceFound, metav1.ConditionFalse, "SourceNotFound",
			fmt.Sprintf("Source %s/%s not found", sr.Spec.Source.Kind, sr.Spec.Source.Name))
		setCondition(sr, ConditionTypeReady, metav1.ConditionFalse, "SourceNotFound", "Cannot sync: source resource not found")

		if statusErr := r.Status().Update(ctx, sr); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		// Requeue after delay to check if source appears
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	log.Error(err, "Failed to fetch source resource")
	return ctrl.Result{}, err
}

// syncAllTargets syncs the source data to all target namespaces.
func (r *SharedResourceReconciler) syncAllTargets(
	ctx context.Context,
	sr *platformv1alpha1.SharedResource,
	data map[string][]byte,
	sourceType corev1.SecretType,
	checksum string,
	log logr.Logger,
) ([]platformv1alpha1.TargetSyncStatus, bool) {
	syncedTargets := make([]platformv1alpha1.TargetSyncStatus, 0, len(sr.Spec.Targets))
	allSynced := true
	now := metav1.Now()

	for _, target := range sr.Spec.Targets {
		// Determine target resource name
		targetName := target.Name
		if targetName == "" {
			targetName = sr.Spec.Source.Name
		}

		targetStatus := platformv1alpha1.TargetSyncStatus{
			Namespace: target.Namespace,
			Name:      targetName,
		}

		// Sync to this target
		err := r.syncToTarget(ctx, sr, target.Namespace, targetName, data, sourceType, checksum)
		if err != nil {
			log.Error(err, "Failed to sync to target", "namespace", target.Namespace, "name", targetName)
			targetStatus.Synced = false
			targetStatus.Error = err.Error()
			allSynced = false
		} else {
			log.Info("Successfully synced to target", "namespace", target.Namespace, "name", targetName)
			targetStatus.Synced = true
			targetStatus.LastSynced = now
		}

		syncedTargets = append(syncedTargets, targetStatus)
	}

	return syncedTargets, allSynced
}

// updateStatus updates the SharedResource status with sync results.
func (r *SharedResourceReconciler) updateStatus(
	ctx context.Context,
	sr *platformv1alpha1.SharedResource,
	syncedTargets []platformv1alpha1.TargetSyncStatus,
	checksum string,
	allSynced bool,
	log logr.Logger,
) (ctrl.Result, error) {
	now := metav1.Now()

	sr.Status.SyncedTargets = syncedTargets
	sr.Status.SourceChecksum = checksum

	if allSynced {
		sr.Status.LastSyncTime = &now
		setCondition(sr, ConditionTypeReady, metav1.ConditionTrue, "SyncSuccessful", "All targets synced successfully")
	} else {
		setCondition(sr, ConditionTypeReady, metav1.ConditionFalse, "SyncFailed", "Some targets failed to sync")
	}

	if err := r.Status().Update(ctx, sr); err != nil {
		log.Error(err, "Failed to update SharedResource status")
		return ctrl.Result{}, err
	}

	log.Info("Reconciliation complete", "allSynced", allSynced)
	return ctrl.Result{}, nil
}

// =============================================================================
// SetupWithManager registers the controller with the Manager.
//
// We watch:
// 1. SharedResource CRs - primary resource
// 2. Secrets - to trigger sync when source secrets change
// 3. ConfigMaps - to trigger sync when source configmaps change
// =============================================================================
func (r *SharedResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.SharedResource{}).
		// Watch Secrets and map back to SharedResources that reference them
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findSharedResourcesForSecret),
		).
		// Watch ConfigMaps and map back to SharedResources that reference them
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findSharedResourcesForConfigMap),
		).
		Named("sharedresource").
		Complete(r)
}

// findSharedResourcesForSecret returns reconcile requests for all SharedResources
// that are affected by the changed Secret (either as source or as target).
func (r *SharedResourceReconciler) findSharedResourcesForSecret(ctx context.Context, obj client.Object) []ctrl.Request {
	secret := obj.(*corev1.Secret)

	// Check if this is a managed target resource
	if managedBy, ok := secret.Annotations[AnnotationManagedBy]; ok && managedBy == ManagedByValue {
		return r.findSharedResourceForManagedResource(ctx, secret.Annotations, "Secret")
	}

	// Otherwise, check if it's a source resource
	return r.findSharedResourcesForSource(ctx, secret.Namespace, secret.Name, "Secret")
}

// findSharedResourcesForConfigMap returns reconcile requests for all SharedResources
// that are affected by the changed ConfigMap (either as source or as target).
func (r *SharedResourceReconciler) findSharedResourcesForConfigMap(ctx context.Context, obj client.Object) []ctrl.Request {
	cm := obj.(*corev1.ConfigMap)

	// Check if this is a managed target resource
	if managedBy, ok := cm.Annotations[AnnotationManagedBy]; ok && managedBy == ManagedByValue {
		return r.findSharedResourceForManagedResource(ctx, cm.Annotations, "ConfigMap")
	}

	// Otherwise, check if it's a source resource
	return r.findSharedResourcesForSource(ctx, cm.Namespace, cm.Name, "ConfigMap")
}

// findSharedResourceForManagedResource returns a reconcile request for the SharedResource
// that owns the managed target resource (based on annotations).
func (r *SharedResourceReconciler) findSharedResourceForManagedResource(ctx context.Context, annotations map[string]string, kind string) []ctrl.Request {
	log := logf.FromContext(ctx)

	sourceNamespace := annotations[AnnotationSourceNamespace]
	sourceCR := annotations[AnnotationSourceCR]

	if sourceNamespace == "" || sourceCR == "" {
		return nil
	}

	log.Info("Managed target resource changed, triggering reconcile",
		"kind", kind,
		"sharedresource", sourceCR)

	return []ctrl.Request{{
		NamespacedName: client.ObjectKey{
			Namespace: sourceNamespace,
			Name:      sourceCR,
		},
	}}
}

// findSharedResourcesForSource finds all SharedResources in the given namespace
// that reference the specified source resource.
func (r *SharedResourceReconciler) findSharedResourcesForSource(ctx context.Context, namespace, name, kind string) []ctrl.Request {
	log := logf.FromContext(ctx)

	// List all SharedResources in the same namespace as the source
	var sharedResourceList platformv1alpha1.SharedResourceList
	if err := r.List(ctx, &sharedResourceList, client.InNamespace(namespace)); err != nil {
		log.Error(err, "Failed to list SharedResources")
		return nil
	}

	var requests []ctrl.Request
	for _, sr := range sharedResourceList.Items {
		// Check if this SharedResource references the changed resource
		if sr.Spec.Source.Kind == kind && sr.Spec.Source.Name == name {
			log.Info("Source resource changed, triggering reconcile",
				"source", kind+"/"+name,
				"sharedresource", sr.Name)
			requests = append(requests, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Namespace: sr.Namespace,
					Name:      sr.Name,
				},
			})
		}
	}

	return requests
}
