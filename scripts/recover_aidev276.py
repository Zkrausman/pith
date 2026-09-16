"""One-off, stdlib-only AIDEV-276 recovery; not imported or shipped by Pith.

CLI paths and historical bounds are intentionally fixed. See docs/aidev-276-recovery.md.
Never print SQLite exception text: it could contain stored values.
"""
import argparse
from contextlib import closing
from dataclasses import dataclass
import hashlib
import json
import os
from pathlib import Path
import sqlite3
import sys
import time


TABLE_SQL = """CREATE TABLE executions (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
 command TEXT, original_tokens INTEGER, compressed_tokens INTEGER,
 original_content TEXT, compressed_content TEXT, duration_ms INTEGER,
 parser_used TEXT, is_passthrough BOOLEAN, source TEXT DEFAULT 'unknown',
 harness TEXT DEFAULT 'unknown', model TEXT DEFAULT 'unknown',
 input_cost_per_million REAL
);"""
INDEX_SQL = """CREATE UNIQUE INDEX idx_executions_unique
 ON executions(timestamp, command, duration_ms);"""
FTS_SQL = """CREATE VIRTUAL TABLE executions_fts USING fts5(
 command, original_content, compressed_content,
 content='executions', content_rowid='id');
CREATE TRIGGER executions_ai AFTER INSERT ON executions BEGIN
 INSERT INTO executions_fts(rowid, command, original_content, compressed_content)
 VALUES (new.id, new.command, new.original_content, new.compressed_content);
END;
CREATE TRIGGER executions_ad AFTER DELETE ON executions BEGIN
 INSERT INTO executions_fts(executions_fts, rowid, command, original_content, compressed_content)
 VALUES ('delete', old.id, old.command, old.original_content, old.compressed_content);
END;
CREATE TRIGGER executions_au AFTER UPDATE ON executions BEGIN
 INSERT INTO executions_fts(executions_fts, rowid, command, original_content, compressed_content)
 VALUES ('delete', old.id, old.command, old.original_content, old.compressed_content);
 INSERT INTO executions_fts(rowid, command, original_content, compressed_content)
 VALUES (new.id, new.command, new.original_content, new.compressed_content);
END;
"""
SCHEMA_SQL = TABLE_SQL + INDEX_SQL + FTS_SQL
LEDGER = 'aidev276_recovery'
LEDGER_SQL = f"""CREATE TABLE {LEDGER} (
 source_fingerprint TEXT PRIMARY KEY,
 source_slot INTEGER NOT NULL UNIQUE,
 imported_count INTEGER NOT NULL,
 first_destination_id INTEGER NOT NULL,
 last_destination_id INTEGER NOT NULL
)"""
SAFE_COLUMNS = ('timestamp', 'original_tokens', 'compressed_tokens', 'duration_ms',
                'parser_used', 'is_passthrough', 'source', 'harness', 'model',
                'input_cost_per_million')
SAFE_SELECT = ','.join(('id',) + SAFE_COLUMNS)
DESTINATION = Path('C:/Users/zkrau/.pith/pith.db')


@dataclass(frozen=True)
class Source:
    path: Path
    count: int
    first: str
    last: str


SOURCES = (
    Source(Path('E:/pith-worktrees/pith-release-v243-ZHKk44/storage/pith.db'),
           27649, '2026-08-11 06:33:00', '2026-09-16 15:13:24'),
    Source(Path('E:/pith-worktrees/pith-release-v244-8bhFdO/storage/pith.db'),
           99, '2026-09-16 15:13:39', '2026-09-16 15:44:07'),
)
LIVE_FIRST = '2026-09-16 15:44:22'


class RecoveryError(Exception):
    """Only fixed, non-sensitive messages may be raised here."""


def require(ok, message):
    if not ok:
        raise RecoveryError(message)


def connect(path, *, readonly=True, timeout=3):
    mode = 'ro' if readonly else 'rw'  # Never create a missing input database.
    conn = sqlite3.connect(Path(path).resolve().as_uri() + '?mode=' + mode,
                           uri=True, timeout=timeout, isolation_level=None)
    if readonly:
        conn.execute('PRAGMA query_only=ON')
    return conn


def normalize(sql):
    return ''.join(sql.split()).rstrip(';').lower() if sql else None


def schema(conn, alias):
    # GLOB treats '_' literally: legal names such as sqliteXunexpected must
    # remain visible to the allowlist. Exclude only SQLite's reserved prefix.
    objects = conn.execute(f"SELECT type,name,tbl_name,sql FROM {alias}.sqlite_master "
                           "WHERE lower(name) NOT GLOB 'sqlite_*' ORDER BY type,name").fetchall()
    return [(kind, name, table, normalize(sql)) for kind, name, table, sql in objects]


