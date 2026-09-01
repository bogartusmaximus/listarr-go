# homelab-ui — shared Jinja2 web theme

Canonical look-and-feel for homelab **\*-go** dashboards and the **homepage** (Python/Jinja2).

**Palette SoT:** `tdarr-server-go` dark slate + teal (`--accent: #3dbe9a`).

## Layout

| Partial | Purpose |
|---------|---------|
| `_tokens.css.j2` | CSS custom properties |
| `_reset.css.j2` | Global reset, dark color-scheme |
| `_layout.css.j2` | Header, chips, tables, cards, footer |
| `_forms.css.j2` | Tabs, forms, buttons (operator UIs) |
| `_layout.html.j2` | Base HTML document |

## Render

From repo root:

```bash
./scripts/go-ui/render.py tdarr-go
./scripts/go-ui/render.py listarr-go --project-root /path/to/listarr-go
```

Go binaries `go:embed` the **rendered** `web/index.html` and `web/assets/*`. Source lives under each project's `web/src/`.

Homepage pulls the same tokens via `scripts/homepage/templates/styles/homepage.css.j2`.

## Sync to listarr-go (public repo)

```bash
./scripts/go-ui/sync-shared-templates.sh /path/to/listarr-go
```

Copies this directory to `web/templates/_shared/` in the external repo so standalone builds do not depend on autobot-homelab checkout.
