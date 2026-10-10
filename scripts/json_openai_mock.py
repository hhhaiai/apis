#!/usr/bin/env python3
"""Tiny OpenAI-compatible JSON mock upstream for local gateway smoke tests.

It is intentionally dependency-free and only implements the subset used by
the smoke scripts: GET /v1/models and POST /v1/chat/completions, plus the
/openai/v1 alias used by Groq-compatible clients.
"""

from __future__ import annotations

import argparse
import json
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Handler(BaseHTTPRequestHandler):
    server_version = "ccgw-json-openai-mock/1.0"

    def do_GET(self) -> None:  # noqa: N802 - http.server hook name
        if self.path == "/healthz":
            self._write_json({"ok": True})
            return
        if self.path in ("/v1/models", "/openai/v1/models"):
            self._write_json({"object": "list", "data": [{"id": "json-smoke", "object": "model"}]})
            return
        self.send_error(404)

    def do_POST(self) -> None:  # noqa: N802 - http.server hook name
        if self.path not in ("/v1/chat/completions", "/openai/v1/chat/completions"):
            self.send_error(404)
            return

        length = int(self.headers.get("content-length", "0") or "0")
        raw = self.rfile.read(length) if length else b"{}"
        try:
            body = json.loads(raw.decode("utf-8"))
        except Exception:
            body = {}

        content = {
            "summary": "local gateway json smoke",
            "intent": "agent smoke",
            "model": body.get("model", "json-smoke"),
        }
        text = json.dumps(content, ensure_ascii=False)

        if body.get("stream"):
            self.send_response(200)
            self.send_header("content-type", "text/event-stream")
            self.send_header("cache-control", "no-cache")
            self.end_headers()
            chunk = {
                "id": f"chatcmpl-json-{int(time.time())}",
                "object": "chat.completion.chunk",
                "created": int(time.time()),
                "model": body.get("model", "json-smoke"),
                "choices": [{"index": 0, "delta": {"content": text}, "finish_reason": None}],
            }
            self.wfile.write(f"data: {json.dumps(chunk, ensure_ascii=False)}\n\n".encode("utf-8"))
            done = {
                "id": chunk["id"],
                "object": "chat.completion.chunk",
                "created": chunk["created"],
                "model": chunk["model"],
                "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
            }
            self.wfile.write(f"data: {json.dumps(done, ensure_ascii=False)}\n\n".encode("utf-8"))
            self.wfile.write(b"data: [DONE]\n\n")
            return

        response = {
            "id": f"chatcmpl-json-{int(time.time())}",
            "object": "chat.completion",
            "created": int(time.time()),
            "model": body.get("model", "json-smoke"),
            "choices": [
                {
                    "index": 0,
                    "message": {"role": "assistant", "content": text},
                    "finish_reason": "stop",
                }
            ],
            "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
        }
        self._write_json(response)

    def log_message(self, fmt: str, *args: object) -> None:
        return

    def _write_json(self, data: object) -> None:
        raw = json.dumps(data, ensure_ascii=False).encode("utf-8")
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, required=True)
    args = parser.parse_args()
    server = ThreadingHTTPServer((args.host, args.port), Handler)
    print(f"json openai mock listening on http://{args.host}:{args.port}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
