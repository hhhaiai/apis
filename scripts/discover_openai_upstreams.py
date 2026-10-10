#!/usr/bin/env python3
"""Discover local OpenAI-compatible upstream candidates.

The script is deliberately conservative: it only probes localhost, prints
classifications, and suggests an exact smoke_real_models.sh command when an
endpoint answers an OpenAI-compatible chat completion with parseable JSON.
"""

from __future__ import annotations

import argparse
import os
import json
import re
import shlex
import socket
import subprocess
import urllib.error
import urllib.request
from dataclasses import dataclass
from typing import Iterable


COMMON_PORTS = [
    5025,
    5022,
    8000,
    8080,
    8885,
    8887,
    18080,
    18081,
    23333,
    28080,
    3000,
    3001,
]

DEFAULT_MODELS = [
    "GLM-5",
    "GLM-4.7",
    "gpt-4o",
    "gpt-4.1",
    "qwen-max",
    "deepseek-chat",
]

DEFAULT_KEYS = ["free", "admin123456"]

OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}))


@dataclass
class ProbeResult:
    port: int
    base_url: str
    status: str
    detail: str
    api_key: str | None = None
    model: str | None = None
    endpoint: str = "/v1/chat/completions"


def open_ports_from_lsof() -> set[int]:
    ports: set[int] = set()
    try:
        output = subprocess.check_output(
            ["lsof", "-nP", "-iTCP", "-sTCP:LISTEN"],
            text=True,
            stderr=subprocess.DEVNULL,
        )
    except Exception:
        return ports

    for line in output.splitlines()[1:]:
        match = re.search(r":(\d+)\s+\(LISTEN\)$", line)
        if not match:
            continue
        ports.add(int(match.group(1)))
    return ports


def tcp_open(port: int, timeout: float) -> bool:
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=timeout):
            return True
    except OSError:
        return False


def http_request(url: str, method: str = "GET", body: object | None = None, key: str | None = None, timeout: float = 2.0) -> tuple[int, str]:
    data = None
    headers = {"content-type": "application/json"}
    if body is not None:
        data = json.dumps(body, ensure_ascii=False).encode("utf-8")
    if key:
        headers["authorization"] = f"Bearer {key}"
        headers["x-api-key"] = key
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with OPENER.open(req, timeout=timeout) as resp:
            return resp.status, resp.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read().decode("utf-8", errors="replace")
    except Exception as exc:
        return 0, str(exc)


def classify_without_key(base_url: str, timeout: float) -> str:
    snippets: list[str] = []
    for path in ("/healthz", "/v1/models", "/openai/v1/models"):
        status, text = http_request(base_url + path, timeout=timeout)
        if status:
            snippets.append(f"{path}={status}:{text[:120].replace(chr(10), ' ')}")
    return " | ".join(snippets) or "no http response"


def unique_ordered(values: Iterable[str]) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []
    for value in values:
        item = str(value).strip()
        if not item or item in seen:
            continue
        seen.add(item)
        out.append(item)
    return out


def env_key_candidates() -> list[str]:
    """Return local-only auth candidates from env without making them mandatory."""
    values: list[str] = []
    for name in (
        "CCGW_REAL_SMOKE_UPSTREAM_API_KEY",
        "LOCAL_OPENAI_API_KEY",
        "OPENAI_API_KEY",
    ):
        value = os.environ.get(name)
        if value:
            values.append(value)
    raw = os.environ.get("CCGW_REAL_SMOKE_DISCOVER_KEYS", "")
    if raw:
        values.extend(re.split(r"[\s,]+", raw))
    return unique_ordered(values)


