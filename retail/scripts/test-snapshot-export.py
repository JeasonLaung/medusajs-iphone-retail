"""Native/container exports share one read-only protocol and private artifacts."""
import hashlib
import importlib.util
import os
from pathlib import Path
import stat
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('snapshot_export', Path(__file__).with_name('export-cell-snapshot.py'))
exporter = importlib.util.module_from_spec(spec)
spec.loader.exec_module(exporter)


class SnapshotExportTests(unittest.TestCase):
    def test_native_requires_explicit_loopback_and_port(self):
        for host, port in [('example.com', 5432), ('127.0.0.1', None), ('localhost', 0), ('::1', 65536)]:
            with self.assertRaises(ValueError):
                exporter.source_command(None, 'medusa', 'source', host, port)
        command = exporter.source_command(None, 'medusa', 'source', '127.0.0.1', 5433)
        self.assertEqual(command[:5], ['psql', '-h', '127.0.0.1', '-p', '5433'])
        self.assertIn('-w', command)  # Never hang waiting for a password prompt.
        self.assertNotIn('docker', command)

    def test_container_connection_preserved_and_modes_do_not_mix(self):
        command = exporter.source_command('owned-source', 'medusa', 'source')
        self.assertEqual(command[:5], ['docker', 'exec', '-i', 'owned-source', 'psql'])
        with self.assertRaises(ValueError):
            exporter.source_command('owned-source', 'medusa', 'source', 'localhost', 5432)

    def test_source_protocol_is_read_only_and_guards_precede_rows(self):
        sql = exporter.program_sql()
        self.assertTrue(sql.startswith('BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;'))
        self.assertIn('IN ACCESS SHARE MODE;', sql)
        self.assertLess(sql.rindex('DO $'), sql.index('"kind":"snapshot"'))
        self.assertLess(sql.index('COMMIT;'), sql.index('"kind":"snapshot_complete"'))

    def test_restored_column_slots_keep_order_and_type_guards(self):
        columns = exporter.shape.columns()['product_option']
        self.assertTrue(any(c['ordinal_position'] != i for i, c in enumerate(columns, 1)))
        sql = exporter.shape.guard('product_option')
        self.assertIn('row_number() OVER (ORDER BY ordinal_position)', sql)
        # A restored contiguous sequence and the original gapped sequence
        # produce the same visible order. All other pinned properties remain.
        import json
        import re
        expected = json.loads(re.search(r"IS DISTINCT FROM '(.*)'::jsonb THEN", sql)[1].replace("''", "'"))
        self.assertEqual([c['ordinal_position'] for c in expected], list(range(1, len(columns) + 1)))
        self.assertEqual([c['column_name'] for c in expected], [c['column_name'] for c in columns])
        for original, visible in zip(columns, expected):
            self.assertEqual({k: original[k] for k in exporter.shape.KEYS if k != 'ordinal_position'},
                             {k: visible[k] for k in exporter.shape.KEYS if k != 'ordinal_position'})

    def export_case(self, failure=None):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'snapshot.ndjson'
            payload = b'artificial complete snapshot\n'
            calls = []

            def run(command, **kwargs):
                if command[0] == 'git':
                    return subprocess.CompletedProcess(command, 0)
                calls.append(command)
                self.assertIn('READ ONLY', kwargs['stdin'].read().decode())
                self.assertEqual(stat.S_IMODE(output.stat().st_mode), 0o600)
                kwargs['stdout'].write(payload)
                if failure == 'timeout':
                    raise subprocess.TimeoutExpired(command, 600)
                return subprocess.CompletedProcess(command, 1 if failure else 0)

            with patch.object(exporter.subprocess, 'run', side_effect=run):
                if failure:
                    with self.assertRaises(RuntimeError):
                        exporter.export(None, 'medusa', 'source', output, host='127.0.0.1', port=5433)
                    self.assertFalse(output.with_suffix('.ndjson.sha256').exists())
                else:
                    digest = exporter.export(None, 'medusa', 'source', output, host='127.0.0.1', port=5433)
                    self.assertEqual(digest, hashlib.sha256(payload).hexdigest())
                    self.assertEqual(output.with_suffix('.ndjson.sha256').read_text(), digest + '\n')
                    self.assertEqual(stat.S_IMODE(output.with_suffix('.ndjson.sha256').stat().st_mode), 0o600)
                self.assertFalse(output.with_suffix('.ndjson.query.tmp').exists())
                with self.assertRaises(ValueError):
                    exporter.export(None, 'medusa', 'source', output, host='127.0.0.1', port=5433)
                self.assertEqual(len(calls), 1)

    def test_native_snapshot_private_complete_hash_and_no_overwrite(self):
        self.export_case()

    def test_failed_source_never_writes_completion_checksum(self):
        self.export_case('sql')

    def test_timeout_retains_partial_without_checksum(self):
        self.export_case('timeout')


if __name__ == '__main__':
    unittest.main()
