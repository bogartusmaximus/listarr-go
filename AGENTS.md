# Agent notes (public repository)

- **Never** commit secrets, `.env`, private MagicDNS/LAN hostnames, or personal list IDs.
- Examples must use `127.0.0.1` or `example.invalid` only.
- Apply stays off (Safe Mode on) unless `LISTARR_APPLY=1` seeds otherwise.
- Prefer tests before new behavior; keep functions small; redact `apikey=` in logs/errors.
- Feature direction: [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md). Credit Listarr in NOTICE.

## Agent ↔ operator workflow (Docker)

1. Implement in git on a feature branch; run `go test` / contract checks as needed.
2. **Commit and push** the branch; open/merge the PR to `main`.
3. **Do not** start, rebuild, or `docker compose up` on the operator workstation.
4. **Frequent homelab deploys** use the self-hosted **`ci-development`** runner:
   fast-forward / push the `ci-development` branch (see `.github/workflows/ci-development.yml`).
   That stack is app + Tailscale Serve sidecar (`compose.ci-development.yaml`).
   Do not put private MagicDNS or LAN hosts in this public repo.
5. **LXC** deploy waits until a stable release. Do not provision a guest for this app yet.
6. Laptop / local demo stays loopback:

```bash
cd listarr-go
git pull
docker compose -f compose.yaml -f compose.host.yaml up --build -d
```
