#!/usr/bin/env python3
"""HTTP contract checks for listarr-go (health, auth, apply gate, sync routes)."""

from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request
from typing import Any


def request(
    method: str,
    url: str,
    *,
    headers: dict[str, str] | None = None,
    data: bytes | None = None,
    timeout: float = 10.0,
) -> tuple[int, Any]:
    req = urllib.request.Request(url, method=method, data=data, headers=headers or {})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = resp.read()
            if not body:
                return resp.status, None
            text = body.decode(errors="replace")
            try:
                return resp.status, json.loads(text)
            except json.JSONDecodeError:
                return resp.status, text
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        try:
            parsed = json.loads(raw.decode()) if raw else None
        except json.JSONDecodeError:
            parsed = {"raw": raw.decode(errors="replace")}
        return exc.code, parsed


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--api-key", required=True)
    args = parser.parse_args()
    base = args.base_url.rstrip("/")
    key = args.api_key
    failures = 0

    status, body = request("GET", f"{base}/health")
    if status != 200 or not isinstance(body, dict) or body.get("status") != "ok":
        print(f"FAIL health: status={status} body={body}")
        failures += 1
    else:
        print("PASS health")

    status, body = request("GET", f"{base}/")
    if status != 200 or not isinstance(body, str) or "listarr-go" not in body:
        print(f"FAIL ui index: status={status}")
        failures += 1
    else:
        print("PASS ui index")

    status, body = request("GET", f"{base}/api/v1/ui/bootstrap")
    if (
        status != 200
        or not isinstance(body, dict)
        or body.get("apiKey") != key
    ):
        print(f"FAIL ui bootstrap: status={status} body={body}")
        failures += 1
    else:
        print("PASS ui bootstrap")

    status, _ = request("GET", f"{base}/api/v1/system/status")
    if status != 401:
        print(f"FAIL status unauth: status={status} want=401")
        failures += 1
    else:
        print("PASS status unauth")

    status, body = request(
        "GET",
        f"{base}/api/v1/system/status",
        headers={"X-Api-Key": key},
    )
    if (
        status != 200
        or not isinstance(body, dict)
        or body.get("appName") != "listarr-go"
        or body.get("applyEnabled") is not False
        or body.get("storeBackend") != "polars"
    ):
        print(f"FAIL status auth: status={status} body={body}")
        failures += 1
    else:
        print("PASS status auth")

    status, body = request(
        "GET",
        f"{base}/api/v1/activity",
        headers={"X-Api-Key": key},
    )
    if (
        status != 200
        or not isinstance(body, dict)
        or body.get("backend") != "polars"
        or not isinstance(body.get("runs"), list)
    ):
        print(f"FAIL activity: status={status} body={body}")
        failures += 1
    else:
        print("PASS activity")

    status, body = request(
        "GET",
        f"{base}/api/v1/settings",
        headers={"X-Api-Key": key},
    )
    if (
        status != 200
        or not isinstance(body, dict)
        or body.get("apiKey") != key
        or not isinstance(body.get("arrInstances"), list)
    ):
        print(f"FAIL settings get: status={status} body={body}")
        failures += 1
    else:
        print("PASS settings get")

    payload = json.dumps(
        {
            "source": "tmdb",
            "mediaType": "movie",
            "tmdbIds": [1],
            "target": {"rootFolderPath": "/data/movies", "qualityProfileId": 1},
        }
    ).encode()
    status, _ = request(
        "POST",
        f"{base}/api/v1/sync/apply",
        headers={"X-Api-Key": key, "Content-Type": "application/json"},
        data=payload,
    )
    if status != 403:
        print(f"FAIL apply gate: status={status} want=403")
        failures += 1
    else:
        print("PASS apply gate")

    status, _ = request(
        "GET",
        f"{base}/api/v1/discover/movies",
        headers={"X-Api-Key": key},
    )
    if status != 503:
        print(f"FAIL discover without tmdb: status={status} want=503")
        failures += 1
    else:
        print("PASS discover without tmdb")

    if failures:
        print(f"{failures} failure(s)")
        return 1
    print("all contracts passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
