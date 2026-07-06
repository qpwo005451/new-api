#!/usr/bin/env python3
import argparse
import datetime
import json
import sqlite3
import sys
from pathlib import Path


def load_json_object(raw: str, key: str) -> dict:
    text = (raw or '').strip()
    if not text:
        return {}
    try:
        data = json.loads(text)
    except json.JSONDecodeError as exc:
        raise SystemExit(f'{key}: invalid JSON in options table: {exc}')
    if not isinstance(data, dict):
        raise SystemExit(f'{key}: expected JSON object in options table')
    return data


def main() -> int:
    parser = argparse.ArgumentParser(description='Apply local option overrides to new-api options table')
    parser.add_argument('manifest', nargs='?', default='/opt/new-api/patches/local-option-overrides.json')
    parser.add_argument('--db', default='/opt/new-api/data/new-api.db')
    parser.add_argument('--backup-dir', default='/opt/new-api/data')
    args = parser.parse_args()

    manifest_path = Path(args.manifest)
    if not manifest_path.exists():
        raise SystemExit(f'manifest not found: {manifest_path}')

    manifest = json.loads(manifest_path.read_text(encoding='utf-8'))
    if not isinstance(manifest, dict):
        raise SystemExit('manifest root must be an object')

    conn = sqlite3.connect(args.db)
    cur = conn.cursor()

    backups = {}
    changes = []
    for option_key, patch in manifest.items():
        if not isinstance(patch, dict):
            raise SystemExit(f'{option_key}: override payload must be an object')

        row = cur.execute('select value from options where key = ?', (option_key,)).fetchone()
        current_raw = row[0] if row else ''
        current = load_json_object(current_raw, option_key)
        merged = dict(current)
        changed_keys = []
        for subkey, desired_value in patch.items():
            if merged.get(subkey) != desired_value:
                merged[subkey] = desired_value
                changed_keys.append(subkey)

        if not changed_keys:
            continue

        backups[option_key] = current
        payload = json.dumps(merged, ensure_ascii=False, separators=(',', ':'))
        if row:
            cur.execute('update options set value = ? where key = ?', (payload, option_key))
        else:
            cur.execute('insert into options(key, value) values(?, ?)', (option_key, payload))
        changes.append({'option': option_key, 'keys': changed_keys})

    if changes:
        backup_dir = Path(args.backup_dir)
        backup_dir.mkdir(parents=True, exist_ok=True)
        ts = datetime.datetime.now().strftime('%Y%m%d-%H%M%S')
        backup_path = backup_dir / f'local-option-overrides-backup-{ts}.json'
        backup_path.write_text(json.dumps(backups, ensure_ascii=False, indent=2), encoding='utf-8')
        conn.commit()
        print(f'updated {len(changes)} option(s)')
        print(f'backup={backup_path}')
        for item in changes:
            print(f"{item['option']}: {', '.join(item['keys'])}")
    else:
        print('no changes needed')

    conn.close()
    return 0


if __name__ == '__main__':
    sys.exit(main())
