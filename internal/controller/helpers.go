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
	"crypto/sha256"
	"encoding/hex"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	platformv1alpha1 "github.com/vijay-papanaboina/sharedresource-operator/api/v1alpha1"
)

// =============================================================================
// Helper functions for the SharedResource controller.
//
// These are utility functions that don't directly interact with the
// Kubernetes API but provide supporting logic for the reconciler.
// =============================================================================

// computeChecksum generates a SHA256 hash of the data for drift detection.
//
// Why checksums?
// - Avoids unnecessary updates when data hasn't changed
// - Keys are sorted for deterministic hashes regardless of map iteration order
// - Stored as annotation on target resources for comparison
func computeChecksum(data map[string][]byte) string {
	// Sort keys for deterministic ordering
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Hash key-value pairs
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte("="))
		h.Write(data[k])
		h.Write([]byte("\n"))
	}

	return hex.EncodeToString(h.Sum(nil))
}

// filterData applies the SyncPolicy to filter which keys to sync.
//
// Filtering modes:
// - "copy" (default): All keys are synced
// - "selective": Only keys matching Include/Exclude rules are synced
func filterData(data map[string][]byte, policy *platformv1alpha1.SyncPolicySpec) map[string][]byte {
	// If no policy or copy mode, return all data
	if policy == nil || policy.Mode == "" || policy.Mode == platformv1alpha1.SyncModeCopy {
		return data
	}

	// Selective mode - apply key filtering
	if policy.Keys == nil {
		return data
	}

	filtered := make(map[string][]byte)

	// If Include is specified, only include those keys
	if len(policy.Keys.Include) > 0 {
		for _, key := range policy.Keys.Include {
			if val, ok := data[key]; ok {
				filtered[key] = val
			}
		}
	} else {
		// No Include list means start with all keys
		for k, v := range data {
			filtered[k] = v
		}
	}

	// Apply Exclude filter
	for _, key := range policy.Keys.Exclude {
		delete(filtered, key)
	}

	return filtered
}

// setCondition updates or adds a condition to the SharedResource status.
//
// This follows Kubernetes conventions:
// - Each condition type appears at most once
// - LastTransitionTime only updates when status changes
// - Reason and Message can update without changing transition time
func setCondition(sr *platformv1alpha1.SharedResource, condType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}

	// Find and update existing condition, or append new one
	for i, existing := range sr.Status.Conditions {
		if existing.Type == condType {
			if existing.Status != status {
				// Status changed - update everything including transition time
				sr.Status.Conditions[i] = condition
			} else {
				// Status unchanged - keep transition time, update reason/message
				sr.Status.Conditions[i].Reason = reason
				sr.Status.Conditions[i].Message = message
			}
			return
		}
	}

	// Condition doesn't exist, append it
	sr.Status.Conditions = append(sr.Status.Conditions, condition)
}