def check_schema(conn, alias, expected):
    objects = schema(conn, alias)
    if alias == 'main' and any(row[1] == LEDGER for row in objects):
        raise RecoveryError('Recovery ledger already exists; rerun refused.')
    require(objects == expected, 'Unexpected schema, index, or FTS trigger; refused.')
    # Explicit PRAGMA metadata validation in addition to the SQL allowlist.
    with closing(sqlite3.connect(':memory:')) as reference:
        reference.executescript(SCHEMA_SQL)
        for pragma in ('table_info(executions)', 'index_list(executions)',
                       'index_xinfo(idx_executions_unique)'):
            require(conn.execute(f'PRAGMA {alias}.{pragma}').fetchall() ==
                    reference.execute(f'PRAGMA {pragma}').fetchall(),
                    'Unexpected column or index metadata; refused.')


def integrity(conn, alias):
    require(conn.execute(f'PRAGMA {alias}.integrity_check').fetchall() == [('ok',)],
            'SQLite integrity check failed; refused.')


def fts_integrity(conn):
    # rank=1 checks external-content consistency too. No results expose text.
    conn.execute("INSERT INTO executions_fts(executions_fts, rank) "
                 "VALUES ('integrity-check', 1)")


def bounds(conn, alias):
    result = conn.execute(f'SELECT count(*),min(timestamp),max(timestamp) '
                          f'FROM {alias}.executions').fetchone()
    invalid = conn.execute(f"SELECT count(*) FROM {alias}.executions WHERE "
                           "typeof(timestamp) != 'text' OR length(timestamp) != 19 "
                           "OR strftime('%Y-%m-%d %H:%M:%S',timestamp) IS NULL "
                           "OR strftime('%Y-%m-%d %H:%M:%S',timestamp) != timestamp").fetchone()[0]
    require(not invalid and result[0] > 0, 'Missing or noncanonical timestamps; refused.')
    return result


def fingerprint(conn, alias):
    digest = hashlib.sha256(b'AIDEV-276-safe-metadata-v1\n')
    # Include original IDs to retain multiplicity. Raw text is not read or hashed.
    for row in conn.execute(f'SELECT {SAFE_SELECT} FROM {alias}.executions ORDER BY id'):
        digest.update(json.dumps(row, ensure_ascii=True, allow_nan=False,
                                 separators=(',', ':')).encode('ascii') + b'\n')
    return digest.hexdigest()


def bounded_backup(source, target, seconds):
    deadline = time.monotonic() + seconds

    def progress(status, remaining, total):
        require(time.monotonic() < deadline, 'Backup deadline exceeded; refused.')

    source.backup(target, pages=256, progress=progress, sleep=0.05)
    # Copy and verification share one cooperative deadline, including dry-run.
    target.set_progress_handler(lambda: int(time.monotonic() >= deadline), 1000)
    try:
        require(time.monotonic() < deadline, 'Backup deadline exceeded; refused.')
        integrity(target, 'main')
        require(time.monotonic() < deadline, 'Backup deadline exceeded; refused.')
    finally:
        target.set_progress_handler(None, 0)


