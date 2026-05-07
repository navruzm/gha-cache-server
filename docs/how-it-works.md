---
title: How it works
---

# How it works

The cache server reproduces the **cache-service v2** wire protocol so that an unmodified `actions/cache@v4` or `actions/cache@v5` can save and restore caches against it. Both action versions speak the same protocol — v5 just runs on Node 24 and requires runner ≥ 2.327.1.

`actions/cache@v3` (and earlier) used the legacy `_apis/artifactcache` v1 protocol, which GitHub sunset on 2025-02-01. We don't implement v1; upgrade to v4 or v5.

## Protocol surface

- **Twirp v2 control plane** at `/twirp/github.actions.results.api.v1.CacheService/{Create,Get,Finalize}CacheEntry` (JSON over POST). Tells the runner where to upload and where to download.
- **Azure-style block-blob upload** at `/devstoreaccount1/upload/{uploadId}`. The runner PUTs each chunk with `?blockid=<base64>`, then commits with `?comp=blocklist`.
- **Direct streaming download** at `/download/{cacheEntryId}`. Used when direct-download presigned URLs are disabled or the entry hasn't yet been merged.
- **Catch-all proxy** for any other paths the runner might hit — forwards to `DEFAULT_ACTIONS_RESULTS_URL`.

## Storage layout

Every upload gets a numeric folder. Each chunk lands at `<folder>/parts/<index>`. On the first download, the server **streams parts back to the runner *and* simultaneously writes them to a `<folder>/merged` blob** so subsequent downloads can use a single object (and a single presigned URL when supported).

A small state machine in `storage_locations` tracks `mergeStartedAt`/`mergedAt`/`partsDeletedAt`. Cleanup tasks reset stalled merges, drop orphaned locations, and prune entries older than `CACHE_CLEANUP_OLDER_THAN_DAYS`.

## Disk-pressure eviction

When the filesystem driver is in use, a `cleanup:disk-pressure` task runs every 5 minutes and consults `syscall.Statfs` for available bytes on the storage root. If free space is below `DISK_PRESSURE_MIN_FREE_BYTES`, the server evicts cache entries by **least-recently-used** (`COALESCE(lastDownloadedAt, updatedAt) ASC`) until free space climbs back above `DISK_PRESSURE_TARGET_FREE_BYTES`. Entries whose merge is in flight are skipped so an active download is never yanked mid-stream.

The Helm chart sets sensible defaults (`2Gi` / `4Gi`); override via `config.diskPressureMinFreeBytes` and `config.diskPressureTargetFreeBytes`. Set either to `0` (or unset env) to disable the task entirely.

S3 and GCS backends don't have a "disk full" concept — capacity there is bounded by your cloud quota, not by ephemeral disk — so the disk-pressure task no-ops on them.

## Authentication

Each runner request carries a JWT issued by `https://token.actions.githubusercontent.com`. The server fetches that issuer's JWKS, caches it for 10 minutes, and verifies signatures (RS256, ES256). Two custom claims are extracted:

- `ac` — JSON-encoded array of `{Scope, Permission}` pairs (Permission ≥ 2 = write).
- `repository_id` — namespace for the cache.
