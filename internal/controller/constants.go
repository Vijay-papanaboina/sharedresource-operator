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

// =============================================================================
// Constants for the SharedResource operator.
//
// These are used for:
// - Finalizer management (cleanup before deletion)
// - Resource annotations (tracking, auditing, drift detection)
// - Status conditions (health reporting)
// =============================================================================

// Finalizer name used to ensure cleanup happens before deletion
const FinalizerName = "sharedresource.platform.dev/finalizer"

// =============================================================================
// Annotations applied to synced target resources.
// These enable tracking which operator manages the resource,
// where the source data came from, and drift detection via checksums.
// =============================================================================
const (
	// AnnotationManagedBy identifies this resource is managed by our operator
	AnnotationManagedBy = "sharedresource.platform.dev/managed-by"

	// AnnotationSourceNamespace records the namespace of the source resource
	AnnotationSourceNamespace = "sharedresource.platform.dev/source-namespace"

	// AnnotationSourceName records the name of the source resource
	AnnotationSourceName = "sharedresource.platform.dev/source-name"

	// AnnotationSourceCR records the name of the SharedResource CR
	AnnotationSourceCR = "sharedresource.platform.dev/source-cr"

	// AnnotationChecksum stores SHA256 hash of synced data for drift detection
	AnnotationChecksum = "sharedresource.platform.dev/checksum"

	// AnnotationLastSynced records when the resource was last synced
	AnnotationLastSynced = "sharedresource.platform.dev/last-synced"

	// ManagedByValue is the value for AnnotationManagedBy
	ManagedByValue = "sharedresource-operator"
)

// =============================================================================
// Condition types for SharedResource status.
// These follow Kubernetes conventions for reporting resource health.
// =============================================================================
const (
	// ConditionTypeReady indicates overall sync health
	// True = all targets synced, False = some failed
	ConditionTypeReady = "Ready"

	// ConditionTypeSourceFound indicates if source Secret/ConfigMap exists
	// True = source exists, False = source not found
	ConditionTypeSourceFound = "SourceFound"
)
