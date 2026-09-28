#!/usr/bin/env python3
"""Capture client session-affinity fields against a local-only mock API."""

from __future__ import annotations

import argparse
import hashlib
import json
import secrets
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any

SESSION_HEADERS = frozenset(
    name.lower()
    for name in (
        "X-NewAPI-Session-ID",
        "thread-id",
        "thread_id",
        "session-id",
        "session_id",
        "x-session-affinity",
        "x-session-id",
        "x-client-request-id",
    )
)
SECRET_HEADERS = {"authorization", "proxy-authorization", "x-api-key", "api-key"}
MAX_BODY_BYTES = 1 << 20


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bind", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=18081)
    parser.add_argument("--output", required=True)
    return parser.parse_args()


def sanitized_headers(headers: Any, digest_salt: bytes) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for name, value in headers.items():
        normalized = name.lower()
        if normalized in SECRET_HEADERS:
            result[name] = "<redacted>"
            continue
        if normalized in SESSION_HEADERS or normalized == "user-agent":
            result[name] = {
                "present": True,
                "digest": session_value_digest(value, digest_salt),
            }
    return result


def session_value_digest(value: str, digest_salt: bytes) -> str:
    return hashlib.sha256(digest_salt + b"\x00" + value.encode("utf-8")).hexdigest()[:16]


def sanitized_body(body: bytes) -> dict[str, Any]:
    if not body:
        return {}
    try:
        decoded = json.loads(body)
    except (UnicodeDecodeError, json.JSONDecodeError):
        return {"body_bytes": len(body)}
    if not isinstance(decoded, dict):
        return {"body_type": type(decoded).__name__}
    result: dict[str, Any] = {}
    for key in ("model", "stream", "session_id", "prompt_cache_key"):
        if key not in decoded:
            continue
        value = decoded[key]
        if key in ("session_id", "prompt_cache_key"):
            result[key + "_type"] = type(value).__name__
            result[key + "_present"] = True
        else:
            result[key] = value
    return result


class CaptureServer(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, address: tuple[str, int], output: Path):
        super().__init__(address, CaptureHandler)
        self.output = output
        self.output_lock = threading.Lock()
        self.digest_salt = secrets.token_bytes(32)

    def append_record(self, record: dict[str, Any]) -> None:
        line = json.dumps(record, ensure_ascii=False, sort_keys=True)
        with self.output_lock:
            with self.output.open("a", encoding="utf-8") as handle:
                handle.write(line + "\n")


class CaptureHandler(BaseHTTPRequestHandler):
    server: CaptureServer

    def log_message(self, format: str, *args: Any) -> None:
        return

    def do_GET(self) -> None:
        self._handle()

    def do_POST(self) -> None:
        self._handle()

    def _handle(self) -> None:
        length = int(self.headers.get("Content-Length", "0") or "0")
        body = self.rfile.read(min(length, MAX_BODY_BYTES)) if length > 0 else b""
        record = {
            "timestamp": time.time(),
            "method": self.command,
            "path": self.path,
            "headers": sanitized_headers(self.headers, self.server.digest_salt),
            "body": sanitized_body(body),
        }
        self.server.append_record(record)
        self._respond(record)

    def _respond(self, record: dict[str, Any]) -> None:
        path = record["path"]
        if path.endswith("/v1/models"):
            self._json({"object": "list", "data": [{"id": "deepseek v4.1 flash", "object": "model"}]})
            return
        stream = bool(record["body"].get("stream"))
        if path.endswith("/v1/responses"):
            if stream:
                self._responses_stream()
                return
            self._json({
                "id": "resp_capture",
                "object": "response",
                "status": "completed",
                "output": [{
                    "type": "message",
                    "role": "assistant",
                    "content": [{"type": "output_text", "text": "capture-ok"}],
                }],
                "usage": {"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
            })
            return
        if stream:
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.end_headers()
            self.wfile.write(b'data: {"id":"chatcmpl_capture","choices":[{"index":0,"delta":{"content":"capture-ok"},"finish_reason":null}]}\n\n')
            self.wfile.write(b'data: {"id":"chatcmpl_capture","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}\n\n')
            self.wfile.write(b"data: [DONE]\n\n")
            self.wfile.flush()
            return
        self._json({
            "id": "chatcmpl_capture",
            "object": "chat.completion",
            "choices": [{"index": 0, "message": {"role": "assistant", "content": "capture-ok"}, "finish_reason": "stop"}],
            "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
        })

    def _responses_stream(self) -> None:
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.end_headers()
        events = [
            ("response.created", {
                "type": "response.created",
                "response": {
                    "id": "resp_capture",
                    "object": "response",
                    "status": "in_progress",
                    "output": [],
                },
            }),
            ("response.output_item.added", {
                "type": "response.output_item.added",
                "output_index": 0,
                "item": {
                    "id": "msg_capture",
                    "type": "message",
                    "status": "in_progress",
                    "role": "assistant",
                    "content": [],
                },
            }),
            ("response.output_text.delta", {
                "type": "response.output_text.delta",
                "item_id": "msg_capture",
                "output_index": 0,
                "content_index": 0,
                "delta": "capture-ok",
            }),
            ("response.output_text.done", {
                "type": "response.output_text.done",
                "item_id": "msg_capture",
                "output_index": 0,
                "content_index": 0,
                "text": "capture-ok",
            }),
            ("response.output_item.done", {
                "type": "response.output_item.done",
                "output_index": 0,
                "item": {
                    "id": "msg_capture",
                    "type": "message",
                    "status": "completed",
                    "role": "assistant",
                    "content": [{"type": "output_text", "text": "capture-ok"}],
                },
            }),
            ("response.completed", {
                "type": "response.completed",
                "response": {
                    "id": "resp_capture",
                    "object": "response",
                    "status": "completed",
                    "output": [{
                        "id": "msg_capture",
                        "type": "message",
                        "status": "completed",
                        "role": "assistant",
                        "content": [{"type": "output_text", "text": "capture-ok"}],
                    }],
                    "usage": {
                        "input_tokens": 1,
                        "output_tokens": 1,
                        "total_tokens": 2,
                    },
                },
            }),
        ]
        for event, payload in events:
            self.wfile.write(f"event: {event}\n".encode("utf-8"))
            self.wfile.write(b"data: ")
            self.wfile.write(json.dumps(payload).encode("utf-8"))
            self.wfile.write(b"\n\n")
            self.wfile.flush()
        self.wfile.write(b"data: [DONE]\n\n")
        self.wfile.flush()

    def _json(self, payload: dict[str, Any]) -> None:
        data = json.dumps(payload).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def main() -> int:
    args = parse_args()
    if args.bind not in {"127.0.0.1", "::1", "localhost"}:
        raise SystemExit("capture server must bind to a loopback address")
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text("", encoding="utf-8")
    server = CaptureServer((args.bind, args.port), output)
    print(f"capture listening on http://{args.bind}:{args.port}", flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
