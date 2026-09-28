#!/usr/bin/env python3
import argparse
import json
import time
from dataclasses import dataclass, field
from typing import Any

import requests


RUN_TAG = "model-health-4003"
SHARED_CHANNEL_NAME = RUN_TAG + "-shared-model-channel"
SOLO_CHANNEL_NAME = RUN_TAG + "-solo-model-channel"
BASE_MODEL = "deepseek-v4.1-flash"
SOLO_MODEL = "glm-5.3-flash"


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
        self.admin = requests.Session()
        self.admin.headers.update({"Authorization": "Bearer " + args.admin_token})
        self.relay = requests.Session()
        self.relay.headers.update({"Authorization": "Bearer " + args.relay_token})
        self.reports: list[TestReport] = []
        self.shared_channel_id = 0
        self.solo_channel_id = 0
        self.initial_affinity_rules = ""

    def api(self, method: str, path: str, **kwargs: Any) -> requests.Response:
        return self.admin.request(
            method,
            self.base_url + path,
            timeout=kwargs.pop("timeout", 20),
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

    def mock(self, method: str, path: str, **kwargs: Any) -> requests.Response:
        return requests.request(
            method,
            self.mock_url + path,
            timeout=kwargs.pop("timeout", 10),
            **kwargs,
        )

    def find_test_channel(self, name: str) -> int:
        response = self.api("GET", "/api/channel/search", params={"keyword": name, "page": 1, "page_size": 100})
        response.raise_for_status()
        items = response.json()["data"]["items"]
        return int(items[0]["id"]) if items else 0

    def install_affinity_bypass(self) -> None:
        options = self.options()
        self.initial_affinity_rules = options.get("channel_affinity_setting.rules", "[]")
        self.update_option("channel_affinity_setting.rules", "[]")

    def find_channels(self) -> None:
        self.shared_channel_id = self.find_test_channel(SHARED_CHANNEL_NAME)
        self.solo_channel_id = self.find_test_channel(SOLO_CHANNEL_NAME)

    def create_channels(self) -> None:
        common = {
            "type": 1,
            "status": 1,
            "base_url": self.mock_url,
            "group": self.args.group,
            "priority": 700,
            "auto_ban": 0,
            "models": BASE_MODEL + "," + SOLO_MODEL,
        }
        shared_payload = dict(common, name=SHARED_CHANNEL_NAME, key=self.args.mock_key)
        solo_payload = dict(common, name=SOLO_CHANNEL_NAME, key=self.args.mock_key)
        response = self.api("POST", "/api/channel/", json={"mode": "single", "channel": shared_payload})
        response.raise_for_status()
        payload = response.json()
        self.require(payload.get("success"), "failed to create shared mock channel: " + json.dumps(payload))

        response = self.api("POST", "/api/channel/", json={"mode": "single", "channel": solo_payload})
        response.raise_for_status()
        payload = response.json()
        self.require(payload.get("success"), "failed to create solo mock channel: " + json.dumps(payload))

        self.find_channels()
        self.require(self.shared_channel_id > 0 and self.solo_channel_id > 0, "created mock channels are not visible")

    def update_shared_model(self, model: str) -> None:
        response = self.api(
            "GET",
            "/api/channel/" + str(self.shared_channel_id),
        )
        response.raise_for_status()
        channel = response.json()["data"]
        channel["models"] = model
        # The general update endpoint treats status as an operational-only field.
        channel.pop("status", None)
        response = self.api("PUT", "/api/channel/", json=channel)
        response.raise_for_status()
        payload = response.json()
        self.require(payload.get("success"), "failed to update shared channel model: " + json.dumps(payload))

    def delete_channels(self) -> None:
        for channel_id in (self.solo_channel_id, self.shared_channel_id):
            if not channel_id:
                continue
            try:
                response = self.api("DELETE", "/api/channel/" + str(channel_id))
                response.raise_for_status()
                payload = response.json()
                if not payload.get("success"):
                    print("WARN failed to delete channel", channel_id, payload)
            except Exception as exc:
                print("WARN failed to delete channel", channel_id, exc)

    def restore_affinity(self) -> None:
        if self.initial_affinity_rules:
            try:
                self.update_option("channel_affinity_setting.rules", self.initial_affinity_rules)
            except Exception as exc:
                print("WARN failed to restore channel affinity rules:", exc)

    def mock_reset(self, config: dict[str, Any] | None = None) -> dict[str, Any]:
        self.mock("POST", "/control/reset", json={}).raise_for_status()
        if config:
            response = self.mock("POST", "/control/config", json=config)
            response.raise_for_status()
        return self.mock_get_stats()

    def mock_config(self, config: dict[str, Any]) -> dict[str, Any]:
        response = self.mock("POST", "/control/config", json=config)
        response.raise_for_status()
        return response.json()

    def mock_get_stats(self) -> dict[str, Any]:
        response = self.mock("GET", "/stats")
        response.raise_for_status()
        return response.json()

    def calls(self, stats: dict[str, Any]) -> dict[str, int]:
        return {
            str(self.shared_channel_id): int(stats["calls_by_channel"].get(str(self.shared_channel_id), 0)),
            str(self.solo_channel_id): int(stats["calls_by_channel"].get(str(self.solo_channel_id), 0)),
        }

    def chat(self, model: str, timeout: float = 15.0) -> tuple[int, str]:
        response = self.relay.post(
            self.base_url + "/v1/chat/completions",
            json={"model": model, "messages": [{"role": "user", "content": "ping"}]},
            timeout=timeout,
        )
        return response.status_code, response.text

    def record(self, name: str, condition: bool, detail: dict[str, Any], error: str = "") -> None:
        self.reports.append(TestReport(name, condition, detail, error))

    def require(self, condition: bool, message: str) -> None:
        if not condition:
            raise AssertionError(message)

    def install_policy(self) -> None:
        rules = [
            {
                "name": "ordinary deepseek and glm cooldown",
                "enabled": True,
                "models": [BASE_MODEL, SOLO_MODEL],
                "failure_threshold": 2,
                "cooldown_seconds": 2,
                "status_codes": [429, 500, 502, 503, 504],
                "groups": [self.args.group],
            }
        ]
        enabled = self.update_option("model_health_policy_setting.enabled", "true")
        saved = self.update_option(
            "model_health_policy_setting.rules",
            json.dumps(rules, separators=(",", ":")),
        )
        self.record(
            "H01-save-policy",
            bool(enabled.get("success")) and bool(saved.get("success")),
            {"enabled": enabled, "rules": saved},
        )
        stored = self.options()
        parsed = json.loads(stored["model_health_policy_setting.rules"])
        self.record(
            "H02-read-policy",
            stored["model_health_policy_setting.enabled"] == "true"
            and parsed[0]["models"] == [BASE_MODEL, SOLO_MODEL],
            {"enabled": stored["model_health_policy_setting.enabled"], "rules": parsed},
        )

    def case_shared_model_cooldowns(self) -> dict[str, Any]:
        self.update_shared_model(BASE_MODEL)
        stats = self.mock_reset(
            {
                "default_delay_ms": 0,
                "channels": {
                    str(self.shared_channel_id): {"status": 503},
                    str(self.solo_channel_id): {"status": 0},
                },
            }
        )
        self.require(self.calls(stats)[str(self.shared_channel_id)] == 0, "mock counters must start clean")

        for _ in range(30):
            self.chat(BASE_MODEL)
            stats = self.mock_get_stats()
            if self.calls(stats)[str(self.shared_channel_id)] >= 2:
                break
        calls_before = self.calls(stats)
        self.require(
            calls_before[str(self.shared_channel_id)] == 2,
            "shared channel must reach threshold after two attempts, got " + json.dumps(calls_before),
        )
        self.require(calls_before[str(self.solo_channel_id)] > 0, "second attempt must fail over to healthy solo channel")

        for _ in range(3):
            self.chat(BASE_MODEL)
            stats = self.mock_get_stats()
        calls_during = self.calls(stats)
        self.require(
            calls_during[str(self.shared_channel_id)] == 2,
            "cooling channel must be skipped for the same model, got " + json.dumps(calls_during),
        )
        self.require(calls_during[str(self.solo_channel_id)] > calls_before[str(self.solo_channel_id)], "healthy channel must serve during cooldown")
        return {"before": calls_before, "during": calls_during}

    def case_same_channel_other_model(self) -> dict[str, Any]:
        self.update_shared_model(BASE_MODEL + "," + SOLO_MODEL)
        time.sleep(0.2)
        self.mock_config({"channels": {str(self.shared_channel_id): {"status": 503}}})
        status, body = self.chat(SOLO_MODEL)
        stats = self.mock_get_stats()
        self.require(status in (200, 503), "solo model request must reach mock upstream, got " + str(status) + " " + body[:300])
        self.require(
            self.calls(stats)[str(self.shared_channel_id)] > 0,
            "same channel remains selectable for a model that is not cooling",
        )
        return {"status": status, "calls": self.calls(stats)}

    def case_recovery(self) -> dict[str, Any]:
        self.update_shared_model(BASE_MODEL)
        self.mock_reset(
            {
                "default_delay_ms": 0,
                "channels": {
                    str(self.shared_channel_id): {"status": 429},
                    str(self.solo_channel_id): {"status": 0},
                },
            }
        )
        for _ in range(30):
            self.chat(BASE_MODEL)
            stats = self.mock_get_stats()
            if self.calls(stats)[str(self.shared_channel_id)] >= 2:
                break
        calls_before = self.calls(stats)
        self.require(calls_before[str(self.shared_channel_id)] == 2, "recovery test failed to reach threshold: " + json.dumps(calls_before))

        self.mock_config({"channels": {str(self.shared_channel_id): {"status": 0}}})
        status, body = self.chat(BASE_MODEL)
        stats = self.mock_get_stats()
        self.require(status == 200, "cooldown should still be active, got " + str(status) + " " + body[:300])
        self.require(self.calls(stats)[str(self.shared_channel_id)] == 2, "channel must stay cooling after fewer than two seconds")

        time.sleep(2.2)
        status, body = self.chat(BASE_MODEL)
        stats = self.mock_get_stats()
        self.require(status == 200, "recovered call should succeed, got " + str(status) + " " + body[:300])
        self.require(self.calls(stats)[str(self.shared_channel_id)] > 2, "channel must be retried after cooldown expiry")
        return {"before": calls_before, "recovered_status": status, "calls": self.calls(stats)}

    def run(self) -> None:
        print("candidate", self.base_url, "mock", self.mock_url)
        self.create_channels()
        self.install_affinity_bypass()
        self.install_policy()
        for name, function in [
            ("H03-ordinary-shared-model-cooldown", self.case_shared_model_cooldowns),
            ("H04-same-channel-other-model", self.case_same_channel_other_model),
            ("H05-model-channel-recovers", self.case_recovery),
        ]:
            started = time.monotonic()
            try:
                detail = function()
                self.record(name, True, {"elapsed_ms": int((time.monotonic() - started) * 1000), **detail})
            except Exception as exc:
                print("FAIL", name, "->", exc, flush=True)
                self.record(name, False, {"elapsed_ms": int((time.monotonic() - started) * 1000)}, str(exc))


def run(args: argparse.Namespace) -> tuple[int, int]:
    harness = Harness(args)
    options = harness.options()
    original_enabled = options.get("model_health_policy_setting.enabled", "false")
    original_rules = options.get("model_health_policy_setting.rules", "[]")
    try:
        harness.run()
    finally:
        try:
            harness.update_option("model_health_policy_setting.enabled", original_enabled)
            harness.update_option("model_health_policy_setting.rules", original_rules)
            harness.restore_affinity()
        finally:
            harness.delete_channels()
    passed = sum(1 for report in harness.reports if report.passed)
    failed = sum(1 for report in harness.reports if not report.passed)
    return passed, failed


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="http://127.0.0.1:4003")
    parser.add_argument("--mock-url", default="http://127.0.0.1:18083")
    parser.add_argument("--mock-key", default="mock-key-shared")
    parser.add_argument("--group", default="svip")
    parser.add_argument("--admin-token", required=True)
    parser.add_argument("--relay-token", required=True)
    parser.add_argument("--report", default="")
    args = parser.parse_args()
    passed, failed = run(args)
    payload = {"passed": passed, "failed": failed}
    print(json.dumps(payload, ensure_ascii=False, indent=2))
    if args.report:
        with open(args.report, "w", encoding="utf-8") as handle:
            handle.write(json.dumps(payload, ensure_ascii=False, indent=2) + "\n")
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