def discover_models(base_url: str, key: str | None, timeout: float, models_path: str = "/v1/models") -> list[str]:
    status, text = http_request(base_url + models_path, key=key, timeout=timeout)
    if status != 200:
        return []
    try:
        raw = json.loads(text)
    except json.JSONDecodeError:
        return []

    data = raw.get("data") if isinstance(raw, dict) else None
    if not isinstance(data, list):
        return []

    ids: list[str] = []
    for item in data:
        if isinstance(item, dict):
            model_id = item.get("id") or item.get("name")
            if isinstance(model_id, str):
                ids.append(model_id)
        elif isinstance(item, str):
            ids.append(item)
    return unique_ordered(ids)


def chat_json_probe(base_url: str, key: str, model: str, timeout: float, endpoint: str = "/v1/chat/completions") -> tuple[bool, str]:
    payload = {
        "model": model,
        "max_tokens": 128,
        "response_format": {"type": "json_object"},
        "messages": [
            {"role": "system", "content": "Return JSON only."},
            {
                "role": "user",
                "content": "Return a JSON object with summary=real_model_smoke and intent=gateway_quality_check.",
            },
        ],
    }
    status, text = http_request(base_url + endpoint, method="POST", body=payload, key=key, timeout=timeout)
    if status != 200:
        return False, f"endpoint={endpoint} model={model} chat={status}:{text[:180].replace(chr(10), ' ')}"
    try:
        raw = json.loads(text)
        content = raw["choices"][0]["message"]["content"]
        parsed = json.loads(content)
    except Exception as exc:
        return False, f"endpoint={endpoint} model={model} chat=200 but content not parseable JSON: {text[:220]!r}; error={exc}"
    if not isinstance(parsed, dict):
        return False, f"endpoint={endpoint} model={model} chat=200 but parsed content is not object: {parsed!r}"
    return True, f"endpoint={endpoint} model={model} chat=200 json={parsed}"


def probe_models(
    base_url: str,
    key: str,
    requested_models: list[str],
    timeout: float,
    endpoint: str = "/v1/chat/completions",
    models_path: str = "/v1/models",
) -> tuple[bool, str, str | None]:
    candidate_models = unique_ordered([*requested_models, *discover_models(base_url, key, timeout, models_path)])
    details: list[str] = []
    for model in candidate_models:
        ok, chat_detail = chat_json_probe(base_url, key, model, timeout, endpoint)
        if ok:
            return True, chat_detail, model
        details.append(chat_detail)
        low = chat_detail.lower()
        if "invalid" in low or "auth" in low or "forbidden" in low:
            break
    return False, " | ".join(details[-3:]) if details else "no model probe attempted", None


