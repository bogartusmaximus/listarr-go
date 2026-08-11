#!/usr/bin/env python3
"""Assert polars can read the Go polars-backend CSV dump."""

from __future__ import annotations

import sys
from pathlib import Path


def main() -> int:
    try:
        import polars as pl
    except ImportError:
        print("SKIP: polars not installed")
        return 0

    path = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("data/polars/sync_runs.csv")
    if not path.is_file():
        print(f"FAIL: missing {path}")
        return 1
    df = pl.read_csv(path)
    required = {
        "id",
        "dry_run",
        "source",
        "media_type",
        "adds",
        "skips",
        "created_at",
    }
    missing = required - set(df.columns)
    if missing:
        print(f"FAIL: missing columns {missing}")
        return 1
    if df.height < 1:
        print("FAIL: expected at least one row")
        return 1
    print(f"PASS polars rows={df.height} cols={df.columns}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
