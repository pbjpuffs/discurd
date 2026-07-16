# Discurd on Kubernetes

Manifests for running the Discurd app services (api, gateway, web) on Kubernetes.
Narrative walkthrough: [docs/DEPLOYMENT.md §4](../../docs/DEPLOYMENT.md#4-kubernetes).

## What's here

| File | Contents |
|---|---|
| `namespace.yaml` | `discurd` namespace |
| `configmap.yaml` | non-secret env (ARCHITECTURE.md §10) — infra addresses, TTLs, limits |
| `secret.example.yaml` | **example** `JWT_SECRET` / MinIO credentials — replace before production |
| `api.yaml` | api Deployment (3 replicas) + Service :8080 |
| `gateway.yaml` | gateway Deployment (2 replicas) + Service :8080 |
| `web.yaml` | web Deployment (2 replicas) + Service :80 |
| `ingress.yaml` | nginx Ingress: `/api`, `/ws`, `/files` (rewritten), `/` |
| `hpa.yaml` | HPAs for api and gateway (CPU 70%) |
| `scylla-cluster.example.yaml` | example ScyllaCluster CR — **requires the Scylla Operator**, not in kustomization |
| `kustomization.yaml` | ties the above together; set your image names/tags here |

## Prerequisites

- **ingress-nginx** (the Ingress uses `ingressClassName: nginx` and nginx annotations)
- **metrics-server** (for the HPAs)
- **Scylla Operator** + cert-manager (for the ScyllaCluster CR)
- **Helm** (for NATS, Redis, MinIO)
- App images built and pushed — see docs/DEPLOYMENT.md §4.1:
  `docker build -f backend/Dockerfile.api …`, `Dockerfile.gateway`, `web/Dockerfile`.

Redis, NATS and MinIO are deliberately **not** manifested here — bring them from Helm
charts or managed services and point the ConfigMap at them.

## Apply order

```sh
# 1. Infra: Scylla (after installing the Scylla Operator)
kubectl apply -f deploy/k8s/scylla-cluster.example.yaml
# wait for 3 ready members:
kubectl -n scylla get scyllacluster discurd-scylla -w

# 2. Infra: NATS + Redis (Helm)
helm repo add nats https://nats-io.github.io/k8s/helm/charts/
helm install nats nats/nats --namespace nats --create-namespace
helm repo add bitnami https://charts.bitnami.com/bitnami
helm install redis bitnami/redis --namespace redis --create-namespace \
  --set architecture=standalone --set auth.enabled=false

# 3. Infra: MinIO — into the discurd namespace, service name `minio`
#    (the /files Ingress routes to it). Credentials must match the app Secret.
helm repo add minio https://charts.min.io/
helm install minio minio/minio --namespace discurd --create-namespace \
  --set fullnameOverride=minio --set mode=standalone \
  --set rootUser=<user> --set rootPassword=<password> \
  --set "buckets[0].name=avatars,buckets[0].policy=download" \
  --set "buckets[1].name=attachments,buckets[1].policy=download"

# 4. Schema + keyspace replication (from the repo root)
kubectl -n scylla run schema-init --rm -i --image=scylladb/scylla:6.2 --restart=Never \
  --command -- cqlsh discurd-scylla-client -e "$(cat db/schema.cql)"
kubectl -n scylla run alter-ks --rm -i --image=scylladb/scylla:6.2 --restart=Never \
  --command -- cqlsh discurd-scylla-client -e \
  "ALTER KEYSPACE discurd WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3};"

# 5. The app: edit kustomization.yaml (images:), provide the real secret, then
kubectl apply -k deploy/k8s
kubectl -n discurd rollout status deploy/api
kubectl -n discurd rollout status deploy/gateway
kubectl -n discurd rollout status deploy/web

# 6. Optional demo data
kubectl -n discurd exec deploy/api -- /app/seed
```

Applying with plain `-f` instead of `-k`? Use this order (and substitute the image
placeholders yourself): `namespace.yaml`, `configmap.yaml`, your secret, `api.yaml`,
`gateway.yaml`, `web.yaml`, `ingress.yaml`, `hpa.yaml`.

## Pointing at differently-named infra

Everything the services need is in `configmap.yaml` / the Secret — change these values
if your infra lives elsewhere (env var names are the contract, ARCHITECTURE.md §10):

| Env var | Set to |
|---|---|
| `SCYLLA_HOSTS` | comma-separated CQL hosts, e.g. `discurd-scylla-client.scylla.svc.cluster.local` |
| `SCYLLA_KEYSPACE` | `discurd` |
| `REDIS_ADDR` | `host:6379`, e.g. `redis-master.redis.svc.cluster.local:6379` (no auth — the contract has no password var; isolate with a NetworkPolicy) |
| `NATS_URL` | `nats://host:4222`, e.g. `nats://nats.nats.svc.cluster.local:4222` |
| `MINIO_ENDPOINT` | `host:port` of any S3-compatible endpoint, e.g. `minio:9000` or `s3.eu-central-1.amazonaws.com` |
| `MINIO_USE_SSL` | `true` when the endpoint is HTTPS (real S3) |
| `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD` | MinIO credentials or S3 access/secret key (in the Secret) |
| `JWT_SECRET` | strong random value (in the Secret) |

Swapping MinIO for real S3: create buckets `avatars` and `attachments` with
public-read GET, and keep `/files/*` resolving — point the `discurd-files` Ingress at
an `ExternalName` Service for the S3 endpoint or serve the buckets via CDN
(details in docs/DEPLOYMENT.md §4.4).
