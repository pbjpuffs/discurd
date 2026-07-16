# Discurd — Deployment Guide

From a laptop to production, in four steps:

1. [Local single-node](#1-local-single-node) — the default `docker compose up`.
2. [Scale-out on one host](#2-scale-out-on-one-host) — 3-node Scylla + service replicas via the overlay.
3. [Production checklist](#3-production-checklist) — secrets, TLS, Scylla tuning, backups, pinning, logs, alerts.
4. [Kubernetes](#4-kubernetes) — the manifests in `deploy/k8s/`.

Everything here follows the contract in [ARCHITECTURE.md](ARCHITECTURE.md) — same env
var names, ports and paths everywhere.

---

## 1. Local single-node

```sh
cp .env.example .env
docker compose up -d --build
```

What comes up (see `docker-compose.yml`):

| Service | Role |
|---|---|
| `traefik` | edge on `:80` — `/api` → api, `/ws` → gateway, `/files` (prefix stripped) → MinIO, `/` → web |
| `scylla` + `scylla-init` | single Scylla node (developer mode); init job applies `db/schema.cql` |
| `redis`, `nats`, `minio` + `minio-init` | presence/sessions/rate-limits, event fan-out, object storage (buckets `avatars`/`attachments`, anonymous download) |
| `api`, `gateway`, `web` | the application |
| `prometheus`, `grafana`, `redis-exporter`, `nats-exporter` | observability |

`api` and `gateway` wait for `scylla-init` / `minio-init` to complete and for
Redis/NATS health, then retry infra connections with backoff for ~60 s — so a cold
first boot self-heals.

Seed demo data (guild **Discurd HQ**, channels `#general` `#random` `#dev`, users
`alice@discurd.dev` / `bob@discurd.dev` / `charlie@discurd.dev`, password `password123`):

```sh
docker compose exec api /app/seed
```

State lives in named volumes (`scylla_data`, `redis_data`, `nats_data`, `minio_data`,
`prometheus_data`, `grafana_data`). Full reset: `docker compose down -v`.

---

## 2. Scale-out on one host

The overlay `docker-compose.scale.yml` turns the single Scylla node into a 3-node ring
and runs multiple api/gateway replicas:

```sh
docker compose -f docker-compose.yml -f docker-compose.scale.yml up -d
```

### What `deploy.replicas` does

For services carrying `deploy.replicas: N`, compose starts N containers of that service.
Traefik's Docker provider discovers each container individually; because all replicas
carry the same router/service labels, they become backends of one Traefik service and
requests are **round-robin load-balanced** across them. WebSockets are long-lived TCP
connections, so each socket naturally sticks to the replica that accepted it — no
session affinity configuration needed.

### Why api and gateway scale statelessly

- **api**: JWTs are verified locally with `JWT_SECRET`; refresh tokens, presence and
  rate-limit counters live in Redis; all durable data is in Scylla; uploads go to MinIO.
  No replica holds anything another replica needs.
- **gateway**: every instance subscribes to `discurd.events.>` on NATS, so **every
  event reaches every gateway replica**; each replica dispatches only to its own
  connected sessions (filtered by the session's cached guild-membership set). A user can
  therefore connect to *any* gateway replica and still receive all their events.
  Presence counters (`presence_conns:{user_id}`) are shared in Redis, so online/offline
  transitions are correct even with a user's sockets spread across replicas.

### Move the keyspace to real replication

`db/schema.cql` creates the keyspace with `SimpleStrategy` RF=1 for dev. With three
nodes up, switch to `NetworkTopologyStrategy` RF=3, then repair so pre-existing data is
copied to its new replicas.

Wait until all three nodes show `UN` (Up/Normal):

```sh
docker compose exec scylla nodetool status
```

Check the datacenter name (the Scylla image defaults to `datacenter1`), then alter:

```sh
docker compose exec scylla cqlsh -e "SELECT data_center FROM system.local;"

docker compose exec scylla cqlsh -e \
  "ALTER KEYSPACE discurd WITH replication = {'class': 'NetworkTopologyStrategy', 'datacenter1': 3};"
```

Run a primary-range repair on **every** Scylla node. The overlay's nodes are named
`scylla`, `scylla-node2`, `scylla-node3`, so both compose files must be passed for
`ps --services` to enumerate them, and the pattern must match the `-node` suffix (the
loop still skips `scylla-init`, which has no `nodetool`):

```sh
for s in $(docker compose -f docker-compose.yml -f docker-compose.scale.yml ps --services | grep -E '^scylla(-node[0-9]+)?$'); do
  docker compose -f docker-compose.yml -f docker-compose.scale.yml exec "$s" nodetool repair -pr discurd
done
```

After repair, reads/writes are served by a real 3-replica cluster and any single Scylla
node can be lost without data loss.

---

## 3. Production checklist

### 3.1 Real secrets

Set strong values in `.env` (compose reads it automatically) — never ship the defaults:

```sh
JWT_SECRET=$(openssl rand -hex 32)          # signs all access tokens
MINIO_ROOT_USER=discurd-objects
MINIO_ROOT_PASSWORD=$(openssl rand -hex 24)
GRAFANA_ADMIN_USER=ops
GRAFANA_ADMIN_PASSWORD=$(openssl rand -hex 16)
```

Rotating `JWT_SECRET` invalidates all outstanding access tokens (users re-auth via
their refresh tokens after at most `ACCESS_TOKEN_TTL`, default 15m). Also disable the
Traefik dashboard's insecure mode (`--api.insecure=true`) or firewall port 8090, and
don't publish Redis/NATS/Scylla/Grafana/Prometheus ports beyond what you need.

### 3.2 TLS via Traefik ACME (Let's Encrypt)

Add a `websecure` entrypoint and an ACME resolver to the `traefik` service `command`,
publish 443, and persist the cert store:

```yaml
traefik:
  command:
    # ... existing flags ...
    - --entrypoints.websecure.address=:443
    - --entrypoints.web.http.redirections.entrypoint.to=websecure
    - --entrypoints.web.http.redirections.entrypoint.scheme=https
    - --certificatesresolvers.le.acme.email=ops@example.com
    - --certificatesresolvers.le.acme.storage=/letsencrypt/acme.json
    - --certificatesresolvers.le.acme.httpchallenge.entrypoint=web
  ports:
    - "80:80"
    - "443:443"
  volumes:
    - traefik_letsencrypt:/letsencrypt
    - /var/run/docker.sock:/var/run/docker.sock:ro
```

Then, per routed service, pin the host and enable TLS — e.g. on `api`:

```yaml
labels:
  - traefik.http.routers.api.rule=Host(`chat.example.com`) && PathPrefix(`/api`)
  - traefik.http.routers.api.entrypoints=websecure
  - traefik.http.routers.api.tls.certresolver=le
```

Repeat for the `gateway` (`/ws`), `files` (`/files`) and `web` (`/`) routers. The web
app uses relative URLs and `wss://` automatically on HTTPS pages, so no app config
changes are needed.

### 3.3 ScyllaDB: turn off developer mode & size it

The dev stack runs Scylla with `--developer-mode=1 --smp 1 --memory 750M
--overprovisioned 1`. For production, remove all four flags. Guidance:

- Dedicate the host (or pinned cores) to Scylla; let it size itself, or set `--smp` to
  the core count and `--memory` to ~80% of RAM you can actually give it.
- Fast local NVMe with XFS for `/var/lib/scylla`; never network storage for the hot path.
- One Scylla node per physical host; keep RF=3 (`NetworkTopologyStrategy`, §2 above).
- Keep an eye on partition sizes — the message schema's 10-day buckets bound them by
  design, but very hot channels may warrant a smaller bucket constant before launch
  (changing it later requires a data migration).

### 3.4 Backups

**Scylla** — snapshot, then copy off-host:

```sh
docker compose exec scylla nodetool snapshot discurd
# snapshots land under /var/lib/scylla/data/discurd/*/snapshots/<tag>/ inside the
# scylla_data volume — archive them off the host, then clear:
docker compose exec scylla nodetool clearsnapshot discurd
```

Repeat per node. For scheduled, incremental, S3-targeted backups use
[Scylla Manager](https://manager.docs.scylladb.com/).

**MinIO** — mirror both buckets to another S3 target:

```sh
docker compose exec minio sh -c '
  mc alias set local  http://localhost:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" &&
  mc alias set backup https://s3.example.com BACKUP_KEY BACKUP_SECRET &&
  mc mirror --overwrite local/avatars     backup/discurd-avatars &&
  mc mirror --overwrite local/attachments backup/discurd-attachments'
```

Redis holds only ephemeral state (sessions, presence, rate limits) — losing it logs
users out, nothing more. NATS is fire-and-forget fan-out; no backup needed.

### 3.5 Pin image digests

Tags like `minio/minio:latest` move. In production, pin every image to a digest:

```sh
docker compose pull
docker inspect --format='{{index .RepoDigests 0}}' minio/minio:latest
# → minio/minio@sha256:<digest>  — put that in docker-compose.yml:
#   image: minio/minio@sha256:<digest>
```

Do the same for your own `api`/`gateway`/`web` images once you push them to a registry
(build with a version tag, deploy by digest).

### 3.6 Log aggregation

All services log JSON to stdout (`log/slog`). Ship container logs to
[Loki](https://grafana.com/oss/loki/): run a `loki` + `promtail` (or `alloy`) pair in
the compose file — or use Docker's Loki logging driver — and add Loki as a second
Grafana datasource next to the provisioned Prometheus one. You then get logs and the
Discurd metrics dashboard in the same Grafana at :3000.

### 3.7 Alerting starter rules

Load a rule file into Prometheus (`rule_files:` in `deploy/prometheus/prometheus.yml`)
and point `alerting:` at an Alertmanager. Starters:

```yaml
groups:
  - name: discurd
    rules:
      # >5% of HTTP requests failing with 5xx across api+gateway for 5 minutes
      - alert: HighHTTP5xxRate
        expr: |
          sum(rate(discurd_http_requests_total{status=~"5.."}[5m]))
            / sum(rate(discurd_http_requests_total[5m])) > 0.05
        for: 5m
        labels: { severity: page }
        annotations:
          summary: "Discurd 5xx ratio above 5% for 5m"

      # A scrape target (api/gateway/exporter) is down — readiness is gone or the
      # process is dead. /readyz failing makes the service drop from rotation and
      # its scrape fail shortly after.
      - alert: TargetDown
        expr: up == 0
        for: 2m
        labels: { severity: page }
        annotations:
          summary: "{{ $labels.job }} target {{ $labels.instance }} is down"
```

To alert on `/readyz` *directly* (dependency degraded while the process still serves
metrics), add a [blackbox exporter](https://github.com/prometheus/blackbox_exporter)
probing `http://api:8080/readyz` and `http://gateway:8080/readyz`, then:

```yaml
      - alert: ReadyzFailing
        expr: probe_success{job="readyz"} == 0
        for: 2m
        labels: { severity: page }
```

---

## 4. Kubernetes

Production-shaped manifests live in [`deploy/k8s/`](../deploy/k8s/) —
apply order and infra wiring are in [`deploy/k8s/README.md`](../deploy/k8s/README.md).
Summary below.

### 4.1 Build & push images

The app images are the same Dockerfiles compose uses:

```sh
REGISTRY=registry.example.com/discurd   # your registry
TAG=v1.0.0

docker build -f backend/Dockerfile.api     -t $REGISTRY/discurd-api:$TAG     backend
docker build -f backend/Dockerfile.gateway -t $REGISTRY/discurd-gateway:$TAG backend
docker build -t $REGISTRY/discurd-web:$TAG web

docker push $REGISTRY/discurd-api:$TAG
docker push $REGISTRY/discurd-gateway:$TAG
docker push $REGISTRY/discurd-web:$TAG
```

The manifests use `REGISTRY/discurd-api:TAG`-style placeholders; set your real names in
the `images:` block of `deploy/k8s/kustomization.yaml`.

### 4.2 ScyllaDB via the Scylla Operator

Run Scylla on Kubernetes with the
[Scylla Operator](https://operator.docs.scylladb.com/) — install the operator (and its
cert-manager prerequisite) per its docs, then apply our example cluster CR:

```sh
kubectl apply -f deploy/k8s/scylla-cluster.example.yaml
```

That creates a 3-member cluster `discurd-scylla` in namespace `scylla`, reachable at
`discurd-scylla-client.scylla.svc.cluster.local:9042` (which is what
`deploy/k8s/configmap.yaml` sets as `SCYLLA_HOSTS`). Apply the schema, then move the
keyspace to RF=3 in the CR's datacenter (`dc1`):

```sh
kubectl -n scylla run schema-init --rm -i --image=scylladb/scylla:6.2 --restart=Never \
  --command -- cqlsh discurd-scylla-client -e "$(cat db/schema.cql)"

kubectl -n scylla run alter-ks --rm -i --image=scylladb/scylla:6.2 --restart=Never \
  --command -- cqlsh discurd-scylla-client -e \
  "ALTER KEYSPACE discurd WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3};"
```

Then repair (via [Scylla Manager](https://manager.docs.scylladb.com/), or
`kubectl -n scylla exec <pod> -c scylla -- nodetool repair -pr discurd` on each member).

### 4.3 NATS & Redis via Helm

```sh
helm repo add nats https://nats-io.github.io/k8s/helm/charts/
helm install nats nats/nats --namespace nats --create-namespace

helm repo add bitnami https://charts.bitnami.com/bitnami
helm install redis bitnami/redis --namespace redis --create-namespace \
  --set architecture=standalone --set auth.enabled=false
```

These land at `nats://nats.nats.svc.cluster.local:4222` and
`redis-master.redis.svc.cluster.local:6379` — exactly what the ConfigMap defaults to.
Note the config contract has `REDIS_ADDR` only (no password variable), so keep Redis
unauthenticated and locked down with a NetworkPolicy, or terminate auth in a proxy.

### 4.4 MinIO — or swap in real S3

For in-cluster MinIO, install the chart **into the `discurd` namespace with the service
named `minio`** (the Ingress routes `/files/*` to it):

```sh
helm repo add minio https://charts.min.io/
helm install minio minio/minio --namespace discurd --create-namespace \
  --set fullnameOverride=minio --set mode=standalone \
  --set rootUser=<user> --set rootPassword=<password> \
  --set "buckets[0].name=avatars,buckets[0].policy=download" \
  --set "buckets[1].name=attachments,buckets[1].policy=download"
```

To use real S3 instead (the MinIO Go client is S3-compatible), set in the
ConfigMap/Secret: `MINIO_ENDPOINT=s3.<region>.amazonaws.com`, `MINIO_USE_SSL=true`,
`MINIO_ROOT_USER=<access key id>`, `MINIO_ROOT_PASSWORD=<secret access key>`; create
the `avatars` and `attachments` buckets with public-read GET. Because stored URLs are
relative (`/files/{bucket}/{key}`), keep the `/files` route working by pointing the
files Ingress at an `ExternalName` Service for the S3 endpoint — or front the buckets
with a CDN and rewrite `/files/*` there.

### 4.5 Deploy the app

```sh
# 1. Edit deploy/k8s/kustomization.yaml (images:) and create a real secret
#    (start from deploy/k8s/secret.example.yaml).
kubectl apply -k deploy/k8s

# 2. Wait for rollout
kubectl -n discurd rollout status deploy/api
kubectl -n discurd rollout status deploy/gateway
kubectl -n discurd rollout status deploy/web

# 3. Seed demo data (optional)
kubectl -n discurd exec deploy/api -- /app/seed
```

The Ingress (class `nginx`) mirrors the Traefik routing: `/api` → api, `/ws` → gateway
(WebSocket-friendly timeouts), `/files` → MinIO with the prefix stripped, `/` → web.
HPAs scale api (3→12) and gateway (2→10) at 70% CPU — `metrics-server` required.
Prometheus in-cluster: the pods carry `prometheus.io/*` scrape annotations; wire your
Prometheus (e.g. kube-prometheus-stack) to honor them and reuse the dashboard from
`deploy/grafana/`.
