---
title: Getting Started
outline: [2, 3]
---

# Getting Started

The cache server is shipped as a Docker image and a Helm chart. The simplest deployment uses filesystem storage and SQLite, all on a single volume.

## Docker Compose

```yaml
services:
  cache-server:
    image: ghcr.io/navruzm/github-actions-cache-server-go:latest
    ports:
      - '3000:3000'
    environment:
      API_BASE_URL: http://localhost:3000
      STORAGE_DRIVER: filesystem
      STORAGE_FILESYSTEM_PATH: /data/cache
      DB_DRIVER: sqlite
      DB_SQLITE_PATH: /data/cache-server.db
    volumes:
      - cache-data:/data

volumes:
  cache-data:
```

## Kubernetes via Helm

Requires Helm 3.8+ for OCI registries.

```bash
helm install cache-server \
  oci://ghcr.io/navruzm/charts/github-actions-cache-server
```

Customise via `values.yaml` (see [Helm Chart](/helm) for the full surface).

## Self-hosted runner setup

The runner must be told to talk to your cache server instead of GitHub's. Set `ACTIONS_RESULTS_URL` to the cache server's API URL, **with a trailing slash**.

The official GitHub runner overwrites `ACTIONS_RESULTS_URL` with GitHub's endpoint at boot. Two options to defeat that: use a runner image patched to accept a custom URL, or apply a binary patch to `Runner.Worker.dll` to rename the env var the runner overwrites.

## Required environment variables

| Variable | Default | Notes |
|---|---|---|
| `API_BASE_URL` | _required_ | Public base URL the runner can reach |
| `STORAGE_DRIVER` | `filesystem` | `filesystem` \| `s3` \| `gcs` |
| `DB_DRIVER` | `sqlite` | `sqlite` \| `postgres` \| `mysql` |
| `PORT` | `3000` | Listen port |
| `CACHE_CLEANUP_OLDER_THAN_DAYS` | `90` | 0 disables age-based cleanup |
| `DISK_PRESSURE_MIN_FREE_BYTES` | unset | Filesystem only. Start LRU eviction when free space drops below this (e.g. `2Gi`) |
| `DISK_PRESSURE_TARGET_FREE_BYTES` | unset | Stop evicting once free space reaches this (e.g. `4Gi`). Hysteresis prevents flapping. |
| `ENABLE_DIRECT_DOWNLOADS` | `false` | Hand the runner a presigned URL |
| `MANAGEMENT_API_KEY` | _unset_ | Enables `/management-api` when set |
| `SKIP_TOKEN_VALIDATION` | `false` | Dev only — disables JWT verification |
| `DEBUG` | `false` | Verbose logging |

Per-driver variables are documented under [Storage Drivers](/storage-drivers/file-system) and [Database Drivers](/database-drivers/sqlite).
