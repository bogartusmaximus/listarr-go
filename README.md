# listarr-go

Go rewrite of the [Listarr](https://github.com/fisherd80/listarr) idea: discover
media lists and **preview / apply** imports into Radarr, Sonarr, and Seerr —
with TorBox-friendly **search-on-add rate limiting**.

> Inspired by [fisherd80/Listarr](https://github.com/fisherd80/listarr) (MIT).
> This is a clean-room Go implementation, not a fork of that repository.
> See [NOTICE](NOTICE).

**Status:** early bootstrap (API health/status + rate-limiter library).  
TMDB / IMDB / Seerr import paths land next.

## Public-repo security

This repository is **public**. Do not commit API keys, tokens, cookies, or
hostnames that identify a private network. Defaults bind to loopback and keep
**apply disabled** until you opt in.

| Rule | Detail |
|------|--------|
| Secrets | Env vars or a local ignore-listed config file only |
| Apply | Mutations require `LISTARR_APPLY=1` |
| Listen | Default `127.0.0.1:8787` (not all interfaces) |
| Examples | Use `example.invalid` / `127.0.0.1` placeholders only |

See [SECURITY.md](SECURITY.md).

## Quick start (local)

```bash
export LISTARR_API_KEY='replace-with-a-long-random-string'
# optional: export LISTARR_APPLY=1   # off by default
go run ./cmd/listarr-go
```

```bash
curl -s http://127.0.0.1:8787/health
curl -s -H "X-Api-Key: $LISTARR_API_KEY" http://127.0.0.1:8787/api/v1/system/status
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

Service URLs (Radarr / Sonarr / Seerr / TMDB) are **not** defaulted. Configure
them explicitly when those clients land — never bake private MagicDNS or LAN
names into the binary.

Copy [`.env.example`](.env.example) to `.env` (gitignored) for local use.

## Goals

Documented in [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md):

- Feature parity direction: fisherd80/Listarr (TMDB discovery, lists, *arr import)
- Forever a **preview/apply tool** (does not replace *arr Import Lists)
- Import/export: TMDB, IMDB, Seerr
- Seerr: add/delete requests and trigger existing download pipelines
- TorBox `searchOnAdd` with a **60/hour** default rate limit
- Trakt: out of scope for now

## License

MIT — see [LICENSE](LICENSE). Upstream Listarr is also MIT; credit in [NOTICE](NOTICE).
