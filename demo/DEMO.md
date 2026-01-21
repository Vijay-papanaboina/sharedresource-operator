# SharedResource Operator Demo

## Prerequisites

```bash
# Install the SharedResource Operator from the official release
kubectl apply -f https://github.com/Vijay-papanaboina/sharedresource-operator/releases/download/v0.1.0/install.yaml
```

## Setup

```bash
# Create test namespaces for source and target
kubectl apply -f demo/namespaces.yaml

# Create a source secret in the shared-resources namespace
kubectl apply -f demo/source-secret.yaml
```

## Sync Demo

```bash
# Apply the SharedResource CR to declare sync intent
kubectl apply -f demo/sharedresource.yaml

# Verify the secret is synced to the target namespace
kubectl get secrets -n app-namespace

# Check the synced secret's value
kubectl get secret db-credentials -n app-namespace -o jsonpath='{.data.password}' | base64 -d
```

## Update Demo

```bash
# Apply the updated source secret
kubectl apply -f demo/updated-secret.yaml

# Verify the target secret automatically received the new password
kubectl get secret db-credentials -n app-namespace -o jsonpath='{.data.password}' | base64 -d
```

## Deletion Demo

```bash
# Delete the source secret
kubectl delete -f demo/source-secret.yaml

# Check that target is also deleted (deletionPolicy: delete)
kubectl get secrets -n app-namespace
```
