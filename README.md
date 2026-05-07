# GitHub Actions Cache Server

A drop-in replacement for the GitHub Actions cache, compatible with `actions/cache@v4`. Single static binary, ~12 MB. Three storage backends (filesystem / S3 / GCS), three database backends (SQLite / Postgres / MySQL).

> Go reimplementation of the protocol popularised by [falcondev-oss/github-actions-cache-server](https://github.com/falcondev-oss/github-actions-cache-server) (MIT). All code in this repo is original.

## Quick start

```yaml
services:
  cache-server:
    image: ghcr.io/navruzm/github-actions-cache-server-go:latest
    ports: ["3000:3000"]
    environment:
      API_BASE_URL: http://localhost:3000
      STORAGE_DRIVER: filesystem
      STORAGE_FILESYSTEM_PATH: /data/cache
      DB_DRIVER: sqlite
      DB_SQLITE_PATH: /data/cache-server.db
    volumes: [cache-data:/data]
volumes: { cache-data: }
```

### Pointing runners at the server

The runner overwrites `ACTIONS_RESULTS_URL` per-step from GitHub's signed job message, so a workflow-level `env:` doesn't stick. Use the Node.js preload in `runner/preload.js` plus three env vars on the runner host:

```
NODE_OPTIONS=--require=/opt/cache-redirect.js
OVERRIDE_ACTIONS_RESULTS_URL=http://localhost:3000/
ACTIONS_CACHE_SERVICE_V2=true
```

Setup recipes for systemd, Docker, and actions-runner-controller live in `runner/` and are documented in `docs/runner-setup.md`.

## Configuration

| Variable | Default | Notes |
|---|---|---|
| `API_BASE_URL` | _required_ | Public base URL |
| `STORAGE_DRIVER` | `filesystem` | `filesystem` \| `s3` \| `gcs` |
| `STORAGE_FILESYSTEM_PATH` | `.data/storage/filesystem` | filesystem driver |
| `STORAGE_S3_BUCKET` | | required for s3 |
| `AWS_REGION` | `us-east-1` | s3 |
| `AWS_ENDPOINT_URL` | | s3 (MinIO etc.) |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | | s3 |
| `STORAGE_GCS_BUCKET` | | required for gcs |
| `STORAGE_GCS_SERVICE_ACCOUNT_KEY` | | path to SA key JSON |
| `STORAGE_GCS_ENDPOINT` | | gcs (fake-gcs-server etc.) |
| `DB_DRIVER` | `sqlite` | `sqlite` \| `postgres` \| `mysql` |
| `DB_SQLITE_PATH` | `.data/sqlite.db` | sqlite |
| `DB_POSTGRES_URL` | | postgres |
| `DB_POSTGRES_HOST/PORT/DATABASE/USER/PASSWORD` | | postgres |
| `DB_MYSQL_HOST/PORT/DATABASE/USER/PASSWORD` | | mysql |
| `CACHE_CLEANUP_OLDER_THAN_DAYS` | `90` | |
| `DISABLE_CLEANUP_JOBS` | `false` | |
| `ENABLE_DIRECT_DOWNLOADS` | `false` | use presigned URLs |
| `SKIP_TOKEN_VALIDATION` | `false` | dev only |
| `MANAGEMENT_API_KEY` | | enables `/management-api/*` |

## Architecture

- `cmd/cache-server` — entrypoint
- `internal/server` — HTTP handlers
- `internal/storage` — adapter interface + filesystem/S3/GCS
- `internal/db` — `database/sql` queries + migrations
- `internal/auth` — JWKS + JWT verification (RS256/ES256)
- `internal/cron` — interval scheduler
- `internal/tasks` — five cleanup tasks

## License

MIT
