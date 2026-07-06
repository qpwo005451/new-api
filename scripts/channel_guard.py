#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import sqlite3
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any, Callable

STATE_VERSION = 1
DEFAULT_DB_PATH = Path("/opt/new-api/data/new-api.db")
DEFAULT_STATE_PATH = Path("/opt/new-api/data/channel_guard_state.json")
DEFAULT_LOG_PATH = Path("/opt/new-api/data/channel_guard.log")
LEGACY_INPUT_STATE_PATH = Path("/opt/new-api/data/input_budget_guard_state.json")
DEFAULT_QUOTA_PER_UNIT = 500000.0
DEFAULT_INPUT_CHANNEL_ID = 9
DEFAULT_INPUT_BUDGET_USD = 300.0
DEFAULT_FOG_CHANNEL_ID = 1
DEFAULT_FOG_MODEL = "gpt-5.4-mini"
DEFAULT_FOG_TIMEOUT_SECONDS = 20
DEFAULT_FOG_USER_AGENT = "Go-http-client/1.1"
CONSUME_LOG_TYPE = 2
ERROR_LOG_TYPE = 5
CHANNEL_STATUS_ENABLED = 1
CHANNEL_STATUS_DISABLED = 2
CHANNEL_STATUS_AUTO_DISABLED = 3
FOG_SOFT_FAILURE_THRESHOLD = 5
FOG_SUCCESS_THRESHOLD = 2


class ProbeResult:
    def __init__(self, ok: bool, category: str, http_status: int | None, message: str) -> None:
        self.ok = ok
        self.category = category
        self.http_status = http_status
        self.message = message


def now_local() -> dt.datetime:
    return dt.datetime.now().astimezone()


def local_day_bounds(now: dt.datetime) -> tuple[int, int, str]:
    start = now.replace(hour=0, minute=0, second=0, microsecond=0)
    end = start + dt.timedelta(days=1)
    return int(start.timestamp()), int(end.timestamp()), start.date().isoformat()


def _load_json_mapping(path: Path, *, label: str) -> dict[str, Any]:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise ValueError(f"invalid JSON in {label}: {path}") from exc
    if not isinstance(data, dict):
        raise ValueError(f"expected JSON object in {label}: {path}")
    return data