def discover(ports: Iterable[int], keys: list[str], models: list[str], timeout: float, stop_on_ready: bool = False) -> list[ProbeResult]:
    results: list[ProbeResult] = []
    route_profiles = [
        ("/v1/chat/completions", "/v1/models"),
        ("/openai/v1/chat/completions", "/openai/v1/models"),
    ]
    for port in sorted(set(ports)):
        if not tcp_open(port, timeout=min(timeout, 1.0)):
            continue
        base_url = f"http://127.0.0.1:{port}"
        detail = classify_without_key(base_url, timeout=min(timeout, 0.35))
        best = ProbeResult(port=port, base_url=base_url, status="candidate", detail=detail)
        if detail == "no http response":
            results.append(best)
            continue
        for endpoint, models_path in route_profiles:
            for key in keys:
                ok, chat_detail, model = probe_models(base_url, key, models, timeout, endpoint, models_path)
                if ok:
                    best = ProbeResult(port=port, base_url=base_url, status="ready", detail=chat_detail, api_key=key, model=model, endpoint=endpoint)
                    break
                if "invalid" in chat_detail.lower() or "auth" in chat_detail.lower() or "forbidden" in chat_detail.lower():
                    best.status = "requires_auth"
                    best.detail = chat_detail
                    best.endpoint = endpoint
                elif best.status == "candidate":
                    best.detail = f"{detail} | {chat_detail}"
            if best.status == "ready":
                break
        results.append(best)
        if stop_on_ready and best.status == "ready":
            return results
    return results


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ports", nargs="*", type=int, help="ports to probe; defaults to common ports plus lsof-discovered ports")
    parser.add_argument(
        "--keys",
        nargs="*",
        help=(
            "candidate API keys to try; defaults to CCGW_REAL_SMOKE_DISCOVER_KEYS/"
            "CCGW_REAL_SMOKE_UPSTREAM_API_KEY/LOCAL_OPENAI_API_KEY/OPENAI_API_KEY plus free/admin123456"
        ),
    )
    parser.add_argument("--model", default="GLM-5", help="primary model name to try first")
    parser.add_argument("--models", nargs="*", help="extra model names to try before /v1/models discovered IDs")
    parser.add_argument("--format", choices=["text", "shell"], default="text", help="output format; shell prints exportable variables for the first ready endpoint")
    parser.add_argument("--timeout", type=float, default=2.0)
    args = parser.parse_args()

    ports = set(args.ports or COMMON_PORTS)
    if args.ports is None:
        ports |= {p for p in open_ports_from_lsof() if p < 65536}

    keys = unique_ordered([*(args.keys or []), *env_key_candidates(), *DEFAULT_KEYS])
    models = unique_ordered([args.model, *(args.models or []), *DEFAULT_MODELS])
    results = discover(ports, keys, models, args.timeout, stop_on_ready=args.format == "shell")
    ready = [r for r in results if r.status == "ready"]

    if args.format == "shell":
        if ready:
            r = ready[0]
            print(f"CCGW_REAL_SMOKE_UPSTREAM_URL={shlex.quote(r.base_url)}")
            print(f"CCGW_REAL_SMOKE_UPSTREAM_API_KEY={shlex.quote(r.api_key or '')}")
            print(f"CCGW_REAL_SMOKE_MODEL={shlex.quote(r.model or args.model)}")
            print(f"CCGW_REAL_SMOKE_UPSTREAM_ENDPOINT={shlex.quote(r.endpoint)}")
            return 0
        return 2

    for r in results:
        print(f"{r.status.upper():13} {r.base_url:28} {r.detail}")

    if ready:
        r = ready[0]
        print("\nSuggested strict real-model smoke:")
        print(f"CCGW_REAL_SMOKE_UPSTREAM_URL={r.base_url} \\")
        print(f"CCGW_REAL_SMOKE_UPSTREAM_API_KEY={r.api_key} \\")
        print(f"CCGW_REAL_SMOKE_MODEL={r.model} \\")
        print(f"CCGW_REAL_SMOKE_UPSTREAM_ENDPOINT={r.endpoint} \\")
        print("bash scripts/smoke_real_models.sh")
        return 0

    print("\nNo ready OpenAI-compatible JSON upstream found.")
    auth_candidates = [r for r in results if r.status == "requires_auth"]
    if auth_candidates:
        ports_hint = " ".join(str(r.port) for r in auth_candidates)
        print("\nEndpoints that look like OpenAI-compatible services but need a working key:")
        for r in auth_candidates:
            print(f"  - {r.base_url}{r.endpoint}: {r.detail[:240]}")
        print("\nRerun discovery with your local key, for example:")
        print(
            "  python3 scripts/discover_openai_upstreams.py "
            f"--timeout {args.timeout:g} --ports {ports_hint} --keys <your-local-key> "
            f"--models {' '.join(shlex.quote(m) for m in models[:6])}"
        )
        print("\nThen run the strict smoke with the discovered URL/key/model:")
        print("  CCGW_REAL_SMOKE_UPSTREAM_URL=http://127.0.0.1:<port> \\")
        print("  CCGW_REAL_SMOKE_UPSTREAM_API_KEY=<your-local-key> \\")
        print(f"  CCGW_REAL_SMOKE_MODEL={shlex.quote(args.model)} \\")
        print("  CCGW_VERIFY_REQUIRE_REAL=true bash scripts/verify_agent_gateway.sh")
    else:
        print("Start a local OpenAI-compatible model service, or pass --ports/--keys/--models for your runtime.")
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
