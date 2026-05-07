---
title: Google Cloud Storage
---

# Google Cloud Storage

Driver value: `gcs`.

## Variables

| Var | Default | Notes |
|---|---|---|
| `STORAGE_GCS_BUCKET` | _required_ | |
| `STORAGE_GCS_SERVICE_ACCOUNT_KEY` | _empty_ | Path to JSON key file. Omit on GKE to use Workload Identity. |
| `STORAGE_GCS_ENDPOINT` | _empty_ | Set when targeting fake-gcs-server in tests |

## Example

```yaml
services:
  cache-server:
    image: ghcr.io/navruzm/gha-cache-server:latest
    ports: ['3000:3000']
    environment:
      API_BASE_URL: http://localhost:3000
      STORAGE_DRIVER: gcs
      STORAGE_GCS_BUCKET: gh-actions-cache
      STORAGE_GCS_SERVICE_ACCOUNT_KEY: /gcp/sa.json
    volumes:
      - ./sa.json:/gcp/sa.json:ro
```

## Direct downloads

Not yet wired up for GCS — `ENABLE_DIRECT_DOWNLOADS=true` falls back to streaming through the cache server.