def load_state(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {}
    return _load_json_mapping(path, label="state")


def save_state(path: Path, state: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(json.dumps(state, ensure_ascii=False, sort_keys=True, indent=2) + "\n", encoding="utf-8")
    os.replace(tmp, path)


def append_log_line(path: Path, rule: str, result: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    action = str(result.get("action", ""))
    reason = str(result.get("reason", "")).replace("\n", " ").replace("\r", " ")
    with path.open("a", encoding="utf-8") as fh:
        fh.write(f"{now_local().isoformat(timespec='seconds')} rule={rule} action={action} reason={reason}\n")


def ensure_rules_root(state: dict[str, Any]) -> dict[str, Any]:
    state.setdefault("version", STATE_VERSION)
    return state.setdefault("rules", {})


def migrate_legacy_input_budget_state(state: dict[str, Any], legacy_path: Path) -> bool:
    rules = ensure_rules_root(state)
    if "input_budget" in rules or not legacy_path.exists():
        return False
    legacy = _load_json_mapping(legacy_path, label="legacy input budget state")
    rules["input_budget"] = dict(legacy)
    return True


def ensure_fog_rule_state(state: dict[str, Any], channel_id: int) -> dict[str, Any]:
    rules = ensure_rules_root(state)
    fog_state = rules.get("fog_health")
    if not isinstance(fog_state, dict):
        fog_state = {}
        rules["fog_health"] = fog_state
    fog_state["channel_id"] = channel_id
    fog_state.setdefault("managed_by_guard", False)
    fog_state.setdefault("disabled_by_guard", False)
    fog_state.setdefault("soft_failure_streak", 0)
    fog_state.setdefault("success_streak", 0)
    return fog_state


def quota_per_unit(conn: sqlite3.Connection) -> float:
    row = conn.execute("select value from options where key = 'QuotaPerUnit'").fetchone()
    if not row:
        return DEFAULT_QUOTA_PER_UNIT
    try:
        value = float(row[0])
    except (TypeError, ValueError):
        return DEFAULT_QUOTA_PER_UNIT
    return value if value > 0 else DEFAULT_QUOTA_PER_UNIT


def today_quota(conn: sqlite3.Connection, channel_id: int, start_ts: int, end_ts: int) -> int:
    row = conn.execute(
        """
        select coalesce(sum(quota), 0)
        from logs
        where channel_id = ?
          and type = ?
          and created_at >= ?
          and created_at < ?
        """,
        (channel_id, CONSUME_LOG_TYPE, start_ts, end_ts),
    ).fetchone()
    return int(row[0] or 0)


def latest_daily_limit_exceeded_log_id(
    conn: sqlite3.Connection, channel_id: int, start_ts: int, end_ts: int
) -> int | None:
    row = conn.execute(
        """
        select id
        from logs
        where channel_id = ?
          and type = ?
          and created_at >= ?
          and created_at < ?
          and (
            instr(upper(coalesce(content, '')), 'DAILY_LIMIT_EXCEEDED') > 0
            or instr(lower(coalesce(content, '')), 'daily usage limit exceeded') > 0
          )
        order by id desc
        limit 1
        """,
        (channel_id, ERROR_LOG_TYPE, start_ts, end_ts),
    ).fetchone()
    return int(row[0]) if row else None


def channel_status(conn: sqlite3.Connection, channel_id: int) -> int | None:
    row = conn.execute("select status from channels where id = ?", (channel_id,)).fetchone()
    return int(row[0]) if row else None


def channel_status_reason(conn: sqlite3.Connection, channel_id: int) -> str | None:
    row = conn.execute("select other_info from channels where id = ?", (channel_id,)).fetchone()
    if not row or not row[0]:
        return None
    try:
        parsed = json.loads(row[0])
    except json.JSONDecodeError:
        return None
    if not isinstance(parsed, dict):
        return None
    reason = parsed.get("status_reason")
    return reason if isinstance(reason, str) else None


def set_channel_enabled(
    conn: sqlite3.Connection, channel_id: int, enabled: bool, reason: str | None = None
) -> None:
    status = CHANNEL_STATUS_ENABLED if enabled else CHANNEL_STATUS_AUTO_DISABLED
    status_time = int(now_local().timestamp())

    row = conn.execute("select other_info from channels where id = ?", (channel_id,)).fetchone()
    info: dict[str, Any] = {}
    if row and row[0]:
        try:
            parsed = json.loads(row[0])
            if isinstance(parsed, dict):
                info = parsed
        except json.JSONDecodeError:
            info = {}
    if enabled:
        info.pop("status_reason", None)
        info.pop("status_time", None)
    else:
        info["status_reason"] = reason or "input daily budget guard"
        info["status_time"] = status_time

    conn.execute(
        "update channels set status = ?, other_info = ? where id = ?",
        (status, json.dumps(info, ensure_ascii=False, sort_keys=True, separators=(",", ":")), channel_id),
    )
    conn.execute("update abilities set enabled = ? where channel_id = ?", (1 if enabled else 0, channel_id))


def run_input_budget_rule(
    conn: sqlite3.Connection,
    state: dict[str, Any],
    channel_id: int,
    budget_usd: float,
    disable_at_usd: float,
    now: dt.datetime | None = None,
) -> dict[str, Any]:
    del budget_usd
    del disable_at_usd
    now = now or now_local()
    start_ts, end_ts, day = local_day_bounds(now)

    qpu = quota_per_unit(conn)
    quota = today_quota(conn, channel_id, start_ts, end_ts)
    spent_usd = quota / qpu
    limit_exceeded_log_id = latest_daily_limit_exceeded_log_id(conn, channel_id, start_ts, end_ts)
    status = channel_status(conn, channel_id)
    if status is None:
        return {"action": "missing", "reason": f"channel_id={channel_id} missing"}

    rules = ensure_rules_root(state)
    rule_state = rules.get("input_budget")
    if not isinstance(rule_state, dict):
        rule_state = {}
        rules["input_budget"] = rule_state
    stored_channel_id = rule_state.get("channel_id", channel_id)
    if not rule_state.get("disabled_by_guard") or stored_channel_id == channel_id:
        rule_state["channel_id"] = channel_id

    legacy_budget_owned_disable = (
        rule_state.get("disabled_by_guard")
        and stored_channel_id == channel_id
        and rule_state.get("disable_reason") == "budget threshold reached"
    )
    status_reason = channel_status_reason(conn, channel_id)

    disable_reason = None
    if limit_exceeded_log_id is not None:
        disable_reason = f"upstream daily limit exceeded (log_id={limit_exceeded_log_id})"

    if legacy_budget_owned_disable and disable_reason is None:
        if status == CHANNEL_STATUS_AUTO_DISABLED and status_reason == "input daily budget guard":
            conn.execute("begin immediate")
            set_channel_enabled(conn, channel_id, enabled=True)
            conn.commit()
            rule_state.update(
                {
                    "channel_id": channel_id,
                    "disabled_by_guard": False,
                    "restored_for_date": day,
                    "disable_reason": "",
                    "last_spent_usd": spent_usd,
                    "last_quota": quota,
                    "last_checked_date": day,
                    "last_limit_exceeded_log_id": limit_exceeded_log_id,
                }
            )
            return {"action": "enabled", "reason": "legacy budget disable cleared"}

        if status == CHANNEL_STATUS_ENABLED and status_reason == "input daily budget guard":
            conn.execute("begin immediate")
            set_channel_enabled(conn, channel_id, enabled=True)
            conn.commit()

        rule_state.update(
            {
                "channel_id": channel_id,
                "disabled_by_guard": False,
                "disable_reason": "",
                "last_spent_usd": spent_usd,
                "last_quota": quota,
                "last_checked_date": day,
                "last_limit_exceeded_log_id": limit_exceeded_log_id,
            }
        )
        return {"action": "none", "reason": "legacy budget ownership cleared"}

    if disable_reason is not None:
        if status == CHANNEL_STATUS_ENABLED:
            conn.execute("begin immediate")
            set_channel_enabled(conn, channel_id, enabled=False, reason="input daily budget guard")
            conn.commit()
            rule_state.update(
                {
                    "channel_id": channel_id,
                    "disabled_by_guard": True,
                    "disabled_for_date": day,
                    "previous_status": CHANNEL_STATUS_ENABLED,
                    "disable_reason": disable_reason,
                    "last_spent_usd": spent_usd,
                    "last_quota": quota,
                    "last_limit_exceeded_log_id": limit_exceeded_log_id,
                }
            )
            return {"action": "disabled", "reason": disable_reason}

        if rule_state.get("disabled_by_guard") and rule_state.get("disabled_for_date") == day:
            rule_state.update(
                {
                    "disable_reason": disable_reason,
                    "last_spent_usd": spent_usd,
                    "last_quota": quota,
                    "last_limit_exceeded_log_id": limit_exceeded_log_id,
                }
            )
            return {"action": "none", "reason": disable_reason}

        rule_state.update(
            {
                "disable_reason": disable_reason,
                "last_spent_usd": spent_usd,
                "last_quota": quota,
                "last_checked_date": day,
                "last_limit_exceeded_log_id": limit_exceeded_log_id,
            }
        )
        return {"action": "none", "reason": disable_reason}

    if (
        rule_state.get("disabled_by_guard")
        and stored_channel_id == channel_id
        and rule_state.get("disabled_for_date") != day
        and status == CHANNEL_STATUS_AUTO_DISABLED
    ):
        conn.execute("begin immediate")
        set_channel_enabled(conn, channel_id, enabled=True)
        conn.commit()
        rule_state.update(
            {
                "channel_id": channel_id,
                "disabled_by_guard": False,
                "restored_for_date": day,
                "disable_reason": "",
                "last_spent_usd": spent_usd,
                "last_quota": quota,
                "last_limit_exceeded_log_id": limit_exceeded_log_id,
            }
        )
        return {"action": "enabled", "reason": "new day"}

    if (
        rule_state.get("disabled_by_guard")
        and stored_channel_id == channel_id
        and rule_state.get("disabled_for_date") != day
        and status == CHANNEL_STATUS_ENABLED
    ):
        rule_state.update(
            {
                "channel_id": channel_id,
                "disabled_by_guard": False,
                "disable_reason": "",
                "last_spent_usd": spent_usd,
                "last_quota": quota,
                "last_checked_date": day,
                "last_limit_exceeded_log_id": limit_exceeded_log_id,
            }
        )
        return {"action": "none", "reason": "ownership cleared"}

    rule_state.update(
        {
            "last_spent_usd": spent_usd,
            "last_quota": quota,
            "last_checked_date": day,
            "last_limit_exceeded_log_id": limit_exceeded_log_id,
        }
    )
    return {"action": "none", "reason": "no action"}


def apply_fog_probe_result(rule_state: dict[str, Any], probe_result: ProbeResult) -> dict[str, Any]:
    rule_state["last_probe"] = {
        "status": probe_result.category,
        "http_status": probe_result.http_status,
        "message": probe_result.message,
        "checked_at": now_local().isoformat(timespec="seconds"),
    }
    if probe_result.category == "skipped":
        return {"action": "skipped", "reason": probe_result.message}
    if probe_result.ok:
        rule_state["soft_failure_streak"] = 0
        rule_state["success_streak"] = int(rule_state.get("success_streak", 0)) + 1
        return {"action": "success", "reason": probe_result.message}

    rule_state["success_streak"] = 0
    if probe_result.category == "hard_failure":
        rule_state["soft_failure_streak"] = 0
        return {"action": "disable_now", "reason": probe_result.message}

    rule_state["soft_failure_streak"] = int(rule_state.get("soft_failure_streak", 0)) + 1
    if rule_state["soft_failure_streak"] >= FOG_SOFT_FAILURE_THRESHOLD:
        return {"action": "disable_now", "reason": probe_result.message}
    return {"action": "none", "reason": probe_result.message}


def run_fog_health_rule(
    conn: sqlite3.Connection,
    state: dict[str, Any],
    channel_id: int,
    probe: Callable[[], ProbeResult],
) -> dict[str, Any]:
    rule_state = ensure_fog_rule_state(state, channel_id)
    status = channel_status(conn, channel_id)
    if status is None:
        return {"action": "missing", "reason": f"channel_id={channel_id} missing"}
    fog_owns_current_auto_disabled = (
        status == CHANNEL_STATUS_AUTO_DISABLED and channel_status_reason(conn, channel_id) == "fog health guard"
    )
    if status == CHANNEL_STATUS_ENABLED and rule_state.get("disabled_by_guard"):
        rule_state["disabled_by_guard"] = False
        rule_state["disable_reason"] = ""
    if status == CHANNEL_STATUS_AUTO_DISABLED and rule_state.get("disabled_by_guard") and not fog_owns_current_auto_disabled:
        rule_state["disabled_by_guard"] = False
        rule_state["disable_reason"] = ""

    probe_outcome = apply_fog_probe_result(rule_state, probe())
    if probe_outcome["action"] == "success":
        can_bootstrap = not rule_state.get("managed_by_guard") and not rule_state.get("disabled_by_guard")
        can_reenable = bool(rule_state.get("disabled_by_guard")) and fog_owns_current_auto_disabled
        if can_bootstrap and status == CHANNEL_STATUS_ENABLED:
            rule_state["success_streak"] = 0
            return {"action": "none", "reason": probe_outcome["reason"]}
        if rule_state.get("success_streak", 0) >= FOG_SUCCESS_THRESHOLD and can_reenable and status == CHANNEL_STATUS_AUTO_DISABLED:
            conn.execute("begin immediate")
            set_channel_enabled(conn, channel_id, enabled=True)
            conn.commit()
            rule_state["managed_by_guard"] = True
            rule_state["disabled_by_guard"] = False
            rule_state["disable_reason"] = ""
            return {"action": "enabled", "reason": probe_outcome["reason"]}
        return {"action": "none", "reason": probe_outcome["reason"]}

    if probe_outcome["action"] != "disable_now":
        return {"action": probe_outcome["action"], "reason": probe_outcome["reason"]}

    if status == CHANNEL_STATUS_ENABLED:
        conn.execute("begin immediate")
        set_channel_enabled(conn, channel_id, enabled=False, reason="fog health guard")
        conn.commit()
        rule_state["managed_by_guard"] = True
        rule_state["disabled_by_guard"] = True
        rule_state["disable_reason"] = probe_outcome["reason"]
        return {"action": "disabled", "reason": probe_outcome["reason"]}

    return {"action": "none", "reason": probe_outcome["reason"]}


def load_fog_probe_config(conn: sqlite3.Connection, channel_id: int) -> dict[str, Any]:
    row = conn.execute(
        """
        select id, "key", base_url, test_model, models, "group", model_mapping, other_info
        from channels
        where id = ?
        """,
        (channel_id,),
    ).fetchone()
    if row is None:
        raise ValueError(f"channel_id={channel_id} missing")

    model = row[3] or DEFAULT_FOG_MODEL
    base_url = row[2] or ""
    api_key = row[1] or ""
    if not isinstance(base_url, str) or not base_url.strip():
        raise ValueError(f"channel_id={channel_id} missing base_url")
    if not isinstance(api_key, str) or not api_key.strip():
        raise ValueError(f"channel_id={channel_id} missing key")
    if not isinstance(model, str) or not model.strip():
        raise ValueError(f"channel_id={channel_id} missing test_model")

    return {
        "channel_id": int(row[0]),
        "api_key": api_key,
        "base_url": base_url,
        "model": model,
        "models": row[4] or "",
        "group": row[5] or "",
        "model_mapping": row[6] or "",
        "other_info": row[7] or "",
    }


def probe_openai_chat(base_url: str, api_key: str, model: str, timeout_seconds: int = DEFAULT_FOG_TIMEOUT_SECONDS) -> ProbeResult:
    payload = json.dumps(
        {
            "model": model,
            "messages": [{"role": "user", "content": "ping"}],
            "max_tokens": 1,
            "temperature": 0,
        }
    ).encode("utf-8")
    request = urllib.request.Request(
        urllib.parse.urljoin(base_url.rstrip("/") + "/", "v1/chat/completions"),
        data=payload,
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {api_key}",
            "User-Agent": DEFAULT_FOG_USER_AGENT,
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout_seconds) as response:
            body = response.read().decode("utf-8", errors="replace")
            if response.status == 200 and '"choices"' in body:
                return ProbeResult(ok=True, category="success", http_status=200, message="ok")
            return ProbeResult(ok=False, category="soft_failure", http_status=response.status, message=body[:200])
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        message = f"status_code={exc.code}, {body[:200]}"
        hard_markers = (
            "invalid api key",
            "key group deleted",
            "permission denied",
            "not authorized",
            "daily limit exceeded",
            "balance too low",
            "quota exceeded",
        )
        body_lower = body.lower()
        if exc.code in (401, 403, 429) or any(marker in body_lower for marker in hard_markers):
            return ProbeResult(ok=False, category="hard_failure", http_status=exc.code, message=message)
        return ProbeResult(ok=False, category="soft_failure", http_status=exc.code, message=message)
    except Exception as exc:
        text = str(exc).lower()
        if (
            "timed out" in text
            or "cloudflare invalid or incomplete response" in text
            or "do request failed" in text
            or "connection" in text
        ):
            return ProbeResult(ok=False, category="soft_failure", http_status=None, message=str(exc))
        return ProbeResult(ok=False, category="hard_failure", http_status=None, message=str(exc))


def build_default_fog_probe(
    conn: sqlite3.Connection,
    *,
    channel_id: int = DEFAULT_FOG_CHANNEL_ID,
    probe_openai_chat_fn: Callable[[str, str, str, int], ProbeResult] = probe_openai_chat,
) -> Callable[[], ProbeResult]:
    config = load_fog_probe_config(conn, channel_id=channel_id)

    def _probe() -> ProbeResult:
        return probe_openai_chat_fn(
            config["base_url"],
            config["api_key"],
            config["model"],
            DEFAULT_FOG_TIMEOUT_SECONDS,
        )

    return _probe


def run_selected_rules(
    selected_rule: str,
    conn: sqlite3.Connection | None,
    state: dict[str, Any],
    input_rule_runner: Callable[..., dict[str, Any]],
    fog_rule_runner: Callable[..., dict[str, Any]],
    fog_probe: Callable[[], ProbeResult],
) -> int:
    if selected_rule in ("all", "input_budget"):
        input_rule_runner(
            conn,
            state,
            channel_id=DEFAULT_INPUT_CHANNEL_ID,
            budget_usd=DEFAULT_INPUT_BUDGET_USD,
            disable_at_usd=0.0,
        )
    if selected_rule in ("all", "fog_health"):
        fog_rule_runner(conn, state, channel_id=DEFAULT_FOG_CHANNEL_ID, probe=fog_probe)
    return 0


def run(
    args: argparse.Namespace,
    *,
    input_rule_runner: Callable[..., dict[str, Any]] = run_input_budget_rule,
    fog_rule_runner: Callable[..., dict[str, Any]] = run_fog_health_rule,
    fog_probe: Callable[[], ProbeResult] | None = None,
    probe_openai_chat_fn: Callable[[str, str, str, int], ProbeResult] = probe_openai_chat,
) -> int:
    state = load_state(args.state)
    migrate_legacy_input_budget_state(state, args.legacy_input_state)

    conn = sqlite3.connect(args.db, timeout=30)
    try:
        conn.execute("pragma busy_timeout = 30000")
        if args.rule in ("all", "input_budget"):
            input_result = input_rule_runner(
                conn,
                state,
                channel_id=DEFAULT_INPUT_CHANNEL_ID,
                budget_usd=DEFAULT_INPUT_BUDGET_USD,
                disable_at_usd=0.0,
            )
            save_state(args.state, state)
            append_log_line(args.log, "input_budget", input_result)

        if args.rule in ("all", "fog_health"):
            resolved_fog_probe = fog_probe
            if resolved_fog_probe is None:
                resolved_fog_probe = build_default_fog_probe(
                    conn,
                    channel_id=DEFAULT_FOG_CHANNEL_ID,
                    probe_openai_chat_fn=probe_openai_chat_fn,
                )
            fog_result = fog_rule_runner(
                conn,
                state,
                channel_id=DEFAULT_FOG_CHANNEL_ID,
                probe=resolved_fog_probe,
            )
            save_state(args.state, state)
            append_log_line(args.log, "fog_health", fog_result)

        return 0
    except Exception:
        try:
            conn.rollback()
        except Exception:
            pass
        raise
    finally:
        conn.close()


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Unified guard for NewAPI channels.")
    parser.add_argument("--db", type=Path, default=DEFAULT_DB_PATH)
    parser.add_argument("--state", type=Path, default=DEFAULT_STATE_PATH)
    parser.add_argument("--log", type=Path, default=DEFAULT_LOG_PATH)
    parser.add_argument("--legacy-input-state", type=Path, default=LEGACY_INPUT_STATE_PATH)
    parser.add_argument("--rule", choices=("all", "input_budget", "fog_health"), default="all")
    return parser.parse_args(argv)


if __name__ == "__main__":
    raise SystemExit(run(parse_args(sys.argv[1:])))
