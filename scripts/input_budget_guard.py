#!/usr/bin/env python3
"""Daily budget guard for the input channel.

This is intentionally a small sidecar: it only touches channel 9 and records
state so it can restore only the disable action it made itself.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import sqlite3
import sys
from pathlib import Path
from typing import Any


DEFAULT_DB_PATH = Path("/opt/new-api/data/new-api.db")
DEFAULT_STATE_PATH = Path("/opt/new-api/data/input_budget_guard_state.json")
DEFAULT_CHANNEL_ID = 9
DEFAULT_BUDGET_USD = 300.0
DEFAULT_DISABLE_AT_USD = 299.0
DEFAULT_QUOTA_PER_UNIT = 500000.0
STATE_VERSION = 1
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


def load_state(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {}
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except Exception:
        return {}


def save_state(path: Path, state: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(json.dumps(state, ensure_ascii=False, sort_keys=True, indent=2) + "\n", encoding="utf-8")
    os.replace(tmp, path)


def log(message: str) -> None:
    print(f"{now_local().isoformat(timespec='seconds')} {message}", flush=True)


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


def latest_daily_limit_exceeded_log_id(conn: sqlite3.Connection, channel_id: int, start_ts: int, end_ts: int) -> int | None:
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


def set_channel_enabled(conn: sqlite3.Connection, channel_id: int, enabled: bool) -> None:
    status = CHANNEL_STATUS_ENABLED if enabled else CHANNEL_STATUS_AUTO_DISABLED
    reason = "" if enabled else "input daily budget guard"
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
        info["status_reason"] = reason
        info["status_time"] = status_time

    conn.execute(
        "update channels set status = ?, other_info = ? where id = ?",
        (status, json.dumps(info, ensure_ascii=False, sort_keys=True, separators=(",", ":")), channel_id),
    )
    conn.execute("update abilities set enabled = ? where channel_id = ?", (1 if enabled else 0, channel_id))


def run(args: argparse.Namespace) -> int:
    state_path = Path(args.state)
    state = load_state(state_path)
    start_ts, end_ts, day = local_day_bounds(now_local())

    conn = sqlite3.connect(args.db, timeout=30)
    try:
        conn.execute("pragma busy_timeout = 30000")
        qpu = quota_per_unit(conn)
        quota = today_quota(conn, args.channel_id, start_ts, end_ts)
        limit_exceeded_log_id = latest_daily_limit_exceeded_log_id(conn, args.channel_id, start_ts, end_ts)
        spent_usd = quota / qpu
        status = channel_status(conn, args.channel_id)
        if status is None:
            log(f"channel_id={args.channel_id} missing; no action")
            return 2

        log(
            "channel_id=%d day=%s quota=%d quota_per_unit=%.0f spent_usd=%.4f budget_usd=%.2f disable_at_usd=%.2f status=%d limit_exceeded_log_id=%s"
            % (args.channel_id, day, quota, qpu, spent_usd, args.budget_usd, args.disable_at_usd, status, limit_exceeded_log_id or "-")
        )

        disable_reason = None
        if spent_usd >= args.disable_at_usd:
            disable_reason = "budget threshold reached"
        elif limit_exceeded_log_id is not None:
            disable_reason = f"upstream daily limit exceeded (log_id={limit_exceeded_log_id})"

        if disable_reason is not None:
            if status == CHANNEL_STATUS_ENABLED:
                conn.execute("begin immediate")
                set_channel_enabled(conn, args.channel_id, enabled=False)
                conn.commit()
                state.update(
                    {
                        "version": STATE_VERSION,
                        "channel_id": args.channel_id,
                        "disabled_by_guard": True,
                        "disabled_for_date": day,
                        "previous_status": CHANNEL_STATUS_ENABLED,
                        "disable_reason": disable_reason,
                        "last_spent_usd": spent_usd,
                        "last_quota": quota,
                        "last_limit_exceeded_log_id": limit_exceeded_log_id,
                    }
                )
                save_state(state_path, state)
                log(f"disabled channel_id={args.channel_id}; previous_status={CHANNEL_STATUS_ENABLED}; reason={disable_reason}")
            elif state.get("disabled_by_guard") and state.get("disabled_for_date") == day:
                state.update(
                    {
                        "version": STATE_VERSION,
                        "channel_id": args.channel_id,
                        "disable_reason": disable_reason,
                        "last_spent_usd": spent_usd,
                        "last_quota": quota,
                        "last_limit_exceeded_log_id": limit_exceeded_log_id,
                    }
                )
                save_state(state_path, state)
                log(f"channel_id={args.channel_id} remains disabled by this guard; reason={disable_reason}")
            else:
                state.update(
                    {
                        "version": STATE_VERSION,
                        "channel_id": args.channel_id,
                        "disable_reason": disable_reason,
                        "last_spent_usd": spent_usd,
                        "last_quota": quota,
                        "last_checked_date": day,
                        "last_limit_exceeded_log_id": limit_exceeded_log_id,
                    }
                )
                save_state(state_path, state)
                log(f"channel_id={args.channel_id} already disabled by another path; no ownership taken; reason={disable_reason}")
            return 0

        was_disabled_by_guard = bool(state.get("disabled_by_guard"))
        disabled_for_date = state.get("disabled_for_date")
        state_channel_id = state.get("channel_id", args.channel_id)
        if was_disabled_by_guard and state_channel_id == args.channel_id and disabled_for_date != day and status == CHANNEL_STATUS_AUTO_DISABLED:
            conn.execute("begin immediate")
            set_channel_enabled(conn, args.channel_id, enabled=True)
            conn.commit()
            state.update(
                {
                    "version": STATE_VERSION,
                    "channel_id": args.channel_id,
                    "disabled_by_guard": False,
                    "restored_for_date": day,
                    "disable_reason": "",
                    "last_spent_usd": spent_usd,
                    "last_quota": quota,
                    "last_limit_exceeded_log_id": limit_exceeded_log_id,
                }
            )
            save_state(state_path, state)
            log(f"re-enabled channel_id={args.channel_id} for new day {day}")
        else:
            state.update(
                {
                    "version": STATE_VERSION,
                    "channel_id": args.channel_id,
                    "last_spent_usd": spent_usd,
                    "last_quota": quota,
                    "last_checked_date": day,
                    "last_limit_exceeded_log_id": limit_exceeded_log_id,
                }
            )
            save_state(state_path, state)
            log("no action")
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
    parser = argparse.ArgumentParser(description="Disable input channel when today's logged quota reaches the daily budget.")
    parser.add_argument("--db", default=str(DEFAULT_DB_PATH))
    parser.add_argument("--state", default=str(DEFAULT_STATE_PATH))
    parser.add_argument("--channel-id", type=int, default=DEFAULT_CHANNEL_ID)
    parser.add_argument("--budget-usd", type=float, default=DEFAULT_BUDGET_USD)
    parser.add_argument("--disable-at-usd", type=float, default=DEFAULT_DISABLE_AT_USD)
    return parser.parse_args(argv)


if __name__ == "__main__":
    raise SystemExit(run(parse_args(sys.argv[1:])))
