# listarr-go requirements (product SoT)

Locked operator intent (2026-08-11). Homelab-specific deploy notes live in private
ops docs — **not** in this public repository.

## Goal

A standalone Go app that discovers and manages media lists (TMDB / IMDB / Seerr),
preview/applies them into Radarr/Sonarr (and Seerr), and can drive download
pipelines with explicit TorBox-friendly rate limits — forever a **tool**, never
a forced replacement of *arr native Import Lists.

## Locked decisions

| Decision | Choice |
|----------|--------|
| Feature SoT | [fisherd80/Listarr](https://github.com/fisherd80/listarr) product surface |
| Language | Go |
| Credit | NOTICE + README |
| Trakt | Out of scope for now |
| Stance | Preview/apply tool forever |
| TorBox search-on-add | On, default **60 items / rolling hour** |
| Import/export | TMDB, IMDB, Seerr |
| Seerr | First-class: add/delete + trigger existing Seerr → *arr pipelines |
| Secrets | Env / local ignore-listed config only; public-repo safe defaults |

## Non-goals (near term)

- Trakt OAuth / Trakt lists
- Replacing Seerr’s friend/family UI
- Rewriting Radarr / Sonarr / Prowlarr
- Shipping anyone’s private hostnames or credentials as defaults

## Phases

| Phase | Scope |
|-------|--------|
| 0–1a | Health/status API, apply kill switch, TorBox search rate limiter, public security posture |
| 1b | TMDB discover + preview/apply → Radarr/Sonarr — **current** |
| 2 | IMDB + Seerr import/export |
| 3 | Seerr add/delete + pipeline trigger |
| 4 | UI parity (wizard / activity) + optional MCP |

## Success metrics

1. Preview never mutates; apply is opt-in and idempotent.
2. Search-triggering adds stay within the configured hourly budget.
3. No secrets or private inventory in the default binary or docs examples.
4. Agents can drive preview/apply over HTTP without a browser.
