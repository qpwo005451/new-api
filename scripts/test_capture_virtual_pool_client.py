#!/usr/bin/env python3
"""Contract tests for the local virtual-pool client capture server."""

from __future__ import annotations

import importlib.util
import json
import tempfile
import threading
import unittest
import urllib.request
from pathlib import Path


SCRIPT = Path(__file__).with_name("capture_virtual_pool_client.py")
SPEC = importlib.util.spec_from_file_location("capture_virtual_pool_client", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
capture = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(capture)


class CaptureVirtualPoolClientTest(unittest.TestCase):
    def test_records_presence_without_session_secrets(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            output = Path(temp_dir) / "capture.jsonl"
            output.write_text("", encoding="utf-8")
            server = capture.CaptureServer(("127.0.0.1", 0), output)
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                request = urllib.request.Request(
                    f"http://127.0.0.1:{server.server_port}/v1/chat/completions",
                    data=json.dumps({
                        "model": "deepseek v4.1 flash",
                        "session_id": "session-secret-must-not-appear",
                        "prompt_cache_key": "cache-secret-must-not-appear",
                    }).encode("utf-8"),
                    headers={
                        "Content-Type": "application/json",
                        "Authorization": "Bearer secret",
                        "X-NewAPI-Session-ID": "affinity-secret-must-not-appear",
                    },
                    method="POST",
                )
                with urllib.request.urlopen(request, timeout=5) as response:
                    self.assertEqual(200, response.status)
                thread.join(timeout=2)
                records = [json.loads(line) for line in output.read_text(encoding="utf-8").splitlines()]
                self.assertEqual(1, len(records))
                body = records[0]["body"]
                self.assertTrue(body["session_id_present"])
                self.assertEqual("str", body["session_id_type"])
                self.assertTrue(body["prompt_cache_key_present"])
                self.assertEqual("str", body["prompt_cache_key_type"])
                self.assertEqual("<redacted>", records[0]["headers"]["Authorization"])
                affinity = next(
                    value
                    for name, value in records[0]["headers"].items()
                    if name.lower() == "x-newapi-session-id"
                )
                self.assertTrue(affinity["present"])
                self.assertNotIn("affinity-secret-must-not-appear", output.read_text(encoding="utf-8"))
                self.assertNotIn("secret", output.read_text(encoding="utf-8"))
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)


if __name__ == "__main__":
    unittest.main()
