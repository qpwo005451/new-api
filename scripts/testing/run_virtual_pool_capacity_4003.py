#!/usr/bin/env python3
import argparse
import concurrent.futures
import json
import os
import statistics
import sys
import threading
import time
import urllib.parse
from dataclasses import dataclass, field
from typing import Any

import requests


@dataclass
class Result:
    status: int
    elapsed_ms: float
    retry_after: str | None
    body: str


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
        self.admin_token = args.admin_token
        self.relay_token = args.relay_token
        self.report: list[TestReport] = []
        self.initial_routes: dict[str, Any] = {}
        self.initial_sticky: dict[str, Any] = {}
        self.admin_session = requests.Session()
        self.admin_session.headers.update({"Authorization": "Bearer " + self.admin_token})
        self.relay_session = requests.Session()
        self.relay_session.headers.update({"Authorization": "Bearer " + self.relay_token})

    def api(self, method: str, path: str, **kwargs: Any) -> requests.Response:
        url = self.base_url + path
        return self.admin_session.request(method, url, timeout=kwargs.pop("timeout", 15), **kwargs)

    def mock(self, method: str, path: str, **kwargs: Any) -> requests.Response:
        url = self.mock_url + path
        return requests.request(method, url, timeout=kwargs.pop("timeout", 10), **kwargs)

    def options(self) -> dict[str, Any]:
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

    def set_routes(self, routes: dict[str, Any]) -> None:
        result = self.update_option(
            "model_retry_policy_setting.virtual_model_routes",
            json.dumps(routes, separators=(",", ":")),
        )
        if not result.get("success"):
            raise RuntimeError("route update failed: " + json.dumps(result))

    def set_sticky(self, sticky: dict[str, Any]) -> None:
        result = self.update_option(
            "model_retry_policy_setting.virtual_pool_sticky",
            json.dumps(sticky, separators=(",", ":")),
        )
        if not result.get("success"):
            raise RuntimeError("sticky update failed: " + json.dumps(result))

    def snapshot(self) -> dict[str, Any]:
        options = self.options()
        return {
            "routes": json.loads(options["model_retry_policy_setting.virtual_model_routes"]),
            "sticky": json.loads(options["model_retry_policy_setting.virtual_pool_sticky"]),
        }

    def mock_reset(self, config: dict[str, Any] | None = None) -> dict[str, Any]:
        self.mock("POST", "/control/reset", json={}).raise_for_status()
        if config:
            response = self.mock("POST", "/control/config", json=config)
            response.raise_for_status()
        return self.mock("GET", "/stats").json()

    def mock_config(self, config: dict[str, Any]) -> dict[str, Any]:
        response = self.mock("POST", "/control/config", json=config)
        response.raise_for_status()
        return response.json()

    def mock_stats(self) -> dict[str, Any]:
        response = self.mock("GET", "/stats")
        response.raise_for_status()
        return response.json()

    def relay(self, model: str, session_id: str | None = None, body: dict[str, Any] | None = None, timeout: float = 15.0) -> Result:
        payload = body or {
            "model": model,
            "messages": [{"role": "user", "content": "ping"}],
        }
        headers = {}
        if session_id:
            headers["X-NewAPI-Session-ID"] = session_id
        started = time.monotonic()
        try:
            response = requests.post(
                self.base_url + "/v1/chat/completions",
                headers={**self.relay_session.headers, **headers},
                json=payload,
                timeout=timeout,
            )
            elapsed = (time.monotonic() - started) * 1000.0
            return Result(response.status_code, elapsed, response.headers.get("Retry-After"), response.text)
        except requests.RequestException as exc:
            elapsed = (time.monotonic() - started) * 1000.0
            return Result(0, elapsed, None, str(exc))

    def responses(self, model: str, body: dict[str, Any], timeout: float = 15.0) -> Result:
        started = time.monotonic()
        try:
            response = requests.post(
                self.base_url + "/v1/responses",
                headers=self.relay_session.headers,
                json=body,
                timeout=timeout,
            )
            elapsed = (time.monotonic() - started) * 1000.0
            return Result(response.status_code, elapsed, response.headers.get("Retry-After"), response.text)
        except requests.RequestException as exc:
            elapsed = (time.monotonic() - started) * 1000.0
            return Result(0, elapsed, None, str(exc))

    def burst(self, model: str, count: int, timeout: float = 15.0, sessions: list[str | None] | None = None) -> list[Result]:
        barrier = threading.Barrier(count)
        sessions = sessions or [None] * count
        if len(sessions) != count:
            raise ValueError("sessions length must match count")

        def worker(session_id: str | None) -> Result:
            barrier.wait(timeout=10)
            return self.relay(model, session_id=session_id, timeout=timeout)

        with concurrent.futures.ThreadPoolExecutor(max_workers=count) as executor:
            futures = [executor.submit(worker, session_id) for session_id in sessions]
            return [future.result() for future in futures]

    def add_report(self, name: str, passed: bool, detail: dict[str, Any], error: str = "") -> None:
        self.report.append(TestReport(name, passed, detail, error))

    def require(self, condition: bool, message: str) -> None:
        if not condition:
            raise AssertionError(message)

    def setup_routes(self) -> None:
        current = self.snapshot()
        self.initial_routes = current["routes"]
        self.initial_sticky = current["sticky"]
        routes = dict(self.initial_routes)
        routes.update(
            {
                "vpool-input-lab": {
                    "rotation": "round_robin",
                    "max_attempts": 5,
                    "capacity_groups": {"input-lab": {"capacity": 15}},
                    "targets": [
                        {"model": "vpool-mock-model", "channel_id": 8801, "shared_capacity_group": "input-lab"},
                        {"model": "vpool-mock-model", "channel_id": 8802, "shared_capacity_group": "input-lab"},
                        {"model": "vpool-mock-model", "channel_id": 8803, "shared_capacity_group": "input-lab"},
                    ],
                },
                "vpool-ollama-lab": {
                    "rotation": "round_robin",
                    "max_attempts": 2,
                    "capacity_groups": {"ollama-lab": {"capacity": 3}},
                    "targets": [
                        {"model": "vpool-mock-model", "channel_id": 8804, "shared_capacity_group": "ollama-lab"},
                    ],
                },
                "vpool-commandcode-lab": {
                    "rotation": "round_robin",
                    "max_attempts": 2,
                    "capacity_groups": {"commandcode-lab": {"capacity": 4}},
                    "targets": [
                        {"model": "vpool-mock-model", "channel_id": 8805, "shared_capacity_group": "commandcode-lab"},
                    ],
                },
                "vpool-shared-name-lab": {
                    "rotation": "round_robin",
                    "max_attempts": 5,
                    "capacity_groups": {"shared-name-lab": {"capacity": 2}},
                    "targets": [
                        {"model": "vpool-mock-model", "shared_capacity_group": "shared-name-lab"},
                    ],
                },
                "vpool-pinned-lab": {
                    "rotation": "round_robin",
                    "max_attempts": 2,
                    "capacity_groups": {"pinned-lab": {"capacity": 1}},
                    "targets": [
                        {"model": "vpool-mock-model", "channel_id": 8802, "shared_capacity_group": "pinned-lab"},
                    ],
                },
                "vpool-runtime-lab": {
                    "rotation": "round_robin",
                    "max_attempts": 2,
                    "capacity_groups": {"runtime-lab": {"capacity": 1}},
                    "targets": [
                        {"model": "vpool-mock-model", "channel_id": 8801, "shared_capacity_group": "runtime-lab"},
                    ],
                },
                "vpool-inherit-lab": {
                    "rotation": "round_robin",
                    "max_attempts": 2,
                    "capacity_groups": {"inherit-lab": {"capacity": 2}},
                    "targets": [
                        {"model": "vpool-mock-model", "channel_id": 8801, "shared_capacity_group": "inherit-lab"},
                    ],
                },
                "vpool-ceiling-lab": {
                    "rotation": "round_robin",
                    "max_attempts": 2,
                    "capacity_groups": {"ceiling-lab": {"capacity": 2}},
                    "targets": [
                        {"model": "vpool-mock-model", "channel_id": 8801, "capacity": 1, "shared_capacity_group": "ceiling-lab"},
                    ],
                },
                "vpool-health-lab": {
                    "rotation": "ordered",
                    "max_attempts": 2,
                    "health": {"enabled": True, "failure_threshold": 1, "cooldown_seconds": 30, "max_cooldown_seconds": 60},
                    "targets": [
                        {"model": "vpool-mock-model", "channel_id": 8801},
                        {"model": "vpool-mock-model", "channel_id": 8802},
                    ],
                },
                "vpool-disable-model-lab": {
                    "rotation": "ordered",
                    "max_attempts": 2,
                    "health": {
                        "enabled": True,
                        "disable_model": True,
                        "failure_threshold": 2,
                        "cooldown_seconds": 2,
                    },
                    "targets": [
                        {"model": "vpool-mock-model", "channel_id": 8801},
                        {"model": "vpool-mock-model", "channel_id": 8802},
                    ],
                },
                "vpool-disable-recovery-lab": {
                    "rotation": "ordered",
                    "max_attempts": 2,
                    "health": {
                        "enabled": True,
                        "disable_model": True,
                        "failure_threshold": 2,
                        "cooldown_seconds": 2,
                    },
                    "targets": [
                        {"model": "vpool-mock-model", "channel_id": 8801},
                        {"model": "vpool-mock-model", "channel_id": 8802},
                    ],
                },
                "vpool-sticky-lab": {
                    "rotation": "round_robin",
                    "max_attempts": 2,
                    "capacity_groups": {"sticky-lab": {"capacity": 2}},
                    "targets": [
                        {"model": "vpool-mock-model", "channel_id": 8801, "shared_capacity_group": "sticky-lab"},
                        {"model": "vpool-mock-model", "channel_id": 8802, "shared_capacity_group": "sticky-lab"},
                    ],
                },
                "vpool-response-lab": {
                    "rotation": "round_robin",
                    "max_attempts": 2,
                    "capacity_groups": {"response-lab": {"capacity": 2}},
                    "targets": [
                        {"model": "vpool-mock-model", "channel_id": 8801, "shared_capacity_group": "response-lab"},
                        {"model": "vpool-mock-model", "channel_id": 8802, "shared_capacity_group": "response-lab"},
                    ],
                },
            }
        )
        self.set_routes(routes)
        sticky = dict(self.initial_sticky)
        sticky.update(
            {
                "enabled": True,
                "session_mode": "thread",
                "binding_mode": "memory",
                "multi_key_policy": "bind_index",
                "capacity_wait_millis": 2000,
                "claim_wait_millis": 2000,
                "pending_lease_seconds": 60,
                "confirmed_ttl_seconds": 3600,
                "capacity_lease_seconds": 300,
            }
        )
        sticky.pop("redis_required_for_ready", None)
        self.set_sticky(sticky)

    def restore_options(self) -> None:
        if self.initial_routes:
            self.set_routes(self.initial_routes)
        if self.initial_sticky:
            self.set_sticky(self.initial_sticky)

    def run_case(self, name: str, function) -> None:
        try:
            detail = function()
            self.add_report(name, True, detail)
            print("PASS", name, json.dumps(detail, ensure_ascii=False))
        except Exception as exc:
            self.add_report(name, False, {}, str(exc))
            print("FAIL", name, str(exc), file=sys.stderr)

    def case_capacity_input(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 2500})
        results = self.burst("vpool-input-lab", 20, timeout=12)
        stats = self.mock_stats()
        statuses = sorted(result.status for result in results)
        self.require(statuses.count(200) == 15, "expected 15 successful admissions, got " + repr(statuses))
        self.require(statuses.count(429) == 5, "expected 5 capacity rejections, got " + repr(statuses))
        self.require(all(result.retry_after == "1" for result in results if result.status == 429), "429 must include Retry-After: 1")
        self.require(stats["max_active"] == 15, "mock max_active must be 15, got " + str(stats["max_active"]))
        return {
            "success": statuses.count(200),
            "rejected": statuses.count(429),
            "max_active": stats["max_active"],
            "by_channel": stats["calls_by_channel"],
            "elapsed_ms": round(statistics.mean(result.elapsed_ms for result in results), 1),
        }

    def case_capacity_ollama(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 2500})
        results = self.burst("vpool-ollama-lab", 4, timeout=10)
        stats = self.mock_stats()
        statuses = [result.status for result in results]
        self.require(statuses.count(200) == 3, "expected 3 successful Ollama admissions, got " + repr(statuses))
        self.require(statuses.count(429) == 1, "expected 1 Ollama capacity rejection, got " + repr(statuses))
        self.require(stats["max_active"] == 3, "mock max_active must be 3, got " + str(stats["max_active"]))
        return {"statuses": statuses, "max_active": stats["max_active"], "by_channel": stats["calls_by_channel"]}

    def case_capacity_commandcode(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 2500})
        results = self.burst("vpool-commandcode-lab", 5, timeout=10)
        stats = self.mock_stats()
        statuses = [result.status for result in results]
        self.require(statuses.count(200) == 4, "expected 4 successful CommandCode admissions, got " + repr(statuses))
        self.require(statuses.count(429) == 1, "expected 1 CommandCode capacity rejection, got " + repr(statuses))
        self.require(stats["max_active"] == 4, "mock max_active must be 4, got " + str(stats["max_active"]))
        return {"statuses": statuses, "max_active": stats["max_active"], "by_channel": stats["calls_by_channel"]}

    def case_groups_are_isolated(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 2500})
        with concurrent.futures.ThreadPoolExecutor(max_workers=22) as executor:
            futures = []
            futures.extend(executor.submit(self.relay, "vpool-input-lab", None, None, 12) for _ in range(15))
            futures.extend(executor.submit(self.relay, "vpool-ollama-lab", None, None, 12) for _ in range(3))
            futures.extend(executor.submit(self.relay, "vpool-commandcode-lab", None, None, 12) for _ in range(4))
            results = [future.result() for future in futures]
        stats = self.mock_stats()
        self.require(sum(1 for result in results if result.status == 200) == 22, "all 22 requests across independent groups should succeed")
        self.require(stats["max_active"] == 22, "independent groups should run concurrently, got " + str(stats["max_active"]))
        return {"success": 22, "max_active": stats["max_active"], "by_channel": stats["calls_by_channel"]}

    def case_pinned_channel(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 50})
        results = [self.relay("vpool-pinned-lab", timeout=10) for _ in range(4)]
        stats = self.mock_stats()
        self.require(all(result.status == 200 for result in results), "pinned requests should succeed")
        self.require(stats["calls_by_channel"].get("8802") == 4, "pinned route must use channel 8802 only")
        self.require(len(stats["calls_by_channel"]) == 1, "pinned route called another channel: " + repr(stats["calls_by_channel"]))
        return {"calls_by_channel": stats["calls_by_channel"]}

    def case_shared_name_candidates(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 2500})
        results = self.burst("vpool-shared-name-lab", 3, timeout=10)
        stats = self.mock_stats()
        statuses = [result.status for result in results]
        self.require(statuses.count(200) == 2, "shared same-name pool should admit 2 requests, got " + repr(statuses))
        self.require(statuses.count(429) == 1, "shared same-name pool should reject the third request, got " + repr(statuses))
        self.require(stats["max_active"] == 2, "shared same-name pool max_active must be 2")
        return {"statuses": statuses, "max_active": stats["max_active"], "by_channel": stats["calls_by_channel"]}

    def case_runtime_capacity_increase(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 2500})
        routes = self.snapshot()["routes"]
        runtime = routes["vpool-runtime-lab"]
        first_future = concurrent.futures.ThreadPoolExecutor(max_workers=1).submit(self.relay, "vpool-runtime-lab", None, None, 12)
        time.sleep(0.4)
        runtime["capacity_groups"]["runtime-lab"]["capacity"] = 2
        routes["vpool-runtime-lab"] = runtime
        self.set_routes(routes)
        second = self.relay("vpool-runtime-lab", timeout=10)
        first = first_future.result()
        stats = self.mock_stats()
        self.require(first.status == 200 and second.status == 200, "raising capacity must admit the next request")
        self.require(stats["max_active"] == 2, "raised capacity should allow two active calls, got " + str(stats["max_active"]))
        runtime["capacity_groups"]["runtime-lab"]["capacity"] = 1
        routes["vpool-runtime-lab"] = runtime
        self.set_routes(routes)
        return {"first": first.status, "second": second.status, "max_active": stats["max_active"]}

    def case_runtime_capacity_decrease(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 2500})
        routes = self.snapshot()["routes"]
        runtime = routes["vpool-runtime-lab"]
        runtime["capacity_groups"]["runtime-lab"]["capacity"] = 1
        routes["vpool-runtime-lab"] = runtime
        self.set_routes(routes)
        first_future = concurrent.futures.ThreadPoolExecutor(max_workers=1).submit(self.relay, "vpool-runtime-lab", None, None, 12)
        time.sleep(0.4)
        second = self.relay("vpool-runtime-lab", timeout=4)
        first = first_future.result()
        self.require(first.status == 200, "first request should complete successfully")
        self.require(second.status == 429, "capacity 1 should reject a concurrent second request, got " + str(second.status))
        return {"first": first.status, "second": second.status, "second_elapsed_ms": round(second.elapsed_ms, 1)}

    def case_inherited_capacity(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 2500})
        results = self.burst("vpool-inherit-lab", 3, timeout=10)
        statuses = [result.status for result in results]
        self.require(statuses.count(200) == 2 and statuses.count(429) == 1, "omitted target capacity must inherit group capacity 2, got " + repr(statuses))
        return {"statuses": statuses}

    def case_explicit_group_ceiling(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 2500})
        results = self.burst("vpool-ceiling-lab", 3, timeout=10)
        statuses = [result.status for result in results]
        self.require(statuses.count(200) == 2 and statuses.count(429) == 1, "group ceiling 2 must override target capacity 1, got " + repr(statuses))
        return {"statuses": statuses}

    def case_health_cooldown(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 50, "channels": {"8801": {"status": 500}}})
        first = self.relay("vpool-health-lab", timeout=10)
        self.require(first.status == 500, "the failing ordered member should surface its 500, got " + str(first.status))
        first_stats = self.mock_stats()
        self.require(first_stats["calls_by_channel"].get("8801") == 1, "the first health request must reach the failing member")

        second = self.relay("vpool-health-lab", timeout=10)
        self.require(second.status == 200, "the next request must prefer the healthy member, got " + str(second.status))
        second_stats = self.mock_stats()
        self.require(second_stats["calls_by_channel"].get("8802") == 1, "the healthy member must serve the next request")
        self.require(second_stats["calls_by_channel"].get("8801") == 1, "the cooling member must not be retried on the next request")
        self.mock_config({"channels": {"8801": {"status": 0}}})
        return {"first": first.status, "second": second.status, "calls_by_channel": second_stats["calls_by_channel"]}

    def case_disable_model_strict(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 50, "channels": {"8801": {"status": 503}}})
        first = self.relay("vpool-disable-model-lab", timeout=10)
        second = self.relay("vpool-disable-model-lab", timeout=10)
        after_threshold = self.mock_stats()
        self.require(first.status == 503, "the first disabled-model request should surface 503, got " + str(first.status))
        self.require(second.status == 503, "the second request should reach the failure threshold, got " + str(second.status))
        self.require(
            after_threshold["calls_by_channel"].get("8801") == 2,
            "two separate requests must reach the failing member before the disable threshold",
        )

        third = self.relay("vpool-disable-model-lab", timeout=10)
        stats = self.mock_stats()
        self.require(third.status == 200, "a disabled model must not be used as a last resort, got " + str(third.status))
        self.require(stats["calls_by_channel"].get("8801") == 2, "the disabled member must be skipped after the threshold")
        self.require(stats["calls_by_channel"].get("8802") == 1, "the healthy member must serve the post-threshold request")
        return {
            "first": first.status,
            "second": second.status,
            "third": third.status,
            "calls_by_channel": stats["calls_by_channel"],
        }

    def case_disable_model_recovers(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 50, "channels": {"8801": {"status": 429}}})
        first = self.relay("vpool-disable-recovery-lab", timeout=10)
        second = self.relay("vpool-disable-recovery-lab", timeout=10)
        before_recovery = self.mock_stats()
        self.require(first.status == 429, "the first recovery request should surface 429, got " + str(first.status))
        self.require(second.status == 200, "the second recovery request should retry to the healthy member, got " + str(second.status))
        self.require(
            before_recovery["calls_by_channel"].get("8801") == 2,
            "the failing member must be disabled after the configured threshold",
        )

        self.mock_config({"channels": {"8801": {"status": 0}}})
        time.sleep(2.3)
        recovered = self.relay("vpool-disable-recovery-lab", timeout=10)
        stats = self.mock_stats()
        self.require(recovered.status == 200, "a model must be tried again after cooldown expiry, got " + str(recovered.status))
        self.require(stats["calls_by_channel"].get("8801") == 3, "the recovered member must be selected after cooldown expiry")
        return {
            "first": first.status,
            "second": second.status,
            "recovered": recovered.status,
            "calls_by_channel": stats["calls_by_channel"],
        }

    def case_sticky_same_session(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 50})
        session_id = "sticky-lab-session"
        first = self.relay("vpool-sticky-lab", session_id=session_id, timeout=10)
        second = self.relay("vpool-sticky-lab", session_id=session_id, timeout=10)
        stats = self.mock_stats()
        self.require(first.status == 200 and second.status == 200, "sticky requests should succeed")
        self.require(stats["max_active"] == 1, "sticky sequential requests should not overlap")
        used = list(stats["calls_by_channel"].keys())
        self.require(len(used) == 1, "same session should reuse one channel, got " + repr(stats["calls_by_channel"]))
        return {"channel": used[0], "calls_by_channel": stats["calls_by_channel"]}

    def case_sticky_different_sessions(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 50})
        first = self.relay("vpool-sticky-lab", session_id="sticky-session-a", timeout=10)
        second = self.relay("vpool-sticky-lab", session_id="sticky-session-b", timeout=10)
        stats = self.mock_stats()
        self.require(first.status == 200 and second.status == 200, "different sticky sessions should succeed")
        self.require(len(stats["calls_by_channel"]) == 2, "different sessions should spread across candidates, got " + repr(stats["calls_by_channel"]))
        return {"calls_by_channel": stats["calls_by_channel"]}

    def case_response_owner(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 50})
        first = self.responses("vpool-response-lab", {"model": "vpool-response-lab", "input": "first"})
        self.require(first.status == 200, "initial responses request should succeed, got " + str(first.status))
        payload = json.loads(first.body)
        response_id = payload.get("id", "")
        self.require(response_id.startswith("resp_mock_"), "mock response id missing")
        owner_channel = response_id.split("_")[2]
        continuation = self.responses(
            "vpool-response-lab",
            {"model": "vpool-response-lab", "input": "continue", "previous_response_id": response_id},
        )
        unknown = self.responses(
            "vpool-response-lab",
            {"model": "vpool-response-lab", "input": "continue", "previous_response_id": "resp_unknown"},
        )
        stats = self.mock_stats()
        self.require(continuation.status == 200, "known previous_response_id should succeed")
        self.require(unknown.status == 409, "unknown previous_response_id should fail closed with 409, got " + str(unknown.status))
        self.require(stats["calls_by_channel"].get(owner_channel) == 2, "continuation must stay on owner channel")
        return {
            "owner_channel": owner_channel,
            "continuation_status": continuation.status,
            "unknown_status": unknown.status,
            "calls_by_channel": stats["calls_by_channel"],
        }

    def case_capacity_without_sticky(self) -> dict[str, Any]:
        sticky = self.snapshot()["sticky"]
        sticky["enabled"] = False
        self.set_sticky(sticky)
        try:
            self.mock_reset({"default_delay_ms": 2500})
            results = self.burst("vpool-input-lab", 20, timeout=12)
            stats = self.mock_stats()
            statuses = sorted(result.status for result in results)
            self.require(statuses.count(200) == 15, "sticky-disabled capacity must admit 15 requests, got " + repr(statuses))
            self.require(statuses.count(429) == 5, "sticky-disabled capacity must reject 5 requests, got " + repr(statuses))
            self.require(all(result.retry_after == "1" for result in results if result.status == 429), "429 must include Retry-After: 1")
            self.require(stats["max_active"] == 15, "sticky-disabled max_active must be 15, got " + str(stats["max_active"]))
            return {
                "success": statuses.count(200),
                "rejected": statuses.count(429),
                "max_active": stats["max_active"],
                "by_channel": stats["calls_by_channel"],
                "elapsed_ms": round(statistics.mean(result.elapsed_ms for result in results), 1),
            }
        finally:
            sticky["enabled"] = True
            self.set_sticky(sticky)

    def case_redis_required_fail_closed(self) -> dict[str, Any]:
        sticky = self.snapshot()["sticky"]
        sticky["binding_mode"] = "redis"
        sticky["redis_required_for_ready"] = True
        self.set_sticky(sticky)
        result = self.relay("vpool-input-lab", timeout=10)
        self.require(result.status == 503, "redis-required mode without Redis must fail closed with 503, got " + str(result.status))
        sticky["binding_mode"] = "memory"
        sticky.pop("redis_required_for_ready", None)
        self.set_sticky(sticky)
        return {"status": result.status, "body": result.body[:300]}

    def case_capacity_zero(self) -> dict[str, Any]:
        routes = self.snapshot()["routes"]
        routes["vpool-zero-lab"] = {
            "rotation": "ordered",
            "max_attempts": 1,
            "capacity_groups": {"zero-lab": {"capacity": 0}},
            "targets": [{"model": "vpool-mock-model", "channel_id": 8801, "shared_capacity_group": "zero-lab"}],
        }
        self.set_routes(routes)
        self.mock_reset({"default_delay_ms": 2500})
        results = self.burst("vpool-zero-lab", 2, timeout=10)
        statuses = [result.status for result in results]
        self.require(statuses.count(200) == 1 and statuses.count(429) == 1, "capacity 0 must behave as one slot, got " + repr(statuses))
        routes.pop("vpool-zero-lab", None)
        self.set_routes(routes)
        return {"statuses": statuses}

    def case_invalid_capacity(self) -> dict[str, Any]:
        routes = self.snapshot()["routes"]
        invalid = dict(routes)
        invalid["vpool-invalid-lab"] = {
            "rotation": "ordered",
            "capacity_groups": {"bad": {"capacity": -1}},
            "targets": [{"model": "vpool-mock-model", "channel_id": 8801, "shared_capacity_group": "bad"}],
        }
        result = self.update_option(
            "model_retry_policy_setting.virtual_model_routes",
            json.dumps(invalid, separators=(",", ":")),
        )
        self.require(result.get("success") is False, "negative capacity update must be rejected")
        return {"response": result}

    def case_invalid_group_name(self) -> dict[str, Any]:
        routes = self.snapshot()["routes"]
        invalid = dict(routes)
        invalid["vpool-invalid-name-lab"] = {
            "rotation": "ordered",
            "capacity_groups": {"": {"capacity": 1}},
            "targets": [{"model": "vpool-mock-model", "channel_id": 8801, "shared_capacity_group": ""}],
        }
        result = self.update_option(
            "model_retry_policy_setting.virtual_model_routes",
            json.dumps(invalid, separators=(",", ":")),
        )
        self.require(result.get("success") is False, "empty capacity group name must be rejected")
        return {"response": result}

    def case_invalid_long_group_name(self) -> dict[str, Any]:
        routes = self.snapshot()["routes"]
        invalid = dict(routes)
        invalid["vpool-invalid-long-lab"] = {
            "rotation": "ordered",
            "capacity_groups": {"x" * 129: {"capacity": 1}},
            "targets": [{"model": "vpool-mock-model", "channel_id": 8801, "shared_capacity_group": "x" * 129}],
        }
        result = self.update_option(
            "model_retry_policy_setting.virtual_model_routes",
            json.dumps(invalid, separators=(",", ":")),
        )
        self.require(result.get("success") is False, "overlong capacity group name must be rejected")
        return {"response": result}

    def case_missing_channel_id(self) -> dict[str, Any]:
        routes = self.snapshot()["routes"]
        routes["vpool-missing-channel-lab"] = {
            "rotation": "ordered",
            "max_attempts": 2,
            "targets": [{"model": "vpool-mock-model", "channel_id": 999999}],
        }
        self.set_routes(routes)
        self.mock_reset({"default_delay_ms": 50})
        result = self.relay("vpool-missing-channel-lab", timeout=10)
        stats = self.mock_stats()
        self.require(result.status == 503, "missing channel_id must fail without fallback, got " + str(result.status))
        self.require(stats["max_active"] == 0, "missing channel_id must not call an upstream")
        routes.pop("vpool-missing-channel-lab", None)
        self.set_routes(routes)
        return {"status": result.status, "calls": stats["calls_by_channel"]}

    def case_channel_disabled_midflight(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 2500})
        first_future = concurrent.futures.ThreadPoolExecutor(max_workers=1).submit(self.relay, "vpool-ollama-lab", None, None, 12)
        time.sleep(0.4)
        disable = self.api("POST", "/api/channel/8804/status", json={"status": 2})
        disable.raise_for_status()
        try:
            second = self.relay("vpool-ollama-lab", timeout=4)
            first = first_future.result()
            self.require(first.status == 200, "in-flight request should finish after channel disable")
            self.require(second.status in (429, 503), "new admission must stop after channel disable, got " + str(second.status))
        finally:
            restore = self.api("POST", "/api/channel/8804/status", json={"status": 1})
            restore.raise_for_status()
        return {"first": first.status, "second": second.status}

    def case_rotation_modes(self) -> dict[str, Any]:
        routes = self.snapshot()["routes"]
        routes["vpool-ordered-lab"] = {
            "rotation": "ordered",
            "max_attempts": 2,
            "capacity_groups": {"rotation-lab": {"capacity": 2}},
            "targets": [
                {"model": "vpool-mock-model", "channel_id": 8801, "shared_capacity_group": "rotation-lab"},
                {"model": "vpool-mock-model", "channel_id": 8802, "shared_capacity_group": "rotation-lab"},
            ],
        }
        routes["vpool-random-lab"] = {
            "rotation": "random",
            "max_attempts": 2,
            "capacity_groups": {"random-lab": {"capacity": 2}},
            "targets": [
                {"model": "vpool-mock-model", "channel_id": 8801, "shared_capacity_group": "random-lab"},
                {"model": "vpool-mock-model", "channel_id": 8802, "shared_capacity_group": "random-lab"},
            ],
        }
        self.set_routes(routes)
        self.mock_reset({"default_delay_ms": 20})
        ordered = [self.relay("vpool-ordered-lab", timeout=10) for _ in range(3)]
        ordered_stats = self.mock_stats()
        self.require(all(result.status == 200 for result in ordered), "ordered rotation requests should succeed")
        self.require(ordered_stats["calls_by_channel"].get("8801") == 3, "ordered rotation must keep the first target first")
        self.mock_reset({"default_delay_ms": 20})
        random_results = [self.relay("vpool-random-lab", timeout=10) for _ in range(10)]
        random_stats = self.mock_stats()
        self.require(all(result.status == 200 for result in random_results), "random rotation requests should succeed")
        self.require(len(random_stats["calls_by_channel"]) == 2, "random rotation should eventually vary targets")
        routes.pop("vpool-ordered-lab", None)
        routes.pop("vpool-random-lab", None)
        self.set_routes(routes)
        return {
            "ordered": ordered_stats["calls_by_channel"],
            "random": random_stats["calls_by_channel"],
        }

    def case_client_disconnect(self) -> dict[str, Any]:
        self.mock_reset({"default_delay_ms": 2500})
        parsed = urllib.parse.urlparse(self.base_url)
        host = parsed.hostname or "127.0.0.1"
        port = parsed.port or 80
        body = json.dumps({"model": "vpool-input-lab", "messages": [{"role": "user", "content": "ping"}]}).encode()
        import socket

        sock = socket.create_connection((host, port), timeout=5)
        try:
            request = (
                b"POST /v1/chat/completions HTTP/1.1\r\n"
                b"Host: 127.0.0.1\r\n"
                b"Authorization: Bearer " + self.relay_token.encode() + b"\r\n"
                b"Content-Type: application/json\r\n"
                b"Content-Length: " + str(len(body)).encode() + b"\r\n"
                b"\r\n" + body
            )
            sock.sendall(request)
            time.sleep(0.25)
        finally:
            sock.close()
        time.sleep(0.5)
        after = self.relay("vpool-input-lab", timeout=10)
        self.require(after.status == 200, "capacity lease must be released after client disconnect")
        return {"after_disconnect_status": after.status}

    def run(self) -> None:
        print("candidate", self.base_url, "mock", self.mock_url)
        self.setup_routes()
        cases = [
            ("F01-input-capacity-15", self.case_capacity_input),
            ("F02-input-round-robin", self.case_capacity_input),
            ("F03-ollama-capacity-3", self.case_capacity_ollama),
            ("F04-commandcode-capacity-4", self.case_capacity_commandcode),
            ("F04b-groups-isolated", self.case_groups_are_isolated),
            ("F05-pinned-channel", self.case_pinned_channel),
            ("F06-shared-name-candidates", self.case_shared_name_candidates),
            ("F07-runtime-increase", self.case_runtime_capacity_increase),
            ("F08-runtime-decrease", self.case_runtime_capacity_decrease),
            ("F09-inherited-capacity", self.case_inherited_capacity),
            ("F10-explicit-group-ceiling", self.case_explicit_group_ceiling),
            ("F11-rotation-modes", self.case_rotation_modes),
            ("F13-health-cooldown", self.case_health_cooldown),
            ("F17-disable-model-strict", self.case_disable_model_strict),
            ("F17b-disable-model-recovers", self.case_disable_model_recovers),
            ("F14-sticky-same-session", self.case_sticky_same_session),
            ("F14b-sticky-different-sessions", self.case_sticky_different_sessions),
            ("F15-capacity-without-sticky", self.case_capacity_without_sticky),
            ("F16-response-owner", self.case_response_owner),
            ("B01-capacity-zero", self.case_capacity_zero),
            ("B02-invalid-negative-capacity", self.case_invalid_capacity),
            ("B03-invalid-empty-group", self.case_invalid_group_name),
            ("B04-invalid-long-group", self.case_invalid_long_group_name),
            ("B05-missing-channel-id", self.case_missing_channel_id),
            ("B06-channel-disabled-midflight", self.case_channel_disabled_midflight),
            ("B07-redis-required-fail-closed", self.case_redis_required_fail_closed),
            ("B08-client-disconnect", self.case_client_disconnect),
        ]
        for name, function in cases:
            self.run_case(name, function)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="http://127.0.0.1:4003")
    parser.add_argument("--mock-url", default="http://127.0.0.1:18083")
    parser.add_argument("--admin-token", default=os.environ.get("NEW_API_ADMIN_PAT", ""))
    parser.add_argument("--relay-token", default=os.environ.get("NEW_API_RELAY_TOKEN", ""))
    parser.add_argument("--report", default="/tmp/vpool-capacity-4003-report.json")
    args = parser.parse_args()
    if not args.admin_token or not args.relay_token:
        print("NEW_API_ADMIN_PAT and NEW_API_RELAY_TOKEN are required", file=sys.stderr)
        return 2

    harness = Harness(args)
    try:
        harness.run()
    finally:
        try:
            harness.restore_options()
        except Exception as exc:
            print("WARN failed to restore options:", exc, file=sys.stderr)
        report = {
            "candidate": args.base_url,
            "mock": args.mock_url,
            "passed": sum(1 for item in harness.report if item.passed),
            "failed": sum(1 for item in harness.report if not item.passed),
            "tests": [
                {"name": item.name, "passed": item.passed, "detail": item.detail, "error": item.error}
                for item in harness.report
            ],
        }
        with open(args.report, "w", encoding="utf-8") as handle:
            json.dump(report, handle, indent=2, ensure_ascii=False)
        print("REPORT", args.report, "passed", report["passed"], "failed", report["failed"])
    return 1 if any(not item.passed for item in harness.report) else 0


if __name__ == "__main__":
    raise SystemExit(main())
