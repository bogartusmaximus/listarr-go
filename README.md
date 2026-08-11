# listarr-go

Go rewrite of the [Listarr](https://github.com/fisherd80/listarr) idea: discover
and **preview / apply** media into Radarr/Sonarr — including **multi-instance
library sync** (local ↔ external) — with TorBox-friendly search-on-add limits.

> Inspired by [fisherd80/Listarr](https://github.com/fisherd80/listarr) (MIT).
> Clean-room Go implementation; see [NOTICE](NOTICE).

**Status:** v0.3 — TMDB discover + named *arr registry + `arr-library` sync.

## Public-repo security

| Rule | Detail |
|------|--------|
| Secrets | Env vars only (gitignored `.env`) |
| Apply | Requires `LISTARR_APPLY=1` |
| Listen | Default `127.0.0.1:8787` |
| Examples | `127.0.0.1` placeholders only — never private MagicDNS |

See [SECURITY.md](SECURITY.md).

## Dual *arr sync (first use case)

Configure two Radarrs (same pattern for Sonarr):

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

## Configuration (env)

| Variable | Default | Notes |
|----------|---------|-------|
| `LISTARR_API_KEY` | required | Auth |
| `LISTARR_LISTEN` | `127.0.0.1:8787` | Loopback |
| `LISTARR_APPLY` | off | Set `1` to mutate |
| `LISTARR_TORBOX_SEARCH_PER_HOUR` | `60` | Search budget |
| `LISTARR_TMDB_API_KEY` | none | Discover |
| `LISTARR_ARR_<NAME>_URL` | none | Named instance |
| `LISTARR_ARR_<NAME>_API_KEY` | none | Named instance |
| `LISTARR_ARR_<NAME>_KIND` | none | `radarr` or `sonarr` |
| `LISTARR_RADARR_*` / `LISTARR_SONARR_*` | none | Legacy aliases → names `radarr` / `sonarr` |

## Goals

See [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md).

## License

MIT — [LICENSE](LICENSE). Upstream Listarr credit — [NOTICE](NOTICE).
