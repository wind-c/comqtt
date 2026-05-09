# comqtt

A Helm chart for [comqtt](https://github.com/wind-c/comqtt), a lightweight,
high-performance MQTT broker (v3.0/v3.1.1/v5.0) supporting standalone and
clustered deployments via Raft + Gossip.

Resolves [issue #137](https://github.com/wind-c/comqtt/issues/137).

## TL;DR

```bash
helm repo add comqtt https://wind-c.github.io/comqtt
helm install my-broker comqtt/comqtt
```

## Single-node install

```bash
helm install single deploy/helm/comqtt
helm test single --logs
```

## Clustered install

A 3-replica Raft cluster needs a RESP-compatible store for shared session
state. The chart **does not bundle** Redis or Valkey because the public
Bitnami images are gated as of 2025; bring your own. The simplest option is
the [Valkey](https://valkey.io) OSS fork of Redis:

```bash
kubectl apply -f deploy/helm/comqtt/ci/valkey.yaml

helm install cluster deploy/helm/comqtt \
  --set mode=cluster \
  --set replicaCount=3 \
  --set config.redis.options.addr=valkey:6379
```

`replicaCount` must be **odd** (1, 3, 5, 7) for Raft quorum — the schema enforces this.

### Required external services

| comqtt feature | Required when | Address value | Example |
|---|---|---|---|
| RESP store (Redis or Valkey) | `mode=cluster` or `config.storage-way=3` | `config.redis.options.addr` | [ci/valkey.yaml](ci/valkey.yaml) |
| MySQL | `config.auth.datasource=2` | via `config.auth.conf-path` | upstream auth-mysql.yml |
| PostgreSQL | `config.auth.datasource=3` | via `config.auth.conf-path` | upstream auth-postgresql.yml |
| HTTP auth | `config.auth.datasource=4` | via `config.auth.conf-path` | upstream auth-http.yml |


## Connecting

Inside the cluster:

```text
MQTT  tcp://<release>-comqtt.<namespace>.svc.cluster.local:1883
WS    ws://<release>-comqtt.<namespace>.svc.cluster.local:1882/mqtt
HTTP  http://<release>-comqtt.<namespace>.svc.cluster.local:8080
```

Smoke test using `eclipse-mosquitto`:

```bash
kubectl run mqtt-pub --rm -i --restart=Never \
  --image=eclipse-mosquitto:2 -- \
  mosquitto_pub -h <release>-comqtt -t demo -m "hello"
```

## How cluster mode works

The chart deploys a StatefulSet plus a headless Service so each pod gets a
stable DNS name like `release-comqtt-0.release-comqtt-headless.namespace.svc.cluster.local`.

An entrypoint script (mounted from a ConfigMap) renders the broker config
at boot, performing two runtime steps:

1. **Seed members** are computed from `replicaCount` and the headless Service
   FQDN — horizontal scaling does not require a chart upgrade.
2. **Raft bootstrap** is set to `true` only when both conditions hold:
   - the pod hostname ends in `-0` (the genesis pod), and
   - the Raft data directory is empty.

   This is why a `helm rollout restart` is safe: existing pods see a populated
   data dir, the bootstrap flag stays off, and Raft re-joins. Re-bootstrapping
   a populated cluster is destructive and the chart prevents it.

The pod's IP is bound via `--bind-ip=$POD_IP` from the downward API.

A PodDisruptionBudget enforces `minAvailable = ⌈(replicas+1)/2⌉` so voluntary
disruptions cannot drop quorum. Soft pod anti-affinity is the default; flip
`cluster.hardAntiAffinity=true` to require distinct hostnames.

Each replica gets its own PVC via `volumeClaimTemplates` — Raft log
durability is non-negotiable in cluster mode and `persistence.enabled=false`
is rejected by the schema.

## Values reference

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `mode` | string | `single` | `single` or `cluster`. |
| `replicaCount` | int | `3` | Cluster only. Must be odd. |
| `image.repository` | string | `ghcr.io/wind-c/comqtt` | |
| `image.tag` | string | `""` | Falls back to `Chart.appVersion`. `latest` is rejected. |
| `image.pullPolicy` | string | `IfNotPresent` | |
| `image.pullSecrets` | list | `[]` | |
| `serviceAccount.create` | bool | `true` | |
| `podSecurityContext` | object | non-root, fsGroup 1000 | |
| `securityContext` | object | drop ALL caps, RO root fs | |
| `resources.requests` | object | `100m / 128Mi` | |
| `resources.limits` | object | `{}` | unset by default |
| `livenessProbe` | object | TCP 1883, period 20s | |
| `readinessProbe` | object | TCP 1883, period 10s | |
| `startupProbe` | object | TCP 1883, period 5s, 30 fails | |
| `service.mqtt.{type,port,nodePort}` | — | `ClusterIP / 1883` | |
| `service.ws.{type,port,nodePort}` | — | `ClusterIP / 1882` | |
| `service.dashboard.{type,port,nodePort}` | — | `ClusterIP / 8080` | |
| `ingress.enabled` | bool | `false` | **DEPRECATED.** Use `gateway.*`. HTTP listener only. |
| `gateway.enabled` | bool | `false` | Master toggle for Gateway API resources. |
| `gateway.parentRefs` | list | `[]` | Default `parentRefs` for both routes. |
| `gateway.api.enabled` | bool | `true` | Emit `HTTPRoute` for the broker's HTTP listener (REST + metrics). |
| `gateway.api.hostnames` | list | `[]` | HTTPRoute hostnames. |
| `gateway.api.matches` | list | PathPrefix `/` | HTTPRoute path matches. |
| `gateway.mqtt.enabled` | bool | `false` | Emit `TCPRoute` for raw MQTT (alpha API). |
| `tls.enabled` | bool | `false` | |
| `tls.existingSecret` | string | `""` | Pre-created Secret with tls.crt/tls.key/ca.crt. |
| `tls.certManager.enabled` | bool | `false` | |
| `config.*` | object | mirrors `cmd/config/single.yml` | Rendered into ConfigMap. |
| `cluster.gossipPort` | int | `7946` | |
| `cluster.raftPort` | int | `8946` | |
| `cluster.grpcPort` | int | `17946` | |
| `cluster.grpcEnable` | bool | `true` | |
| `cluster.discoveryWay` | int | `0` | 0 serf, 1 memberlist. |
| `cluster.raftImpl` | int | `0` | 0 hashicorp, 1 etcd. |
| `cluster.hardAntiAffinity` | bool | `false` | |
| `cluster.podDisruptionBudget.enabled` | bool | `true` | |
| `persistence.enabled` | bool | `true` | Required in cluster mode. |
| `persistence.size` | string | `5Gi` | |
| `persistence.storageClass` | string | `""` | |
| `secret.create` | bool | `false` | |
| `secret.existingSecret` | string | `""` | |
| `secret.data` | map | `{}` | Keys become file names under `/etc/comqtt/secrets`. |
| `serviceMonitor.enabled` | bool | `false` | kube-prometheus-stack. |
| `tests.enabled` | bool | `true` | `helm test` mosquitto pod. |

For the full list of `config.*` keys (storage, auth, mqtt, redis, log) see
[cmd/config/single.yml](https://github.com/wind-c/comqtt/blob/main/cmd/config/single.yml)
and [cmd/config/node1.yml](https://github.com/wind-c/comqtt/blob/main/cmd/config/node1.yml)
in the upstream repo. The chart's `config:` block mirrors those files
verbatim.

## Exposing the broker externally

The chart prefers [Gateway API](https://gateway-api.sigs.k8s.io/) over the
legacy `Ingress` resource. ingress-nginx is in [retirement as of
2025-11-11](https://kubernetes.io/blog/2025/11/11/ingress-nginx-retirement/)
and a Layer 7 Ingress cannot proxy raw MQTT in any case.

### Gateway API (preferred)

You bring the `Gateway` resource and a Gateway API provider. Examples:
[Envoy Gateway](https://gateway.envoyproxy.io/), Cilium, Istio, Kong.

The chart emits:

- `HTTPRoute` for the broker's HTTP listener on port 8080
  (`gateway.api.enabled`, default on). This serves the existing `/api/v1/*`
  REST surface and `/metrics`. Most users will not expose this externally;
  flip the toggle off if you don't need it.
- `TCPRoute` for raw MQTT (`gateway.mqtt.enabled`, default off — TCPRoute is
  in `gateway.networking.k8s.io/v1alpha2` and requires a TCP-aware provider)

Minimal example with Envoy Gateway in front:

```yaml
# values.yaml
gateway:
  enabled: true
  parentRefs:
    - name: eg
      namespace: envoy-gateway-system
  api:
    enabled: true
    hostnames: ["comqtt.example.com"]
  mqtt:
    enabled: true
```

For TLS termination at the Gateway, configure listeners on the `Gateway`
itself; for TLS at the broker, set `tls.existingSecret` and wire
`config.mqtt.tls.{ca-cert,server-cert,server-key}`.

### Legacy fallbacks

If Gateway API is not available in your cluster:

- `service.mqtt.type: LoadBalancer` — straightforward on cloud providers.
- A NodePort plus an external load balancer or DNS round-robin.
- `ingress.*` (deprecated; HTTP listener only, cannot proxy raw MQTT).

## Upgrades

- **Switching `mode=single` ↔ `mode=cluster` is not supported in-place.**
  The on-disk data layouts differ; the chart will refuse to roll a Deployment
  into a StatefulSet (and Helm itself errors on incompatible kinds). Migrate
  by exporting state, uninstalling, and reinstalling.

- **Scaling cluster replicas:** scale up by `helm upgrade --set replicaCount=N`
  (must remain odd). The new pods receive a populated data dir on first boot
  only after they join the existing cluster — the bootstrap shim handles this
  automatically.

- **Scaling cluster replicas down** is risky: pods removed without explicit
  Raft eviction become permanent failures from the cluster's perspective. See
  the limitations section.

## Limitations

- No operator. The chart does not perform Raft member eviction when a pod is
  permanently removed — the operator is expected to scale via `helm upgrade`
  and accept that decommissioned members remain in the Raft membership list
  until manually evicted.
- No automatic recovery from split-brain. If two halves of the cluster
  diverge, manually pick a survivor, scale the StatefulSet to 1, delete the
  PVCs of the losers, then scale back up.
- No bundled RESP store. The chart expects an externally-deployed Redis or
  Valkey (see [ci/valkey.yaml](ci/valkey.yaml) for a starting point) — the
  chart does not manage failover or HA for it.
- The HTTP `HTTPRoute` defaults to `PathPrefix /`. Override
  `gateway.api.matches` to mount under a sub-path; consumers expecting
  routes at `/api/v1/...` and `/metrics` will need an HTTPRoute filter
  (`URLRewrite`) to strip the prefix before forwarding.

## Contributing

Bug reports and PRs welcome. Please run before submitting:

```bash
helm lint deploy/helm/comqtt
helm template ci deploy/helm/comqtt -f deploy/helm/comqtt/ci/single-values.yaml
helm template ci deploy/helm/comqtt -f deploy/helm/comqtt/ci/cluster-values.yaml
```
