#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import sqlite3
import sys
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
CONSUME_LOG_TYPE = 2
ERROR_LOG_TYPE = 5
CHANNEL_STATUS_ENABLED = 1
CHANNEL_STATUS_AUTO_DISABLED = 3


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


def run(
    args: argparse.Namespace,
    *,
    input_rule_runner: Callable[..., dict[str, Any]] = run_input_budget_rule,
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
    parser = argparse.ArgumentParser(description="Guard the input channel against upstream daily-limit failures.")
    parser.add_argument("--db", type=Path, default=DEFAULT_DB_PATH)
    parser.add_argument("--state", type=Path, default=DEFAULT_STATE_PATH)
    parser.add_argument("--log", type=Path, default=DEFAULT_LOG_PATH)
    parser.add_argument("--legacy-input-state", type=Path, default=LEGACY_INPUT_STATE_PATH)
    parser.add_argument("--rule", choices=("all", "input_budget"), default="all")
    return parser.parse_args(argv)


if __name__ == "__main__":
    raise SystemExit(run(parse_args(sys.argv[1:])))