def backup_live(destination, backup, timeout, seconds):
    backup = Path(backup).resolve()
    require(backup.parent.is_dir(), 'Backup directory must already exist.')
    # Exclusive creation also refuses existing symlinks/files; do not overwrite.
    fd = os.open(backup, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    os.close(fd)
    # A failed/partial backup stays local for inspection, never reused automatically.
    with closing(connect(destination, timeout=timeout)) as source:
        with closing(sqlite3.connect(backup, isolation_level=None)) as target:
            bounded_backup(source, target, seconds)


def import_transaction(conn, sources, live_first, timeout, seconds):
    """Same transaction for apply and in-memory dry-run. Return only aggregate counts."""
    conn.execute(f'PRAGMA busy_timeout={int(timeout * 1000)}')
    for i, source in enumerate(sources):
        conn.execute(f'ATTACH DATABASE ? AS src{i}',
                     (source.path.resolve().as_uri() + '?mode=ro',))
    with closing(sqlite3.connect(':memory:')) as reference:
        reference.executescript(SCHEMA_SQL)
        expected = schema(reference, 'main')
    deadline = time.monotonic() + seconds
    conn.set_progress_handler(lambda: int(time.monotonic() >= deadline), 1000)
    try:
        conn.execute('BEGIN IMMEDIATE')
        # Source read snapshots stay pinned until commit; revalidate after backup/lock.
        check_schema(conn, 'main', expected)
        integrity(conn, 'main')
        live_count, first, _ = bounds(conn, 'main')
        require(first == live_first, 'Live lower time bound changed; refused.')
        high_id = conn.execute('SELECT max(id) FROM executions').fetchone()[0]
        previous_last = None
        fingerprints = []
        for i, source in enumerate(sources):
            alias = f'src{i}'
            check_schema(conn, alias, expected)
            integrity(conn, alias)
            require(bounds(conn, alias) == (source.count, source.first, source.last),
                    'Source count or time bounds changed; refused.')
            require(source.first <= source.last < first and
                    (previous_last is None or previous_last < source.first),
                    'Source/live time ranges overlap; refused.')
            previous_last = source.last
            fingerprints.append(fingerprint(conn, alias))
        require(len(set(fingerprints)) == len(sources), 'Duplicate source; refused.')
        fts_integrity(conn)
        conn.execute(LEDGER_SQL)
        for i, (source, digest) in enumerate(zip(sources, fingerprints)):
            before_id = conn.execute('SELECT max(id) FROM executions').fetchone()[0]
            columns = ','.join(SAFE_COLUMNS)
            # Never SELECT the historical command or either content column.
            conn.execute(f"INSERT INTO executions ({columns},command,original_content,compressed_content) "
                         f"SELECT {columns}, ? || CAST(id AS TEXT) || ']', '', '' "
                         f'FROM src{i}.executions ORDER BY id',
                         (f'[recovered metadata:aidev-276:{digest}:',))
            require(conn.execute('SELECT changes()').fetchone()[0] == source.count,
                    'Imported count mismatch; rollback required.')
            first_id, last_id = conn.execute('SELECT min(id),max(id) FROM executions WHERE id>?',
                                             (before_id,)).fetchone()
            conn.execute(f'INSERT INTO {LEDGER} VALUES (?,?,?,?,?)',
                         (digest, i, source.count, first_id, last_id))
        imported = sum(source.count for source in sources)
        total = conn.execute('SELECT count(*) FROM executions').fetchone()[0]
        require(total == live_count + imported, 'Final count mismatch; rollback required.')
        require(conn.execute('SELECT count(*) FROM executions WHERE id<=?',
                             (high_id,)).fetchone()[0] == live_count,
                'Live row count changed; rollback required.')
        fts_integrity(conn)
        integrity(conn, 'main')
        require(time.monotonic() < deadline, 'Transaction deadline exceeded; rollback required.')
        conn.execute('COMMIT')
        return {'live_rows_before': live_count, 'imported_rows': imported,
                'rows_after': total, 'sources': len(sources)}
    except BaseException:
        # Disable expired handler so rollback itself cannot be interrupted.
        conn.set_progress_handler(None, 0)
        if conn.in_transaction:
            conn.execute('ROLLBACK')
        raise
    finally:
        conn.set_progress_handler(None, 0)


def recover(destination, sources, live_first, *, backup=None, timeout=3, seconds=60):
    """No backup argument means read-only live access + an in-memory rehearsal."""
    require(0 < timeout <= 10 and 0 < seconds <= 300, 'Invalid bounded timeout.')
    require(len(sources) == 2, 'Exactly two approved sources required.')
    paths = [Path(destination).resolve()] + [s.path.resolve() for s in sources]
    require(all(p.is_file() for p in paths), 'Missing input database; refused.')
    require(all(not os.path.samefile(a, b) for i, a in enumerate(paths) for b in paths[i+1:]),
            'Input database paths alias each other; refused.')
    if backup is None:
        with closing(connect(destination, timeout=timeout)) as live:
            with closing(sqlite3.connect(':memory:', isolation_level=None, uri=True)) as rehearsal:
                bounded_backup(live, rehearsal, seconds)
                return import_transaction(rehearsal, sources, live_first, timeout, seconds)
    backup_live(destination, backup, timeout, seconds)
    with closing(connect(destination, readonly=False, timeout=timeout)) as live:
        return import_transaction(live, sources, live_first, timeout, seconds)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument('--dry-run', action='store_true')
    mode.add_argument('--apply', action='store_true')
    parser.add_argument('--backup', type=Path, help='New local backup filename, required with --apply')
    args = parser.parse_args()
    if args.apply != (args.backup is not None):
        parser.error('--apply requires --backup; --dry-run must not specify it')
    try:
        result = recover(DESTINATION, SOURCES, LIVE_FIRST, backup=args.backup)
        print(json.dumps(result, sort_keys=True))
        return 0
    except (RecoveryError, sqlite3.Error, OSError, ValueError):
        # Suppress paths, stored text, tracebacks, and exception details.
        print('Recovery refused or rolled back; no success claimed. Review locally.', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
