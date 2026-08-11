# listarr-go requirements (product SoT)

Locked operator intent (2026-08-11, extended for multi-*arr sync).

Homelab-specific hostnames and credentials live in private ops docs — **not** here.

## Goal

A standalone Go preview/apply tool that:

1. Discovers titles (TMDB today; IMDB/Seerr next)
2. **Imports from Radarr/Sonarr libraries** (and, next, any *arr Import List–shaped sources)
3. Applies into one or more Radarr/Sonarr targets — including **keeping a local and an external *arr pair in sync**
4. Rate-limits TorBox-oriented `searchOnAdd` (default **60/hour**)
5. Persists sync activity via a pluggable store (**Postgres** for production, **Polars/CSV** for tests; SQLite/MySQL stubbed for community)

Forever a **tool**, never a forced replacement of *arr native Import Lists.

## First use case (priority)

Synchronize two Radarr instances and two Sonarr instances (local ↔ external):

```text
source instance library  ──preview/apply──►  target instance
   (filters: monitored / tags / path)         (root / QP / tags / searchOnAdd)
```

Operators configure named instances via env (`LISTARR_ARR_<NAME>_…`). Requests
reference instance **names** only — URLs/keys never appear in sync JSON.

## 2nd / 3rd order thinking

| Order | Intent | Status |
|-------|--------|--------|
| 1st | Dual Radarr + dual Sonarr library sync | **v0.3** (`source=arr-library`) |
| 1st | Persist sync activity (postgres + polars) | **v0.4** |
| 2nd | Treat *arr **Import Lists** as named sources (tag filters today; list-item fetch next) | Metadata API now; item fetch follows |
| 3rd | Any *arr family (Lidarr, …) behind the same registry + sync contracts | Kind enum ready; clients later |
| 3rd | Seerr as both catalog IO and request/pipeline control plane | Phase 2–3 |
| 3rd | Portable export/import JSON between instances / backups | Follow-on |

Tag-filtered `arr-library` sync is the practical stand-in for “sync this Import List’s
titles” when lists stamp tags on add.

## Locked decisions

| Decision | Choice |
|----------|--------|
| Feature SoT | [fisherd80/Listarr](https://github.com/fisherd80/listarr) + multi-instance sync |
| Language | Go |
| Trakt | Out of scope for now |
| Stance | Preview/apply tool forever |
| TorBox search-on-add | On, default **60 / rolling hour** |
| Instances | Named registry; no private URL defaults |
| Secrets | Env only; status endpoints never echo URLs/keys |
| Persistence | Multi-backend store: `postgres` + `polars` first; `sqlite`/`mysql` stubs |

## Phases

| Phase | Scope |
|-------|--------|
| 0–1a | Health/status, apply gate, rate limiter |
| 1b | TMDB discover + single-target *arr apply |
| 1c | Named *arr registry + arr-library dual-instance sync |
| **1e** | **Embedded operator web UI** (`/`) — **current** |
| 2 | IMDB + Seerr import/export; Import List item fetch |
| 3 | Seerr add/delete + pipeline trigger |
| 4 | UI parity with upstream Listarr + optional MCP |

## Success metrics

1. Preview never mutates; apply is opt-in and idempotent across two Radarrs/Sonarrs.
2. Search-triggering adds stay within the hourly budget.
3. No secrets or private inventory in defaults/docs examples.
4. Agents can drive local→remote sync over HTTP without a browser.
