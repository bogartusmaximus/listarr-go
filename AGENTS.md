# Agent notes (public repository)

- **Never** commit secrets, `.env`, private MagicDNS/LAN hostnames, or personal list IDs.
- Examples must use `127.0.0.1` or `example.invalid` only.
- Apply stays off unless `LISTARR_APPLY=1`.
- Prefer tests before new behavior; keep functions small; redact `apikey=` in logs/errors.
- Feature direction: [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md). Credit Listarr in NOTICE.
