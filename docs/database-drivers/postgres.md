---
title: PostgreSQL
---

# PostgreSQL

Driver value: `postgres`.

## Variables

Use either `DB_POSTGRES_URL` or the individual fields below.

| Var | Required | Notes |
|---|---|---|
| `DB_POSTGRES_URL` | one-of | Connection string (`postgres://user:pass@host:port/db`) |
| `DB_POSTGRES_HOST` | one-of | |
| `DB_POSTGRES_PORT` | one-of | |
| `DB_POSTGRES_DATABASE` | one-of | |
| `DB_POSTGRES_USER` | one-of | |
| `DB_POSTGRES_PASSWORD` | one-of | |

## Notes

- Migrations run on startup. Idempotent.
- Pool size: 10 connections.
