# listarr-go

Go rewrite of the [Listarr](https://github.com/fisherd80/listarr) idea: discover
and **preview / apply** media into Radarr/Sonarr — including **multi-instance
library sync** (local ↔ external) — with TorBox-friendly search-on-add limits.

> Inspired by [fisherd80/Listarr](https://github.com/fisherd80/listarr) (MIT).
> Clean-room Go implementation; see [NOTICE](NOTICE).

**Status:** v0.5 — embedded operator web UI for sync/discover/activity testing.

## Quick start (Docker)

On your Docker host as your login user:

```bash
cp .env.docker.example .env
# edit LISTARR_API_KEY to a long random string
docker compose up --build
```

Then open the UI:

```bash
# browser
open http://127.0.0.1:8787/
# paste LISTARR_API_KEY → Connect
```

Or curl:

```bash
curl -s http://127.0.0.1:8787/health
curl -s -H "X-Api-Key: $LISTARR_API_KEY" http://127.0.0.1:8787/api/v1/system/status
curl -s -H "X-Api-Key: $LISTARR_API_KEY" http://127.0.0.1:8787/api/v1/activity
```

Smoke script (ephemeral key if unset): `./scripts/docker-smoke.sh`

- Published only on **`127.0.0.1:8787`** (not the LAN).
- Inside the container listen is `0.0.0.0:8787` so the publish works; the host binary default stays loopback.
- Optional Postgres: set `LISTARR_STORE_BACKEND=postgres` and
  `LISTARR_DATABASE_URL=postgres://listarr:listarr@postgres:5432/listarr?sslmode=disable`,
  then `docker compose --profile postgres up --build`.
- Reach *arr on the host via `http://host.docker.internal:<port>` (see `.env.docker.example`).
  - **Docker Desktop:** bridge networking + `extra_hosts: host.docker.internal:host-gateway` is enough.
  - **Linux rootless:** use host networking override:
    `docker compose -f compose.yaml -f compose.host.yaml up -d`
    and ensure `host.docker.internal` resolves on the host (often `127.0.0.1` in `/etc/hosts`),
    with *arr reachable on those host ports (or a local proxy).

## Public-repo security

| Rule | Detail |
|------|--------|
| Secrets | Env seeds store; bootstrap (`LISTEN` / store backend) stays env; Settings UI may show secrets to API-key holders |
| Apply | Opt-in via Settings (seeded from `LISTARR_APPLY=1`) |
| Listen | Default `127.0.0.1:8787` |
| Examples | `127.0.0.1` placeholders only — never private MagicDNS |

See [SECURITY.md](SECURITY.md).

## Store backends

Sync activity and **operator settings** persist through a pluggable store:

| Backend | Role |
|---------|------|
| `polars` (default) | `sync_runs.csv` + `settings.json` under `LISTARR_POLARS_DIR` |
| `postgres` | Production OLTP (`LISTARR_DATABASE_URL` / `DATABASE_URL`) |
| `sqlite` / `mysql` | Stubbed — clear error until implemented |

On first boot with an empty store, settings are **seeded from `.env`**. Afterwards the datastore is SoT — edit via the Settings tab or `GET`/`PUT /api/v1/settings`. Listen address and store backend/DSN remain env-only (require restart).

```bash
# Dev / CI (default)
export LISTARR_STORE_BACKEND=polars
# export LISTARR_POLARS_DIR=data/polars

# Production
# export LISTARR_STORE_BACKEND=postgres
# export LISTARR_DATABASE_URL='postgres://listarr:…@127.0.0.1:5432/listarr?sslmode=disable'
```

Recent runs: `GET /api/v1/activity`. Settings: `GET /api/v1/settings`. Polars CSV smoke: `uv run --with polars python scripts/test_polars_store.py data/polars/sync_runs.csv`.

## Dual *arr sync (first use case)

Seed two Radarrs via env (first boot), or add them in the Settings UI:

```bash
export LISTARR_API_KEY='replace-with-a-long-random-string'
export LISTARR_ARR_LOCAL_URL='http://127.0.0.1:7878'
export LISTARR_ARR_LOCAL_API_KEY='…'
export LISTARR_ARR_LOCAL_KIND=radarr
export LISTARR_ARR_REMOTE_URL='http://127.0.0.1:7879'
export LISTARR_ARR_REMOTE_API_KEY='…'
export LISTARR_ARR_REMOTE_KIND=radarr
# export LISTARR_APPLY=1   # only when ready to mutate
go run ./cmd/listarr-go
```

Preview titles that exist on `local` (monitored + tag) but not yet on `remote`:

```bash
curl -s -X POST http://127.0.0.1:8787/api/v1/sync/preview \
  -H "X-Api-Key: $LISTARR_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "source":"arr-library",
    "mediaType":"movie",
    "sourceInstance":"local",
    "sourceFilter":{"monitoredOnly":true,"tagIds":[1]},
    "maxItems":500,
    "target":{
      "instance":"remote",
      "rootFolderPath":"/data/movies",
      "qualityProfileId":1,
      "monitored":true,
      "searchOnAdd":true
    }
  }'
```

List configured instance **names** (no URLs leaked):

```bash
curl -s -H "X-Api-Key: $LISTARR_API_KEY" http://127.0.0.1:8787/api/v1/arr/instances
```

Import List **configs** on an instance (metadata; title fetch comes next):

```bash
curl -s -H "X-Api-Key: $LISTARR_API_KEY" \
  http://127.0.0.1:8787/api/v1/arr/local/importlists
```

Until list-item fetch lands, sync “a list” by filtering the library on the tags
that Import List stamped (`sourceFilter.tagIds`).

## Configuration

**Env-only (bootstrap):** `LISTARR_LISTEN`, `LISTARR_STORE_BACKEND`, `LISTARR_DATABASE_URL` / `DATABASE_URL`, `LISTARR_POLARS_DIR`.

**Seeded into the store on first boot** (then edited via Settings / API):

| Variable | Default | Notes |
|----------|---------|-------|
| `LISTARR_API_KEY` | required on empty store | Auth |
| `LISTARR_INSTANCE_NAME` | `listarr` | Display name |
| `LISTARR_APPLY` | off | Set `1` to seed apply enabled |
| `LISTARR_TORBOX_SEARCH_PER_HOUR` | `60` | Search budget |
| `LISTARR_TMDB_API_KEY` | none | Discover |
| `LISTARR_ARR_<NAME>_URL` | none | Named instance |
| `LISTARR_ARR_<NAME>_API_KEY` | none | Named instance |
| `LISTARR_ARR_<NAME>_KIND` | none | `radarr` or `sonarr` |
| `LISTARR_RADARR_*` / `LISTARR_SONARR_*` | none | Legacy aliases → names `radarr` / `sonarr` |

| Always env | Default | Notes |
|------------|---------|-------|
| `LISTARR_LISTEN` | `127.0.0.1:8787` | Loopback |
| `LISTARR_STORE_BACKEND` | `polars` | `postgres` \| `polars` \| `sqlite` \| `mysql` |
| `LISTARR_DATABASE_URL` / `DATABASE_URL` | none | Required for `postgres` |
| `LISTARR_POLARS_DIR` | `data/polars` | CSV + `settings.json` directory |

## Goals

See [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md).

## License

MIT — [LICENSE](LICENSE). Upstream Listarr credit — [NOTICE](NOTICE).
