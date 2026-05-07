---
title: Helm Chart
---

# Helm Chart

The chart lives at `install/kubernetes/gha-cache-server` in the repo and is published to `oci://ghcr.io/navruzm/charts/gha-cache-server`.

## Install

```bash
helm install cache-server \
  oci://ghcr.io/navruzm/charts/gha-cache-server \
  -f my-values.yaml
```

## Common configurations

### Filesystem + SQLite (single-node)

```yaml
replicaCount: 1
config:
  storage:
    driver: filesystem
    filesystem:
      path: /data/cache
  db:
    driver: sqlite
    sqlite:
      path: /data/cache-server.db
persistentVolumeClaim:
  storage: 50Gi
```

### S3 + Postgres (HA)

```yaml
replicaCount: 3
autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10

existingSecret: cache-server-secrets

config:
  enableDirectDownloads: true
  storage:
    driver: s3
    s3:
      bucket: gh-actions-cache
      region: eu-west-1
  db:
    driver: postgres
    postgres:
      host: pg.internal
      port: 5432
      database: cache
      user: cache
```

The `cache-server-secrets` secret should contain `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and `DB_POSTGRES_PASSWORD`.

## Behaviour notes

- **PVC auto-enables** when `storage.driver=filesystem` *or* `db.driver=sqlite`.
- **`accessModes` auto-switches** to `ReadWriteMany` if you scale the filesystem driver beyond one replica.
- **SQLite + multi-replica is rejected** at install time — switch to Postgres or MySQL.
