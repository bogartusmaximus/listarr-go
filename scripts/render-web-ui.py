#!/usr/bin/env python3
"""Render listarr-go web UI from Jinja2 sources (web/src → web/)."""

from __future__ import annotations

import shutil
import sys
from pathlib import Path

from jinja2 import ChoiceLoader, Environment, FileSystemLoader, select_autoescape

ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "web" / "src"
SHARED = ROOT / "web" / "templates" / "_shared"
WEB = ROOT / "web"


def _env() -> Environment:
    if not SRC.is_dir():
        raise SystemExit(f"missing {SRC}")
    if not SHARED.is_dir():
        raise SystemExit(f"missing {SHARED} — run autobot-homelab/scripts/go-ui/sync-shared-templates.sh")
    return Environment(
        loader=ChoiceLoader(
            [
                FileSystemLoader(str(SRC)),
                FileSystemLoader(str(SHARED)),
            ]
        ),
        autoescape=select_autoescape(["html", "xml"]),
        trim_blocks=True,
        lstrip_blocks=True,
    )


def main() -> int:
    env = _env()
    html = env.get_template("index.html.j2").render(defer_scripts=True)
    (WEB / "index.html").write_text(html, encoding="utf-8")
    css = env.get_template("assets/app.css.j2").render()
    assets = WEB / "assets"
    assets.mkdir(parents=True, exist_ok=True)
    (assets / "app.css").write_text(css, encoding="utf-8")
    shutil.copy2(SRC / "assets" / "app.js", assets / "app.js")
    print(f"rendered {WEB / 'index.html'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
