#!/usr/bin/env python3
"""Per-model channel weight verification against the isolated 4003 candidate.

Runs entirely against mock upstreams and mock channels (fake keys), so it never
contacts a real provider. It drives traffic through the candidate and reads the
resulting distribution from the mock upstream call counters.

Assertions:
  A01 same channel, different models use independent weights
  A02 (paired with A01) the opposite channel dominates for the other model
  A03 models without an override fall back to channel.weight
  A04 an explicit weight: 0 override receives no traffic
  A05 real model names honor the override and stay on the mock upstream
  A06 the distribution follows a live weight update (cache invalidation)
"""
import argparse
import json
import subprocess
import time
from dataclasses import dataclass, field
from typing import Any

import requests

RUN_TAG = "model-weight-4003"
CHANNEL_A_NAME = RUN_TAG + "-channel-a"
CHANNEL_B_NAME = RUN_TAG + "-channel-b"
MOCK_A = "8801"
MOCK_B = "8802"
MODEL_A = "per-model-weight-test-a"
MODEL_B = "per-model-weight-test-b"
MODEL_C = "per-model-weight-test-c"
MODEL_D = "per-model-weight-test-d"
REAL_A = "deepseek-v4.1-flash"
REAL_B = "glm-5.3-flash"
ALL_MODELS = [MODEL_A, MODEL_B, MODEL_C, MODEL_D, REAL_A, REAL_B]


@dataclass
class TestReport:
    name: str
    passed: bool
    detail: dict[str, Any] = field(default_factory=dict)
    error: str = ""


