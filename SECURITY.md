# Security policy

## Reporting

If you find a vulnerability in **listarr-go**, open a [GitHub security advisory](https://github.com/bogartusmaximus/listarr-go/security/advisories/new) (preferred) or contact the maintainer privately. Do not file a public issue that includes secrets or private hostnames.

## Maintainer commitments (public repo)

1. **No secrets in git** — API keys, tokens, session cookies, and private URLs must not appear in commits, fixtures, or docs.
2. **Safe defaults** — the process refuses to seed an empty store without `LISTARR_API_KEY`; listens on loopback by default; **apply is off** unless enabled in Settings (seeded from `LISTARR_APPLY=1`).
3. **No personal inventory in defaults** — the binary must not ship with anyone’s Radarr/Sonarr/Seerr/TMDB endpoints, usernames, list IDs, or Tailscale names.
4. **Log redaction** — authenticated request URLs and headers must not be logged with raw keys (`apikey=`, `X-Api-Key`, bearer tokens).
5. **Rate limits** — TorBox-oriented search-on-add is budgeted (default 60/hour) so a misconfig cannot freely hammer a debrid account.
6. **Settings store** — operator secrets may live in the datastore (`settings.json` / Postgres). Keep that data out of git; treat authenticated Settings UI as secret-capable.

## Operators

- Put bootstrap secrets in environment variables or a gitignored local config; after first seed, prefer the Settings UI/API.
- Expose the API beyond loopback only behind your own TLS / auth gateway.
- Treat preview vs apply deliberately; keep apply disabled on shared hosts until trusted.
