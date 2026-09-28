#!/usr/bin/env python3
import http.server
import json
import socketserver
import threading
import time
import urllib.parse


PORT = 18083
DEFAULT_DELAY_MS = 2500

KEY_TO_CHANNEL = {
    "mock-key-a": "8801",
    "mock-key-b": "8802",
    "mock-key-c": "8803",
    "mock-key-ollama": "8804",
    "mock-key-commandcode": "8805",
}

lock = threading.Lock()
active_by_channel = {}
max_active_by_channel = {}
calls_by_channel = {}
statuses_by_channel = {}
max_active = 0
config = {"default_delay_ms": DEFAULT_DELAY_MS, "channels": {}}


def empty_counter():
    return {}


def reset_state():
    global active_by_channel, max_active_by_channel, calls_by_channel, statuses_by_channel, max_active, config
    with lock:
        active_by_channel = empty_counter()
        max_active_by_channel = empty_counter()
        calls_by_channel = empty_counter()
        statuses_by_channel = empty_counter()
        max_active = 0
        config = {"default_delay_ms": DEFAULT_DELAY_MS, "channels": {}}


def merge_config(value):
    global config
    if not isinstance(value, dict):
        raise ValueError("config must be a JSON object")
    with lock:
        default_delay = value.get("default_delay_ms")
        if default_delay is not None:
            config["default_delay_ms"] = max(0, int(default_delay))
        channels = value.get("channels")
        if channels is not None:
            if not isinstance(channels, dict):
                raise ValueError("config.channels must be a JSON object")
            for channel, channel_config in channels.items():
                if not isinstance(channel_config, dict):
                    raise ValueError("channel config must be a JSON object")
                current = config["channels"].setdefault(str(channel), {})
                if "delay_ms" in channel_config:
                    current["delay_ms"] = max(0, int(channel_config["delay_ms"]))
                if "status" in channel_config:
                    current["status"] = int(channel_config["status"])


def channel_config(channel):
    with lock:
        default_delay = config.get("default_delay_ms", DEFAULT_DELAY_MS)
        current = dict(config.get("channels", {}).get(channel, {}))
    current.setdefault("delay_ms", default_delay)
    current.setdefault("status", 0)
    return current


def record_start(channel):
    global max_active
    with lock:
        active_by_channel[channel] = active_by_channel.get(channel, 0) + 1
        current_active = active_by_channel[channel]
        max_active_by_channel[channel] = max(max_active_by_channel.get(channel, 0), current_active)
        max_active = max(max_active, sum(active_by_channel.values()))
        calls_by_channel[channel] = calls_by_channel.get(channel, 0) + 1


def record_end(channel, status):
    with lock:
        active_by_channel[channel] = max(0, active_by_channel.get(channel, 0) - 1)
        by_status = statuses_by_channel.setdefault(channel, {})
        by_status[str(status)] = by_status.get(str(status), 0) + 1


def snapshot():
    with lock:
        return {
            "max_active": max_active,
            "max_active_by_channel": dict(max_active_by_channel),
            "active_by_channel": dict(active_by_channel),
            "calls_by_channel": dict(calls_by_channel),
            "statuses_by_channel": {k: dict(v) for k, v in statuses_by_channel.items()},
            "config": json.loads(json.dumps(config)),
        }


def identify_channel(handler):
    auth = handler.headers.get("Authorization", "").strip()
    if auth.lower().startswith("bearer "):
        auth = auth[7:].strip()
    if auth in KEY_TO_CHANNEL:
        return KEY_TO_CHANNEL[auth]

    explicit = handler.headers.get("X-VPool-Mock-Channel", "").strip()
    if explicit:
        return explicit

    path = urllib.parse.urlparse(handler.path).path
    parts = [part for part in path.split("/") if part]
    if parts and parts[0].isdigit():
        return parts[0]
    return "unknown"


class Handler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):
        pass

    def read_body(self):
        length = int(self.headers.get("Content-Length", "0"))
        if length <= 0:
            return b""
        return self.rfile.read(length)

    def write_json(self, status, payload, extra_headers=None):
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        if extra_headers:
            for key, value in extra_headers.items():
                self.send_header(key, value)
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        path = urllib.parse.urlparse(self.path).path
        if path == "/stats":
            self.write_json(200, snapshot())
            return
        if path == "/control/state":
            self.write_json(200, snapshot())
            return
        self.send_error(404)

    def do_POST(self):
        path = urllib.parse.urlparse(self.path).path
        if path == "/control/reset":
            self.read_body()
            reset_state()
            self.write_json(200, snapshot())
            return
        if path == "/control/config":
            try:
                payload = json.loads(self.read_body() or b"{}")
                merge_config(payload)
            except Exception as exc:
                self.write_json(400, {"error": str(exc)})
                return
            self.write_json(200, snapshot())
            return
        self.handle_relay()

    def handle_relay(self):
        body = self.read_body()
        channel = identify_channel(self)
        current_config = channel_config(channel)
        status = int(current_config.get("status", 0))
        delay_ms = int(current_config.get("delay_ms", DEFAULT_DELAY_MS))
        record_start(channel)
        response_status = status if status >= 400 else 200
        try:
            if delay_ms > 0:
                time.sleep(delay_ms / 1000.0)
            if status >= 400:
                extra = {"Retry-After": "1"} if status == 429 else None
                self.write_json(
                    status,
                    {"error": {"message": "mock upstream status %d" % status, "type": "mock_error"}},
                    extra,
                )
                return
            try:
                parsed = json.loads(body or b"{}")
            except Exception:
                parsed = {}
            path = urllib.parse.urlparse(self.path).path
            if path.endswith("/responses"):
                self.write_json(
                    200,
                    {
                        "id": "resp_mock_%s_%d" % (channel, int(time.time() * 1000000)),
                        "object": "response",
                        "created": int(time.time()),
                        "model": "vpool-mock-upstream",
                        "output": [
                            {
                                "type": "message",
                                "role": "assistant",
                                "content": [{"type": "output_text", "text": "ok"}],
                            }
                        ],
                        "usage": {"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
                    },
                )
                return
            if parsed.get("stream"):
                self.write_sse(channel)
                return
            self.write_json(
                200,
                {
                    "id": "chatcmpl_mock_%s_%d" % (channel, int(time.time() * 1000000)),
                    "object": "chat.completion",
                    "created": int(time.time()),
                    "model": "vpool-mock-upstream",
                    "choices": [
                        {
                            "index": 0,
                            "message": {"role": "assistant", "content": "ok"},
                            "finish_reason": "stop",
                        }
                    ],
                    "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
                },
            )
        finally:
            record_end(channel, response_status)

    def write_sse(self, channel):
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "close")
        self.end_headers()
        chunk = {
            "id": "chatcmpl_mock_%s_%d" % (channel, int(time.time() * 1000000)),
            "object": "chat.completion.chunk",
            "created": int(time.time()),
            "model": "vpool-mock-upstream",
            "choices": [
                {
                    "index": 0,
                    "delta": {"role": "assistant", "content": "ok"},
                    "finish_reason": None,
                }
            ],
        }
        self.wfile.write(b"data: " + json.dumps(chunk).encode() + b"\n\n")
        self.wfile.write(b"data: [DONE]\n\n")
        self.wfile.flush()


class ThreadingServer(socketserver.ThreadingMixIn, http.server.HTTPServer):
    daemon_threads = True


if __name__ == "__main__":
    ThreadingServer(("127.0.0.1", PORT), Handler).serve_forever()
