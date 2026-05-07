---
title: File System
---

# File System

Driver value: `filesystem` (default).

## Variables

| Var | Default | Notes |
|---|---|---|
| `STORAGE_FILESYSTEM_PATH` | `.data/storage/filesystem` | Created if missing |

## When to use

- Single-host deployments.
- Small teams. The filesystem driver is the simplest path; combine it with the SQLite driver for zero-dependency operation.

For multi-replica deployments use S3 or GCS instead — the filesystem driver requires `ReadWriteMany` shared volumes which add operational cost.
