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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// =============================================================================
// SharedResourceSpec defines the desired state of SharedResource.
//
// This is where users declare WHAT they want to sync and WHERE:
//   - Source: The Secret or ConfigMap to copy FROM (must exist in same namespace as this CR)
//   - Targets: List of namespaces to copy TO
//   - SyncPolicy: How to filter/transform data during sync
//   - DeletionPolicy: What happens to synced resources when this CR is deleted
//
// =============================================================================
type SharedResourceSpec struct {
	// Source specifies the Secret or ConfigMap to synchronize.
	// The source resource must exist in the SAME namespace as this SharedResource CR.
	// This design ensures the team owning the secret also controls its distribution.
	//
	// Example:
	//   source:
	//     kind: Secret
	//     name: db-credentials
	//
	// +required
	Source SourceSpec `json:"source"`

	// Targets lists the namespaces where the source should be synchronized.
	// Each target can optionally rename the resource in that namespace.
	//
	// Example:
	//   targets:
	//     - namespace: backend
	//     - namespace: jobs
	//       name: database-creds  # Optional: rename in this namespace
	//
	// +required
	// +kubebuilder:validation:MinItems=1
	Targets []TargetSpec `json:"targets"`

	// SyncPolicy configures how data is copied to targets.
	// By default, all keys are copied. Use selective mode to filter specific keys.
	//
	// +optional
	SyncPolicy *SyncPolicySpec `json:"syncPolicy,omitempty"`

	// DeletionPolicy determines what happens to target resources when this
	// SharedResource CR is deleted.
	//   - "orphan" (default): Target resources are left in place (safe)
	//   - "delete": Target resources are deleted (use with caution)
	//
	// +kubebuilder:validation:Enum=orphan;delete
	// +kubebuilder:default=orphan
	// +optional
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// =============================================================================
// SourceSpec identifies the source Secret or ConfigMap to sync.
// =============================================================================
type SourceSpec struct {
	// Kind specifies the type of Kubernetes resource to sync.
	// Must be either "Secret" or "ConfigMap".
	//
	// Note: TLS secrets (type: kubernetes.io/tls) are still "Secret" kind -
	// the secret type is preserved during sync.
	//
	// +kubebuilder:validation:Enum=Secret;ConfigMap
	// +required
	Kind string `json:"kind"`

	// Name is the name of the source resource in the SharedResource's namespace.
	//
	// +required
	Name string `json:"name"`
}

// =============================================================================
// TargetSpec identifies a destination namespace for synchronization.
// =============================================================================
type TargetSpec struct {
	// Namespace is the target namespace to sync the resource to.
	// The namespace must already exist - the operator will NOT create it.
	//
	// +required
	Namespace string `json:"namespace"`

	// Name optionally overrides the resource name in the target namespace.
	// If not specified, the source resource's name is used.
	//
	// Use case: When the target namespace already has a resource with the
	// same name, or when different naming conventions are required.
	//
	// +optional
	Name string `json:"name,omitempty"`
}

// =============================================================================
// SyncPolicySpec configures how data is filtered during synchronization.
// =============================================================================
type SyncPolicySpec struct {
	// Mode determines the sync strategy:
	//   - "copy" (default): Sync all keys from source to target
	//   - "selective": Only sync keys specified in the Keys field
	//
	// +kubebuilder:validation:Enum=copy;selective
	// +kubebuilder:default=copy
	// +optional
	Mode SyncMode `json:"mode,omitempty"`

	// Keys specifies which keys to include or exclude.
	// Only used when Mode is "selective".
	//
	// +optional
	Keys *KeySelector `json:"keys,omitempty"`
}

// SyncMode defines how data is copied during synchronization.
// +kubebuilder:validation:Enum=copy;selective
type SyncMode string

const (
	// SyncModeCopy copies all keys from source to target (default behavior)
	SyncModeCopy SyncMode = "copy"

	// SyncModeSelective copies only specified keys from source to target
	SyncModeSelective SyncMode = "selective"
)

// DeletionPolicy defines what happens to target resources when the SharedResource is deleted.
// +kubebuilder:validation:Enum=orphan;delete
type DeletionPolicy string

const (
	// DeletionPolicyOrphan leaves target resources in place when SharedResource is deleted.
	// This is the safe default - resources continue to exist for running workloads.
	DeletionPolicyOrphan DeletionPolicy = "orphan"

	// DeletionPolicyDelete removes target resources when SharedResource is deleted.
	// Use with caution - this could break running workloads that depend on these resources.
	DeletionPolicyDelete DeletionPolicy = "delete"
)

// =============================================================================
// KeySelector specifies which keys to include or exclude during selective sync.
// =============================================================================
type KeySelector struct {
	// Include lists the keys to sync. If empty, all keys are synced.
	// When specified, ONLY these keys are copied to targets.
	//
	// Example: Only sync username and password, not connection-string
	//   keys:
	//     include:
	//       - username
	//       - password
	//
	// +optional
	Include []string `json:"include,omitempty"`

	// Exclude lists keys to skip during sync.
	// Applied after Include filter.
	//
	// Example: Sync everything except internal metadata
	//   keys:
	//     exclude:
	//       - internal-metadata
	//
	// +optional
	Exclude []string `json:"exclude,omitempty"`
}

// =============================================================================
// SharedResourceStatus defines the observed state of SharedResource.
//
// This is updated by the controller to reflect the actual sync state.
// Users can check this to see if sync is healthy or has errors.
// =============================================================================
type SharedResourceStatus struct {
	// Conditions represent the overall state of the SharedResource.
	// Standard condition types:
	//   - "Ready": True when all targets are successfully synced
	//   - "SourceFound": True when the source resource exists
	//   - "Degraded": True when some (but not all) targets failed to sync
	//
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// SyncedTargets shows the sync status for each target namespace.
	// This allows users to see which targets succeeded and which failed.
	//
	// +optional
	SyncedTargets []TargetSyncStatus `json:"syncedTargets,omitempty"`

	// LastSyncTime is the timestamp of the last successful full sync.
	//
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// SourceChecksum is the SHA256 hash of the source resource's data.
	// Used for drift detection - if source changes, checksum changes,
	// triggering a re-sync to all targets.
	//
	// +optional
	SourceChecksum string `json:"sourceChecksum,omitempty"`
}

// =============================================================================
// TargetSyncStatus tracks sync status for a single target namespace.
// =============================================================================
type TargetSyncStatus struct {
	// Namespace is the target namespace
	Namespace string `json:"namespace"`

	// Name is the resource name in the target namespace
	Name string `json:"name"`

	// Synced indicates whether the sync to this target was successful
	Synced bool `json:"synced"`

	// LastSynced is when this target was last successfully synced
	// +optional
	LastSynced metav1.Time `json:"lastSynced,omitempty"`

	// Error contains the error message if sync failed for this target
	// +optional
	Error string `json:"error,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// SharedResource is the Schema for the sharedresources API
type SharedResource struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of SharedResource
	// +required
	Spec SharedResourceSpec `json:"spec"`

	// status defines the observed state of SharedResource
	// +optional
	Status SharedResourceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SharedResourceList contains a list of SharedResource
type SharedResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []SharedResource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SharedResource{}, &SharedResourceList{})
}
