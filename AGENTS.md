# Agent notes (public repository)

- **Never** commit secrets, `.env`, private MagicDNS/LAN hostnames, or personal list IDs.
- Examples must use `127.0.0.1` or `example.invalid` only.
- Apply stays off (Safe Mode on) unless `LISTARR_APPLY=1` seeds otherwise.
- Prefer tests before new behavior; keep functions small; redact `apikey=` in logs/errors.
- Feature direction: [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md). Credit Listarr in NOTICE.

## Agent ↔ operator workflow (Docker)

1. Implement in git on a feature branch; run `go test` / contract checks as needed.
2. **Commit and push** the branch; open/merge the PR to `main`.
3. **Do not** start, rebuild, or `docker compose up` containers for the operator.
4. The operator pulls `main` and runs compose locally, e.g.:

```bash
cd listarr-go
git pull
docker compose -f compose.yaml -f compose.host.yaml up --build -d
```