class Harness:
    def __init__(self, args: argparse.Namespace) -> None:
        self.args = args
        self.base_url = args.base_url.rstrip("/")
        self.mock_url = args.mock_url.rstrip("/")
        self.admin_token = args.admin_token or self.read_token(
            "select access_token from users where role >= 10 "
            "and access_token is not null and length(access_token) > 0 order by id limit 1;"
        )
        self.relay_token = args.relay_token or self.read_token(
            "select key from tokens where status = 1 order by id limit 1;"
        )
        self.admin = requests.Session()
        self.admin.headers.update({"Authorization": "Bearer " + self.admin_token})
        self.relay = requests.Session()
        self.relay.headers.update({"Authorization": "Bearer " + self.relay_token})
        self.reports: list[TestReport] = []
        self.channel_a_id = 0
        self.channel_b_id = 0
        self.initial_weights = "[]"
        self.initial_affinity_enabled = ""
        self.initial_affinity_rules = ""
        self.monitor_probe_original = ""
        self.monitor_original = ""

    def read_token(self, query: str) -> str:
        if not self.args.db_path:
            raise RuntimeError("either a token or --db-path is required")
        output = subprocess.check_output(
            ["sqlite3", "-cmd", ".timeout 5000", "-noheader", "-batch", self.args.db_path, query],
            text=True,
        ).strip()
        if not output:
            raise RuntimeError("no usable token found in " + self.args.db_path)
        return output

    def api(self, method: str, path: str, **kwargs: Any) -> requests.Response:
        return self.admin.request(
            method,
            self.base_url + path,
            timeout=kwargs.pop("timeout", 30),
            **kwargs,
        )

    def options(self) -> dict[str, str]:
        response = self.api("GET", "/api/option/")
        response.raise_for_status()
        payload = response.json()
        if not payload.get("success"):
            raise RuntimeError("option read failed: " + json.dumps(payload))
        return {item["key"]: item["value"] for item in payload["data"]}

    def update_option(self, key: str, value: str) -> dict[str, Any]:
        response = self.api("PUT", "/api/option/", json={"key": key, "value": value})
        response.raise_for_status()
        return response.json()

    def update_request_policy(self, options: dict[str, str]) -> dict[str, Any]:
        response = self.api("PATCH", "/api/option/request_policy", json={"options": options})
        response.raise_for_status()
        return response.json()

    def set_weights(self, entries: list[dict[str, Any]]) -> dict[str, Any]:
        return self.update_request_policy(
            {"model_weight_setting.weights": json.dumps(entries, separators=(",", ":"))}
        )

    def mock(self, method: str, path: str, **kwargs: Any) -> requests.Response:
        return requests.request(
            method,
            self.mock_url + path,
            timeout=kwargs.pop("timeout", 20),
            **kwargs,
        )

    def mock_reset(self) -> dict[str, Any]:
        self.mock("POST", "/control/reset", json={}).raise_for_status()
        return self.mock("POST", "/control/config", json={"default_delay_ms": 0}).json()

    def mock_stats(self) -> dict[str, Any]:
        response = self.mock("GET", "/stats")
        response.raise_for_status()
        return response.json()

    def hits(self, stats: dict[str, Any] | None = None) -> dict[str, int]:
        stats = stats or self.mock_stats()
        return {
            "a": int(stats["calls_by_channel"].get(MOCK_A, 0)),
            "b": int(stats["calls_by_channel"].get(MOCK_B, 0)),
        }

    def find_test_channel(self, name: str) -> int:
        response = self.api(
            "GET",
            "/api/channel/search",
            params={"keyword": name, "page": 1, "page_size": 100},
        )
        response.raise_for_status()
        items = response.json()["data"]["items"]
        return int(items[0]["id"]) if items else 0

    def create_channels(self) -> None:
        for name in (CHANNEL_A_NAME, CHANNEL_B_NAME):
            existing = self.find_test_channel(name)
            if existing:
                self.api("DELETE", "/api/channel/" + str(existing))
        common = {
            "type": 1,
            "status": 1,
            "base_url": self.mock_url,
            "group": self.args.group,
            "priority": self.args.priority,
            "weight": 0,
            "auto_ban": 0,
            "models": ",".join(ALL_MODELS),
        }
        for name, key in ((CHANNEL_A_NAME, "mock-key-a"), (CHANNEL_B_NAME, "mock-key-b")):
            payload = dict(common, name=name, key=key)
            response = self.api("POST", "/api/channel/", json={"mode": "single", "channel": payload})
            response.raise_for_status()
            body = response.json()
            self.require(body.get("success"), "failed to create mock channel %s: %s" % (name, body))
        self.channel_a_id = self.find_test_channel(CHANNEL_A_NAME)
        self.channel_b_id = self.find_test_channel(CHANNEL_B_NAME)
        self.require(
            self.channel_a_id > 0 and self.channel_b_id > 0,
            "created mock channels are not visible",
        )

    def delete_channels(self) -> None:
        for channel_id in (self.channel_a_id, self.channel_b_id):
            if not channel_id:
                continue
            try:
                response = self.api("DELETE", "/api/channel/" + str(channel_id))
                if not response.json().get("success"):
                    print("WARN failed to delete channel", channel_id, response.text)
            except Exception as exc:  # noqa: BLE001
                print("WARN failed to delete channel", channel_id, exc)

    def quiet_background_jobs(self) -> None:
        options = self.options()
        self.initial_weights = options.get("model_weight_setting.weights", "[]")
        self.initial_affinity_enabled = options.get("channel_affinity_setting.enabled", "")
        self.initial_affinity_rules = options.get("channel_affinity_setting.rules", "[]")
        self.monitor_probe_original = options.get("model_monitor_setting.auto_probe_enabled", "")
        self.monitor_original = options.get("model_monitor_setting.enabled", "")
        self.update_option("model_monitor_setting.auto_probe_enabled", "false")
        self.update_option("model_monitor_setting.enabled", "false")
        self.update_option("model_health_policy_setting.enabled", "false")
        # Session affinity would pin repeated requests to one channel and skew
        # the measured distribution.
        self.update_option("channel_affinity_setting.enabled", "false")
        self.update_option("channel_affinity_setting.rules", "[]")

    def restore(self) -> None:
        try:
            # The option read can come back as an empty string when the key has
            # no stored row yet; fall back to the registered empty-array value
            # so a previous run's overrides never leak into the next one.
            self.update_request_policy(
                {"model_weight_setting.weights": self.initial_weights or "[]"}
            )
        except Exception as exc:  # noqa: BLE001
            print("WARN failed to restore model weights:", exc)
        for key, value in (
            ("channel_affinity_setting.enabled", self.initial_affinity_enabled),
            ("channel_affinity_setting.rules", self.initial_affinity_rules),
            ("model_monitor_setting.auto_probe_enabled", self.monitor_probe_original),
            ("model_monitor_setting.enabled", self.monitor_original),
        ):
            if value == "":
                continue
            try:
                self.update_option(key, value)
            except Exception as exc:  # noqa: BLE001
                print("WARN failed to restore", key, exc)

    def force_cache_refresh(self) -> None:
        # AddChannel only inserts into the database; the running candidate
        # refreshes its in-memory channel cache on UpdateChannel (PUT) and on
        # the periodic sync. Re-saving each mock channel makes it selectable
        # immediately instead of waiting for the 60s sync.
        for channel_id in (self.channel_a_id, self.channel_b_id):
            response = self.api("GET", "/api/channel/" + str(channel_id))
            response.raise_for_status()
            channel = response.json()["data"]
            # The general update endpoint treats status as an operational-only
            # field, so drop it to avoid an unintended status change.
            channel.pop("status", None)
            response = self.api("PUT", "/api/channel/", json=channel)
            response.raise_for_status()
            self.require(
                response.json().get("success"),
                "failed to refresh the channel cache for %d" % channel_id,
            )

    def assert_mock_only(self) -> None:
        response = self.api("GET", "/api/channel/", params={"p": 1, "page_size": 1000})
        response.raise_for_status()
        items = response.json()["data"]["items"]
        enabled_real = [
            (int(channel["id"]), channel["name"])
            for channel in items
            if not str(channel["name"]).startswith(RUN_TAG) and channel.get("status") == 1
        ]
        self.require(
            not enabled_real,
            "refusing to send traffic: real channels are still enabled: " + json.dumps(enabled_real),
        )

    def preflight(self) -> None:
        # Prove the mock channels are selectable for the test models before the
        # measured runs. If the cache is stale the request fails locally (all
        # real channels are disabled), so this can never reach a real upstream.
        self.mock_reset()
        status = self.chat(MODEL_A)
        hits = self.hits()
        self.require(
            hits["a"] + hits["b"] == 1,
            "mock channel preflight failed: status=%s hits=%s" % (status, hits),
        )

    def chat(self, model: str) -> int:
        response = self.relay.post(
            self.base_url + "/v1/chat/completions",
            json={"model": model, "messages": [{"role": "user", "content": "ping"}], "max_tokens": 1},
            timeout=self.args.request_timeout,
        )
        return response.status_code

    def drive(self, model: str, count: int) -> tuple[dict[str, int], dict[int, int]]:
        # Reset the mock counters so the returned hits belong to this model only.
        self.mock_reset()
        statuses: dict[int, int] = {}
        for _ in range(count):
            status = self.chat(model)
            statuses[status] = statuses.get(status, 0) + 1
        return self.hits(), statuses

    def require(self, condition: bool, message: str) -> None:
        if not condition:
            raise AssertionError(message)

    def record(self, name: str, condition: bool, detail: dict[str, Any], error: str = "") -> None:
        self.reports.append(TestReport(name, condition, detail, error))

    def run(self) -> None:
        print("candidate", self.base_url, "mock", self.mock_url, flush=True)
        self.assert_mock_only()
        self.create_channels()
        self.force_cache_refresh()
        self.assert_mock_only()
        self.quiet_background_jobs()
        self.preflight()
        self.require(self.channel_a_id != self.channel_b_id, "mock channels share an id")

        initial = [
            {"channel_id": self.channel_a_id, "model": MODEL_A, "weight": 100},
            {"channel_id": self.channel_b_id, "model": MODEL_A, "weight": 1},
            {"channel_id": self.channel_a_id, "model": MODEL_B, "weight": 1},
            {"channel_id": self.channel_b_id, "model": MODEL_B, "weight": 100},
            {"channel_id": self.channel_a_id, "model": MODEL_D, "weight": 100},
            {"channel_id": self.channel_b_id, "model": MODEL_D, "weight": 0},
            {"channel_id": self.channel_a_id, "model": REAL_A, "weight": 100},
            {"channel_id": self.channel_b_id, "model": REAL_A, "weight": 1},
            {"channel_id": self.channel_a_id, "model": REAL_B, "weight": 1},
            {"channel_id": self.channel_b_id, "model": REAL_B, "weight": 100},
        ]
        saved = self.set_weights(initial)
        self.record("A00-save-weights", bool(saved.get("success")), {"response": saved})

        n = self.args.requests
        hits_a, statuses_a = self.drive(MODEL_A, n)
        self.record(
            "A01-model-a-prefers-channel-a",
            hits_a["a"] > 0.95 * n,
            {"model": MODEL_A, "hits": hits_a, "requests": n, "statuses": statuses_a},
        )

        hits_b, statuses_b = self.drive(MODEL_B, n)
        self.record(
            "A02-model-b-prefers-channel-b",
            hits_b["b"] > 0.95 * n,
            {"model": MODEL_B, "hits": hits_b, "requests": n, "statuses": statuses_b},
        )

        hits_c, statuses_c = self.drive(MODEL_C, n)
        share_a = hits_c["a"] / max(1, hits_c["a"] + hits_c["b"])
        self.record(
            "A03-unset-model-falls-back-to-channel-weight",
            0.3 < share_a < 0.7,
            {"model": MODEL_C, "hits": hits_c, "share_a": round(share_a, 4), "statuses": statuses_c},
        )

        hits_d, statuses_d = self.drive(MODEL_D, n)
        self.record(
            "A04-explicit-zero-gets-no-traffic",
            hits_d["b"] == 0 and hits_d["a"] == n,
            {"model": MODEL_D, "hits": hits_d, "requests": n, "statuses": statuses_d},
        )

        real_n = self.args.real_requests
        hits_ra, statuses_ra = self.drive(REAL_A, real_n)
        self.record(
            "A05-real-model-deepseek-honors-override",
            hits_ra["a"] > 0.95 * real_n,
            {"model": REAL_A, "hits": hits_ra, "requests": real_n, "statuses": statuses_ra},
        )
        hits_rb, statuses_rb = self.drive(REAL_B, real_n)
        self.record(
            "A06-real-model-glm-honors-override",
            hits_rb["b"] > 0.95 * real_n,
            {"model": REAL_B, "hits": hits_rb, "requests": real_n, "statuses": statuses_rb},
        )

        swapped = [
            {"channel_id": self.channel_a_id, "model": MODEL_A, "weight": 1},
            {"channel_id": self.channel_b_id, "model": MODEL_A, "weight": 100},
        ]
        saved = self.set_weights(swapped)
        self.record("A07-save-swapped-weights", bool(saved.get("success")), {"response": saved})
        hits_swap, statuses_swap = self.drive(MODEL_A, n)
        self.record(
            "A08-live-update-changes-distribution",
            hits_swap["b"] > 0.95 * n,
            {"model": MODEL_A, "hits": hits_swap, "requests": n, "statuses": statuses_swap},
        )


