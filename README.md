# listarr-go

Go rewrite of the [Listarr](https://github.com/fisherd80/listarr) idea: discover
media lists and **preview / apply** imports into Radarr, Sonarr, and Seerr —
with TorBox-friendly **search-on-add rate limiting**.

> Inspired by [fisherd80/Listarr](https://github.com/fisherd80/listarr) (MIT).
> This is a clean-room Go implementation, not a fork of that repository.
> See [NOTICE](NOTICE).

**Status:** v0.2 — TMDB discover + Radarr/Sonarr preview/apply (Seerr/IMDB next).

## Public-repo security

This repository is **public**. Do not commit API keys, tokens, cookies, or
hostnames that identify a private network. Defaults bind to loopback and keep
**apply disabled** until you opt in.

| Rule | Detail |
|------|--------|
| Secrets | Env vars or a local ignore-listed config file only |
| Apply | Mutations require `LISTARR_APPLY=1` |
| Listen | Default `127.0.0.1:8787` (not all interfaces) |
| Examples | Use `127.0.0.1` placeholders only |

See [SECURITY.md](SECURITY.md).

## Quick start (local)

```bash
export LISTARR_API_KEY='replace-with-a-long-random-string'
# optional integrations (local placeholders only):
# export LISTARR_TMDB_API_KEY='...'
# export LISTARR_RADARR_URL='http://127.0.0.1:7878'
# export LISTARR_RADARR_API_KEY='...'
# export LISTARR_APPLY=1
go run ./cmd/listarr-go
```

```bash
curl -s http://127.0.0.1:8787/health
curl -s -H "X-Api-Key: $LISTARR_API_KEY" http://127.0.0.1:8787/api/v1/system/status
```

Preview (no writes) — paths/IDs are caller-supplied, never baked into the binary:

```bash
curl -s -X POST http://127.0.0.1:8787/api/v1/sync/preview \
  -H "X-Api-Key: $LISTARR_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "source":"tmdb",
    "mediaType":"movie",
    "discover":{"page":1,"sortBy":"popularity.desc"},
    "target":{
      "rootFolderPath":"/data/movies",
      "qualityProfileId":1,
      "monitored":true,
      "searchOnAdd":true
    }
  }'
```

Contract smoke:

```bash
./scripts/run-contract-tests.py \
  --base-url http://127.0.0.1:8787 \
  --api-key "$LISTARR_API_KEY"
```

## Configuration (env)

| Variable | Default | Notes |
|----------|---------|-------|
| `LISTARR_API_KEY` | _(required)_ | `X-Api-Key` / `apikey=` |
| `LISTARR_LISTEN` | `127.0.0.1:8787` | Prefer loopback unless you terminate TLS elsewhere |
| `LISTARR_INSTANCE_NAME` | `listarr` | Status label only |
| `LISTARR_APPLY` | _(unset = off)_ | Set to `1` to allow mutating apply routes |
| `LISTARR_TORBOX_SEARCH_PER_HOUR` | `60` | Search-on-add budget (rolling hour) |
| `LISTARR_TMDB_API_KEY` | _(none)_ | Enables `/api/v1/discover/*` and discover-based sync |
| `LISTARR_RADARR_URL` / `LISTARR_RADARR_API_KEY` | _(none)_ | Movie sync target |
| `LISTARR_SONARR_URL` / `LISTARR_SONARR_API_KEY` | _(none)_ | TV sync target |

Copy [`.env.example`](.env.example) to `.env` (gitignored) for local use.

## Goals

Documented in [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md).

## License

MIT — see [LICENSE](LICENSE). Upstream Listarr is also MIT; credit in [NOTICE](NOTICE).
