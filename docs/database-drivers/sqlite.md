---
title: SQLite
---

# SQLite

Driver value: `sqlite` (default).

## Variables

| Var | Default | Notes |
|---|---|---|
| `DB_SQLITE_PATH` | `.data/sqlite.db` | Created if missing |

## Caveats

- **Single-replica only.** SQLite cannot support concurrent writers from multiple processes. The Helm chart refuses to render a Deployment when `replicaCount > 1` with the SQLite driver.
- WAL mode is enabled automatically for better write throughput.
