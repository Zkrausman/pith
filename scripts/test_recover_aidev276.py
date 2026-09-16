"""Synthetic-only recovery tests. Never opens Pith or any real telemetry path."""
from contextlib import closing
from dataclasses import replace
from pathlib import Path
import sqlite3
import tempfile
import threading
import time
import unittest
from unittest.mock import patch

import recover_aidev276 as recovery


class RecoveryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='aidev276-synthetic-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.live = self.root / 'live.db'
        self.backup = self.root / 'backup.db'
        self.first = '2026-09-16 15:44:22'
        self.sources = (
            recovery.Source(self.root / 'first.db', 3, '2026-08-11 06:33:00', '2026-09-16 15:13:24'),
            recovery.Source(self.root / 'second.db', 2, '2026-09-16 15:13:39', '2026-09-16 15:44:07'),
        )
        for path in [self.live] + [s.path for s in self.sources]:
            with closing(sqlite3.connect(path)) as conn:
                conn.executescript(recovery.SCHEMA_SQL)
        self.insert(self.live, self.first, 'live secret')
        self.insert(self.sources[0].path, self.sources[0].first, 'synthetic secret A')
        # Legitimate metadata collision: every safe field but ID matches.
        self.insert(self.sources[0].path, self.sources[0].first, 'synthetic secret B')
        self.insert(self.sources[0].path, self.sources[0].last, 'synthetic secret C')
        self.insert(self.sources[1].path, self.sources[1].first, 'synthetic secret D')
        self.insert(self.sources[1].path, self.sources[1].last, 'synthetic secret E')

    @staticmethod
    def insert(path, timestamp, command):
        with closing(sqlite3.connect(path, timeout=3)) as conn:
            conn.execute('INSERT INTO executions(timestamp,command,original_tokens,compressed_tokens,'
                         'original_content,compressed_content,duration_ms,parser_used,is_passthrough,'
                         'source,harness,model,input_cost_per_million) VALUES (?,?,100,30,?,?,5,?,0,?,?,?,2.5)',
                         (timestamp, command, 'synthetic raw', 'synthetic compressed', 'git', 'pi', 'pi', 'test'))
            conn.commit()

    @staticmethod
    def rows(path, query='SELECT * FROM executions ORDER BY id'):
        with closing(sqlite3.connect(path)) as conn:
            return conn.execute(query).fetchall()

    def apply(self, **kwargs):
        return recovery.recover(self.live, self.sources, self.first, backup=self.backup, **kwargs)

    def assert_no_ledger(self):
        self.assertEqual(self.rows(self.live, "SELECT count(*) FROM sqlite_master WHERE name='aidev276_recovery'"), [(0,)])

    def test_apply_preserves_live_metadata_multiplicity_sources_and_backup(self):
        before = self.rows(self.live)
        source_bytes = [s.path.read_bytes() for s in self.sources]
        result = self.apply()
        self.assertEqual(result, {'live_rows_before': 1, 'imported_rows': 5, 'rows_after': 6, 'sources': 2})
        after = self.rows(self.live)
        self.assertEqual(after[:1], before)
        self.assertEqual(self.rows(self.backup), before)
        self.assertEqual([s.path.read_bytes() for s in self.sources], source_bytes)
        expected = []
        for source in self.sources:
            expected.extend(self.rows(source.path, f'SELECT {",".join(recovery.SAFE_COLUMNS)} FROM executions ORDER BY id'))
        self.assertEqual(self.rows(self.live, f'SELECT {",".join(recovery.SAFE_COLUMNS)} FROM executions WHERE id>1 ORDER BY id'), expected)
        self.assertEqual(len({r[2] for r in after[1:]}), 5)
        for row in after[1:]:
            self.assertTrue(row[2].startswith('[recovered metadata:aidev-276:'))
            self.assertNotIn('secret', row[2])
            self.assertEqual(row[5:7], ('', ''))
        self.assertEqual(self.rows(self.live, 'SELECT sum(imported_count) FROM aidev276_recovery'), [(5,)])
        with closing(sqlite3.connect(self.live)) as conn:
            recovery.fts_integrity(conn)
            self.assertEqual(conn.execute("SELECT count(*) FROM executions_fts WHERE executions_fts MATCH 'recovered'").fetchone(), (5,))
        # Normal live writes and existing uniqueness constraints still work.
        self.insert(self.live, '2026-09-16 16:00:00', 'new live command')

    def test_dry_run_changes_no_input_bytes_or_creates_backup(self):
        paths = [self.live] + [s.path for s in self.sources]
        before = [p.read_bytes() for p in paths]
        self.assertEqual(recovery.recover(self.live, self.sources, self.first)['imported_rows'], 5)
        self.assertEqual([p.read_bytes() for p in paths], before)
        self.assertFalse(self.backup.exists())
        self.assert_no_ledger()

    def test_post_insert_failure_rolls_back_rows_ledger_sequence_and_fts(self):
        before = self.rows(self.live)
        sequence = self.rows(self.live, 'SELECT * FROM sqlite_sequence')
        original = recovery.fts_integrity
        calls = 0

        def fail_second(conn):
            nonlocal calls
            calls += 1
            original(conn)
            if calls == 2:
                raise recovery.RecoveryError('Synthetic failure.')

        with patch.object(recovery, 'fts_integrity', side_effect=fail_second):
            with self.assertRaises(recovery.RecoveryError):
                self.apply()
        self.assertEqual(self.rows(self.live), before)
        self.assertEqual(self.rows(self.live, 'SELECT * FROM sqlite_sequence'), sequence)
        self.assert_no_ledger()
        with closing(sqlite3.connect(self.live)) as conn:
            recovery.fts_integrity(conn)

    def test_rerun_refused_even_when_source_fingerprint_changes(self):
        self.apply()
        before = self.rows(self.live)
        with closing(sqlite3.connect(self.sources[0].path)) as conn:
            conn.execute('UPDATE executions SET original_tokens=101 WHERE id=1')
            conn.commit()
        with self.assertRaisesRegex(recovery.RecoveryError, 'ledger'):
            recovery.recover(self.live, self.sources, self.first, backup=self.root / 'second-backup.db')
        self.assertEqual(self.rows(self.live), before)

    def test_source_overlap_refused(self):
        source = self.sources[1]
        overlap = self.sources[0].last
        with closing(sqlite3.connect(source.path)) as conn:
            conn.execute('UPDATE executions SET timestamp=? WHERE id=1', (overlap,))
            conn.commit()
        self.sources = (self.sources[0], replace(source, first=overlap))
        with self.assertRaisesRegex(recovery.RecoveryError, 'overlap'):
            self.apply()
        self.assertEqual(len(self.rows(self.live)), 1)
        self.assert_no_ledger()

    def test_live_overlap_refused(self):
        self.insert(self.live, self.sources[1].last, 'overlapping live record')
        with self.assertRaisesRegex(recovery.RecoveryError, 'lower time bound'):
            self.apply()
        self.assertEqual(len(self.rows(self.live)), 2)

    def test_count_and_time_bounds_refused(self):
        for change in ({'count': 4}, {'last': '2026-09-16 15:13:25'}):
            sources = (replace(self.sources[0], **change), self.sources[1])
            with self.assertRaisesRegex(recovery.RecoveryError, 'count or time'):
                recovery.recover(self.live, sources, self.first)
        self.assert_no_ledger()

    def test_source_column_mismatch_refused(self):
        with closing(sqlite3.connect(self.sources[0].path)) as conn:
            conn.execute('ALTER TABLE executions ADD COLUMN unexpected TEXT')
            conn.commit()
        with self.assertRaisesRegex(recovery.RecoveryError, 'schema'):
            recovery.recover(self.live, self.sources, self.first)
        self.assert_no_ledger()

    def test_missing_source_trigger_refused(self):
        # Independent fixture: no preceding column mismatch can mask this check.
        with closing(sqlite3.connect(self.sources[0].path)) as conn:
            conn.execute('DROP TRIGGER executions_ai')
            conn.commit()
        with self.assertRaisesRegex(recovery.RecoveryError, 'schema'):
            recovery.recover(self.live, self.sources, self.first)
        self.assert_no_ledger()

    def test_source_attachment_is_read_only(self):
        def attempt_write(conn):
            conn.execute('UPDATE src0.executions SET original_tokens=0')
        with patch.object(recovery, 'fts_integrity', side_effect=attempt_write):
            with self.assertRaises(sqlite3.OperationalError):
                self.apply()
        self.assert_no_ledger()

    def test_inconsistent_destination_fts_refused(self):
        with closing(sqlite3.connect(self.live)) as conn:
            conn.execute("INSERT INTO executions_fts(executions_fts) VALUES ('delete-all')")
            conn.commit()
        with self.assertRaises(sqlite3.DatabaseError):
            self.apply()
        self.assertEqual(len(self.rows(self.live)), 1)
        self.assert_no_ledger()

    def test_live_commit_between_backup_and_lock_preserved(self):
        original = recovery.backup_live

        def backup_then_live_commit(*args):
            original(*args)
            self.insert(self.live, '2026-09-16 16:00:00', 'new concurrent live record')

        with patch.object(recovery, 'backup_live', side_effect=backup_then_live_commit):
            result = self.apply()
        self.assertEqual(result['live_rows_before'], 2)
        self.assertEqual(result['rows_after'], 7)
        self.assertEqual(len(self.rows(self.backup)), 1)
        self.assertEqual(self.rows(self.live)[1][2], 'new concurrent live record')

    def test_busy_writer_timeout_no_changes(self):
        before = self.rows(self.live)
        with closing(sqlite3.connect(self.live, isolation_level=None)) as writer:
            writer.execute('BEGIN IMMEDIATE')
            started = time.monotonic()
            with self.assertRaises(sqlite3.OperationalError):
                self.apply(timeout=0.1)
            self.assertLess(time.monotonic() - started, 3)
            writer.execute('ROLLBACK')
        self.assertEqual(self.rows(self.live), before)
        self.assert_no_ledger()

    def test_concurrent_writer_waits_for_import_commit(self):
        entered = threading.Event()
        finished = threading.Event()
        errors = []
        original = recovery.fts_integrity
        worker = None

        def write():
            entered.set()
            try:
                self.insert(self.live, '2026-09-16 16:00:00', 'waiting live record')
            except Exception as error:
                errors.append(type(error).__name__)
            finally:
                finished.set()

        def check(conn):
            nonlocal worker
            if worker is None:
                worker = threading.Thread(target=write)
                worker.start()
                self.assertTrue(entered.wait(1))
                self.assertFalse(finished.wait(0.1))
            original(conn)

        try:
            with patch.object(recovery, 'fts_integrity', side_effect=check):
                self.apply()
        finally:
            if worker:
                worker.join(5)
        self.assertTrue(finished.is_set())
        self.assertEqual(errors, [])
        self.assertEqual(len(self.rows(self.live)), 7)

    def test_deadline_rolls_back(self):
        with self.assertRaises((recovery.RecoveryError, sqlite3.OperationalError)):
            self.apply(seconds=0.000001)
        self.assertEqual(len(self.rows(self.live)), 1)
        self.assert_no_ledger()

    def test_backup_integrity_shares_copy_deadline(self):
        # Enough pages/opcodes to exercise the actual integrity_check handler.
        with closing(sqlite3.connect(self.live)) as conn:
            conn.executemany('INSERT INTO executions(timestamp,command) VALUES (?,?)',
                             [(self.first, f'synthetic deadline {i}') for i in range(2000)])
            conn.commit()
        before = self.rows(self.live)
        original = recovery.integrity
        clock = 0

        def expired_integrity(conn, alias):
            nonlocal clock
            clock = 2  # Copy succeeded; verification exhausts the shared second.
            original(conn, alias)

        with patch.object(recovery.time, 'monotonic', side_effect=lambda: clock):
            with patch.object(recovery, 'integrity', side_effect=expired_integrity):
                with patch.object(recovery, 'import_transaction') as import_transaction:
                    with self.assertRaisesRegex(sqlite3.OperationalError, 'interrupted'):
                        self.apply(seconds=1)
                    import_transaction.assert_not_called()
        self.assertEqual(self.rows(self.live), before)
        self.assert_no_ledger()

    def test_transaction_deadline_after_successful_backup(self):
        original = recovery.backup_live

        def allow_backup(destination, backup, timeout, seconds):
            original(destination, backup, timeout, 60)

        with patch.object(recovery, 'backup_live', side_effect=allow_backup):
            with self.assertRaises((recovery.RecoveryError, sqlite3.OperationalError)):
                self.apply(seconds=0.000001)
        self.assertEqual(len(self.rows(self.backup)), 1)
        self.assertEqual(len(self.rows(self.live)), 1)
        self.assert_no_ledger()

    def test_online_backup_includes_committed_wal_rows(self):
        with closing(sqlite3.connect(self.live)) as writer:
            writer.execute('PRAGMA journal_mode=WAL')
            writer.execute('PRAGMA wal_autocheckpoint=0')
            writer.execute('INSERT INTO executions(timestamp,command) VALUES (?,?)',
                           ('2026-09-16 16:00:00', 'committed WAL record'))
            writer.commit()
            self.assertTrue(Path(str(self.live) + '-wal').exists())
            self.apply()
            self.assertEqual(len(self.rows(self.backup)), 2)
            self.assertEqual(len(self.rows(self.live)), 7)

    def test_destination_schema_mismatch_refused(self):
        with closing(sqlite3.connect(self.live)) as conn:
            conn.execute('CREATE TRIGGER unexpected AFTER INSERT ON executions BEGIN DELETE FROM executions; END')
            conn.commit()
        with self.assertRaisesRegex(recovery.RecoveryError, 'schema'):
            self.apply()
        self.assertEqual(len(self.rows(self.live)), 1)
        self.assert_no_ledger()

    def test_sqlite_lookalike_mutating_trigger_refused(self):
        before = self.rows(self.live)
        with closing(sqlite3.connect(self.live)) as conn:
            conn.execute('CREATE TRIGGER sqliteXunexpected AFTER INSERT ON executions '
                         'BEGIN UPDATE executions SET original_tokens=original_tokens+1 '
                         'WHERE id=1; END')
            conn.commit()
        # This legal name escaped the old LIKE 'sqlite_%' schema filter.
        with self.assertRaisesRegex(recovery.RecoveryError, 'schema'):
            self.apply()
        self.assertEqual(self.rows(self.live), before)
        self.assert_no_ledger()

    def test_large_synthetic_import_with_historical_counts(self):
        for source, count in zip(self.sources, (27649, 99)):
            with closing(sqlite3.connect(source.path)) as conn:
                for index in range(source.count, count):
                    conn.execute('INSERT INTO executions(timestamp,command,duration_ms) VALUES (?,?,5)',
                                 (source.first, f'synthetic command {index}'))
                conn.commit()
        self.sources = tuple(replace(s, count=n) for s, n in zip(self.sources, (27649, 99)))
        self.assertEqual(self.apply()['imported_rows'], 27748)
        self.assertEqual(len(self.rows(self.live)), 27749)

    def test_existing_backup_never_overwritten(self):
        self.backup.write_bytes(b'precious backup')
        with self.assertRaises(FileExistsError):
            self.apply()
        self.assertEqual(self.backup.read_bytes(), b'precious backup')
        self.assert_no_ledger()

    def test_missing_input_never_created(self):
        self.sources[0].path.unlink()
        with self.assertRaisesRegex(recovery.RecoveryError, 'Missing'):
            self.apply()
        self.assertFalse(self.sources[0].path.exists())
        self.assertFalse(self.backup.exists())

    def test_high_autoincrement_sequence_is_preserved(self):
        with closing(sqlite3.connect(self.live)) as conn:
            conn.execute("UPDATE sqlite_sequence SET seq=1000 WHERE name='executions'")
            conn.commit()
        self.apply()
        self.assertEqual([r[0] for r in self.rows(self.live)], [1, 1001, 1002, 1003, 1004, 1005])


if __name__ == '__main__':
    unittest.main()
