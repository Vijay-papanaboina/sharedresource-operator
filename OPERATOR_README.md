# SharedResource Operator

A Kubernetes operator that synchronizes Secrets and ConfigMaps across namespaces using explicit, auditable intent.

## Problem

Kubernetes namespaces are isolation boundaries, but real clusters often need to share configuration:

- TLS certificates used by multiple services
- Database credentials across microservices
- Feature flags duplicated across environments

Manual copying is error-prone and breaks during secret rotation.

## Solution

Declare sync intent with a `SharedResource` CR:

```yaml
apiVersion: platform.platform.dev/v1alpha1
kind: SharedResource
metadata:
  name: sync-db-credentials
  namespace: security
spec:
  source:
    kind: Secret
    name: db-credentials
  targets:
    - namespace: backend
    - namespace: jobs
  deletionPolicy: orphan
```

The operator continuously syncs the source to all targets.

---

## Architecture

### Reconciliation Loop

```mermaid
flowchart TD
    subgraph Trigger["Trigger Events"]
        A[SharedResource Created/Updated/Deleted]
    end

    subgraph Step1["Step 1: Fetch CR"]
        B["r.Get(ctx, req.NamespacedName, &sharedResource)"]
        B1{CR Found?}
        B2[Return - Nothing to do]
    end

    subgraph Step2["Step 2: Handle Deletion"]
        C{"DeletionTimestamp != Zero?"}
        D["handleDeletion()"]
        D1{"DeletionPolicy == delete?"}
        D2["deleteTargetResources()"]
        D3["RemoveFinalizer()"]
        D4[Orphan targets]
    end

    subgraph Step3["Step 3: Ensure Finalizer"]
        E{"HasFinalizer?"}
        F["AddFinalizer()"]
        G[Requeue]
    end

    subgraph Step4["Step 4: Fetch Source"]
        H["fetchSourceResource()"]
        H1{Source Found?}
        H2["handleSourceError()"]
        H3["setCondition(SourceNotFound)"]
        H4[Requeue after 30s]
    end

    subgraph Step5["Step 5: Compute Checksum"]
        I["filterData()"]
        J["computeChecksum()"]
    end

    subgraph Step6["Step 6: Sync Targets"]
        K["syncAllTargets()"]
        L["For each target:"]
        M["syncToTarget()"]
        N{"Source.Kind?"}
        O["syncSecret()"]
        P["syncConfigMap()"]
        Q{Checksum Match?}
        R[Skip - Already up to date]
        S["r.Create() or r.Update()"]
    end

    subgraph Step7["Step 7: Update Status"]
        T["updateStatus()"]
        U["setCondition(Ready)"]
        V["r.Status().Update()"]
    end

    A --> B
    B --> B1
    B1 -->|No| B2
    B1 -->|Yes| C

    C -->|Yes| D
    D --> D1
    D1 -->|Yes| D2
    D1 -->|No| D4
    D2 --> D3
    D4 --> D3

    C -->|No| E
    E -->|No| F
    F --> G
    E -->|Yes| H

    H --> H1
    H1 -->|No| H2
    H2 --> H3
    H3 --> H4
    H1 -->|Yes| I

    I --> J
    J --> K
    K --> L
    L --> M
    M --> N
    N -->|Secret| O
    N -->|ConfigMap| P
    O --> Q
    P --> Q
    Q -->|Yes| R
    Q -->|No| S

    S --> T
    R --> T
    T --> U
    U --> V
```

---

## Project Structure

```
internal/controller/
├── constants.go              # Annotation keys, finalizer name
├── helpers.go                # computeChecksum(), filterData(), setCondition()
├── sync.go                   # fetchSourceResource(), syncToTarget(), syncSecret(), syncConfigMap()
└── sharedresource_controller.go  # Reconcile(), handleDeletion(), updateStatus()
```

---

## Key Functions

| Function                  | File                         | Purpose                                     |
| ------------------------- | ---------------------------- | ------------------------------------------- |
| `Reconcile()`             | sharedresource_controller.go | Main reconciliation loop entry point        |
| `handleDeletion()`        | sharedresource_controller.go | Finalizer cleanup on CR deletion            |
| `handleSourceError()`     | sharedresource_controller.go | Handle missing source resource              |
| `syncAllTargets()`        | sharedresource_controller.go | Iterate and sync to each target namespace   |
| `updateStatus()`          | sharedresource_controller.go | Update CR status with sync results          |
| `fetchSourceResource()`   | sync.go                      | Get source Secret or ConfigMap data         |
| `syncToTarget()`          | sync.go                      | Orchestrate sync to single target           |
| `syncSecret()`            | sync.go                      | Create/update Secret in target namespace    |
| `syncConfigMap()`         | sync.go                      | Create/update ConfigMap in target namespace |
| `deleteTargetResources()` | sync.go                      | Delete synced resources (delete policy)     |
| `computeChecksum()`       | helpers.go                   | SHA256 hash of data for drift detection     |
| `filterData()`            | helpers.go                   | Apply include/exclude key filters           |
| `setCondition()`          | helpers.go                   | Update status conditions                    |

---

## Annotations on Synced Resources

```yaml
annotations:
  sharedresource.platform.dev/managed-by: sharedresource-operator
  sharedresource.platform.dev/source-namespace: security
  sharedresource.platform.dev/source-name: db-credentials
  sharedresource.platform.dev/source-cr: sync-db-credentials
  sharedresource.platform.dev/checksum: "a1b2c3..."
  sharedresource.platform.dev/last-synced: "2026-01-17T12:00:00Z"
```

---

## Quick Start

```bash
# Install CRDs
make install

# Run operator locally
make run

# Create test resources
kubectl create namespace security
kubectl create namespace backend
kubectl create secret generic db-credentials -n security \
  --from-literal=username=admin \
  --from-literal=password=secret123

# Apply sample SharedResource
kubectl apply -f config/samples/platform_v1alpha1_sharedresource.yaml

# Verify sync
kubectl get secrets db-credentials -n backend -o yaml
```

---

## Design Philosophy

> "Kubernetes avoids cross-namespace secret sharing for good reasons. This operator respects those boundaries while giving teams an explicit, auditable way to synchronize shared configuration."

- **Explicit intent**: No implicit propagation - sync must be declared
- **Auditability**: Annotations track source and sync history
- **Safety**: Orphan deletion policy by default
- **RBAC-aware**: Operator needs explicit permissions per namespace
