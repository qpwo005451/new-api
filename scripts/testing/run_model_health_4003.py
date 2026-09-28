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
SHARED_MOCK_CHANNEL = "8801"
SOLO_MOCK_CHANNEL = "8802"
BASE_MODEL = "deepseek-v4.1-flash"
SOLO_MODEL = "glm-5.3-flash"
# Recovery uses a dedicated model name so its per-(model, channel) health state
# is untouched by the earlier threshold cases.
RECOVERY_MODEL = "deepseek-recovery-test"


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
            "models": BASE_MODEL + "," + SOLO_MODEL + "," + RECOVERY_MODEL,
        }
        shared_payload = dict(common, name=SHARED_CHANNEL_NAME, key=self.args.shared_mock_key)
        solo_payload = dict(common, name=SOLO_CHANNEL_NAME, key=self.args.solo_mock_key)
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
            str(self.shared_channel_id): int(stats["calls_by_channel"].get(SHARED_MOCK_CHANNEL, 0)),
            str(self.solo_channel_id): int(stats["calls_by_channel"].get(SOLO_MOCK_CHANNEL, 0)),
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
        # Background model-monitor probes relay real requests to the mock and
        # record model-health failures for the same (model, channel) keys, so
        # they must stay off while the harness drives all traffic.
        self.monitor_probe_original = self.options().get(
            "model_monitor_setting.auto_probe_enabled", "true"
        )
        self.monitor_original = self.options().get("model_monitor_setting.enabled", "true")
        self.update_option("model_monitor_setting.auto_probe_enabled", "false")
        self.update_option("model_monitor_setting.enabled", "false")
        rules = [
            {
                "name": "ordinary deepseek and glm cooldown",
                "enabled": True,
                "models": [BASE_MODEL, SOLO_MODEL, RECOVERY_MODEL],
                "failure_threshold": 2,
                # Six seconds keeps the cooldown window comfortably wider than
                # the per-request latency of the during-cooldown checks.
                "cooldown_seconds": 6,
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
            and parsed[0]["models"] == [BASE_MODEL, SOLO_MODEL, RECOVERY_MODEL],
            {"enabled": stored["model_health_policy_setting.enabled"], "rules": parsed},
        )
        # Wait out any cooldowns left in the candidate's in-memory health store
        # by an earlier aborted run before the threshold cases begin.
        time.sleep(7)

    def wait_for_shared_calls(self, model: str, target: int, deadline_s: float, spacing: float = 0.5) -> dict[str, Any]:
        """Send chat requests until the shared mock channel has served `target`
        calls or the deadline passes. A failing channel cools down for the
        policy cooldown after each attempt, so reaching a second hit can take
        more requests than a fixed iteration budget allows.
        """
        deadline = time.monotonic() + deadline_s
        stats = self.mock_get_stats()
        while self.calls(stats)[str(self.shared_channel_id)] < target and time.monotonic() < deadline:
            self.chat(model)
            time.sleep(spacing)
            stats = self.mock_get_stats()
        return stats

    def case_shared_model_cooldowns(self) -> dict[str, Any]:
        self.update_shared_model(BASE_MODEL)
        stats = self.mock_reset(
            {
                "default_delay_ms": 0,
                "channels": {
                    SHARED_MOCK_CHANNEL: {"status": 503},
                    SOLO_MOCK_CHANNEL: {"status": 0},
                },
            }
        )
        self.require(self.calls(stats)[str(self.shared_channel_id)] == 0, "mock counters must start clean")

        stats = self.wait_for_shared_calls(BASE_MODEL, 2, 45)
        calls_before = self.calls(stats)
        self.require(
            calls_before[str(self.shared_channel_id)] == 2,
            "shared channel must reach threshold after two attempts, got " + json.dumps(calls_before),
        )
        self.require(calls_before[str(self.solo_channel_id)] > 0, "second attempt must fail over to healthy solo channel")

        for _ in range(3):
            self.chat(BASE_MODEL)
            time.sleep(0.8)
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
        self.mock_config({"channels": {SHARED_MOCK_CHANNEL: {"status": 503}}})
        # Selection inside the top priority tier is weighted-random, so keep
        # sending until the shared channel is actually exercised.
        baseline = self.calls(self.mock_get_stats())[str(self.shared_channel_id)]
        status = 0
        body = ""
        deadline = time.monotonic() + 45
        while True:
            status, body = self.chat(SOLO_MODEL)
            stats = self.mock_get_stats()
            if self.calls(stats)[str(self.shared_channel_id)] > baseline or time.monotonic() >= deadline:
                break
        self.require(
            self.calls(stats)[str(self.shared_channel_id)] > baseline,
            "same channel remains selectable for a model that is not cooling, got " + json.dumps(self.calls(stats)),
        )
        self.require(status in (200, 503), "solo model request must reach mock upstream, got " + str(status) + " " + body[:300])
        stats = self.mock_get_stats()
        return {"status": status, "calls": self.calls(stats)}

    def case_recovery(self) -> dict[str, Any]:
        # Keep the recovery model on both channels regardless of earlier cases.
        self.update_shared_model(BASE_MODEL + "," + SOLO_MODEL + "," + RECOVERY_MODEL)
        self.mock_reset(
            {
                "default_delay_ms": 0,
                "channels": {
                    SHARED_MOCK_CHANNEL: {"status": 429},
                    SOLO_MOCK_CHANNEL: {"status": 0},
                },
            }
        )
        stats = self.wait_for_shared_calls(RECOVERY_MODEL, 2, 45)
        calls_before = self.calls(stats)
        self.require(calls_before[str(self.shared_channel_id)] == 2, "recovery test failed to reach threshold: " + json.dumps(calls_before))

        self.mock_config({"channels": {SHARED_MOCK_CHANNEL: {"status": 0}}})
        status, body = self.chat(RECOVERY_MODEL)
        stats = self.mock_get_stats()
        self.require(status == 200, "cooldown should still be active, got " + str(status) + " " + body[:300])
        self.require(self.calls(stats)[str(self.shared_channel_id)] == 2, "channel must stay cooling after fewer than two seconds")

        time.sleep(7.2)
        for _ in range(15):
            status, body = self.chat(RECOVERY_MODEL)
            stats = self.mock_get_stats()
            if self.calls(stats)[str(self.shared_channel_id)] > 2:
                break
        self.require(status == 200, "recovered call should succeed, got " + str(status) + " " + body[:300])
        self.require(
            self.calls(stats)[str(self.shared_channel_id)] > 2,
            "channel must be retried after cooldown expiry, got " + json.dumps(self.calls(stats)),
        )
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
    # The candidate database is test-only: always restore a clean baseline
    # instead of echoing back whatever a previously aborted run left behind
    # (stale 300s cooldown policies used to survive across runs this way).
    try:
        harness.run()
    finally:
        try:
            harness.update_option("model_health_policy_setting.enabled", "false")
            harness.update_option("model_health_policy_setting.rules", "[]")
            if getattr(harness, "monitor_probe_original", None) is not None:
                harness.update_option("model_monitor_setting.auto_probe_enabled", harness.monitor_probe_original)
            if getattr(harness, "monitor_original", None) is not None:
                harness.update_option("model_monitor_setting.enabled", harness.monitor_original)
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
    parser.add_argument("--shared-mock-key", default="mock-key-a")
    parser.add_argument("--solo-mock-key", default="mock-key-b")
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
