# Security policy

## Reporting

If you find a vulnerability in **listarr-go**, open a [GitHub security advisory](https://github.com/bogartusmaximus/listarr-go/security/advisories/new) (preferred) or contact the maintainer privately. Do not file a public issue that includes secrets or private hostnames.

## Maintainer commitments (public repo)

1. **No secrets in git** — API keys, tokens, session cookies, and private URLs must not appear in commits, fixtures, or docs.
2. **Safe defaults** — on first boot an API key is generated into the datastore (logged once); listens on loopback by default; **apply is off** unless enabled in Settings (seeded from `LISTARR_APPLY=1`).
3. **No personal inventory in defaults** — the binary must not ship with anyone’s Radarr/Sonarr/Seerr/TMDB endpoints, usernames, list IDs, or Tailscale names.
4. **Log redaction** — authenticated request URLs and headers must not be logged with raw keys (`apikey=`, `X-Api-Key`, bearer tokens). The one-time “generated initial API key” log line is intentional for first-boot recovery.
5. **Rate limits** — TorBox-oriented search-on-add is budgeted (default 60/hour) so a misconfig cannot freely hammer a debrid account.
6. **Settings store** — operator secrets live in the datastore (`settings.json` / Postgres). Keep that data out of git; treat authenticated Settings UI as secret-capable (Show / Copy / Regenerate).

## Operators

- Put bootstrap config in environment variables or a gitignored local `.env`; the listarr API key is **not** an env var — copy it from first-boot logs or Settings.
- Expose the API beyond loopback only behind your own TLS / auth gateway.
- Treat preview vs apply deliberately; keep apply disabled on shared hosts until trusted.