def run(args: argparse.Namespace) -> tuple[int, int, list[TestReport]]:
    harness = Harness(args)
    try:
        harness.run()
    finally:
        harness.restore()
        harness.delete_channels()
    passed = sum(1 for report in harness.reports if report.passed)
    failed = sum(1 for report in harness.reports if not report.passed)
    return passed, failed, harness.reports


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="http://127.0.0.1:4003")
    parser.add_argument("--mock-url", default="http://127.0.0.1:18083")
    parser.add_argument("--group", default="svip")
    parser.add_argument("--priority", type=int, default=100000)
    parser.add_argument("--requests", type=int, default=400)
    parser.add_argument("--real-requests", type=int, default=100)
    parser.add_argument("--request-timeout", type=float, default=15.0)
    parser.add_argument("--db-path", default="")
    parser.add_argument("--admin-token", default="")
    parser.add_argument("--relay-token", default="")
    parser.add_argument("--report", default="")
    args = parser.parse_args()
    started = time.monotonic()
    passed, failed, reports = run(args)
    payload = {
        "passed": passed,
        "failed": failed,
        "elapsed_s": round(time.monotonic() - started, 2),
        "reports": [report.__dict__ for report in reports],
    }
    print(json.dumps(payload, ensure_ascii=False, indent=2))
    if args.report:
        with open(args.report, "w", encoding="utf-8") as handle:
            handle.write(json.dumps(payload, ensure_ascii=False, indent=2) + "\n")
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
