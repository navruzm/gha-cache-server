---
title: Management API
---

# Management API

When `MANAGEMENT_API_KEY` is set, the server exposes a REST surface for inspecting and pruning the cache. Without the key, all `/management-api/*` routes return `503`.

## Auth

Every request must carry the API key in `X-Api-Key`.

## Interactive docs

Swagger UI: `https://<your-cache-server>/management-api/_docs`
OpenAPI JSON: `https://<your-cache-server>/management-api/_docs/spec.json`

## Endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/management-api/cache-entries` | List with filters and pagination |
| GET | `/management-api/cache-entries/{id}` | Fetch one |
| GET | `/management-api/cache-entries/match` | Replay match logic for a primary/restore-key combo |
| DELETE | `/management-api/cache-entries/{id}` | Delete one |
| DELETE | `/management-api/cache-entries` | Bulk delete by filters |
| GET | `/management-api/storage-locations/{id}` | Fetch storage location |
| DELETE | `/management-api/storage-locations/{id}` | Delete location and underlying files |

Filter parameters: `key`, `version`, `scope`, `repoId`, plus `page` and `itemsPerPage` (1-100).
