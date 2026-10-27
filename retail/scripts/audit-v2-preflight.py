"""Verify read-only v2 CLI diagnostics and rejection of the former v1 writer."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import uuid

ROOT = Path(__file__).resolve().parents[2]
PINNED_SHA = 'b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985'


def ignored(path):
    return subprocess.run(['git', 'check-ignore', '--quiet', str(path)], cwd=ROOT, capture_output=True).returncode == 0


def digest(path):
    return hashlib.sha256(path.read_bytes().replace(b'\r\n', b'\n')).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--snapshot', required=True, type=Path)
    snapshot = parser.parse_args().snapshot.resolve()
    if not snapshot.is_file() or not ignored(snapshot) or hashlib.sha256(snapshot.read_bytes()).hexdigest() != PINNED_SHA:
        raise RuntimeError('Pinned ignored v2 snapshot required')
    suffix = '.exe' if os.name == 'nt' else ''
    owner = uuid.uuid4().hex
    paths = [ROOT / 'logs/gin-migration' / (name + '-' + owner + suffix) for name in ['preflight', 'disabled-import']]
    if any(path.exists() or not ignored(path) for path in paths):
        raise RuntimeError('New ignored executable paths required')
    env = os.environ.copy()
    env['MYSQL_DSN'] = 'invalid-private-dsn-marker'
    owned = []
    try:
        for path, command in zip(paths, ['check-snapshot', 'import-medusa']):
            owned.append(path)
            compiled = subprocess.run(['go', 'build', '-o', str(path), './cmd/' + command], cwd=ROOT / 'retail/backend', capture_output=True, timeout=120)
            if compiled.returncode:
                raise RuntimeError('Offline CLI compilation failed')
        tested = subprocess.run([str(paths[0]), '--snapshot', str(snapshot), '--sha256', PINNED_SHA],
                                cwd=ROOT / 'retail/backend', env=env, capture_output=True, timeout=120)
        if tested.returncode != 2 or tested.stderr:
            raise RuntimeError('Known unresolved references did not fail diagnostics as expected')
        report = json.loads(tested.stdout)
        expected = {'tables': 159, 'rows': 27140, 'fields': 220235, 'sql_null_fields': 73405,
                    'ordinary_convertible_rows': 27090, 'protected_rows': 48, 'history_policy_rows': 45, 'invalid_rows': 0}
        if report['counts'] != expected or report['snapshot_sha256'] != PINNED_SHA:
            raise RuntimeError('Preflight aggregate coverage differs')
        if report['scalar_and_reference_checks_passed'] or report['writes_enabled'] or report['complete_import_accepted']:
            raise RuntimeError('Preflight incorrectly accepted writes')
        if sum(report['relations']['missing'].values()) != 375 or report['relations']['physical_missing']:
            raise RuntimeError('Reference failure evidence differs')
        for option in [[], ['--sha256', '0' * 64], ['--max-rows', '1'], ['--max-key-bytes', '1']]:
            # Empty option tests the retired command even with a nonexistent
            # private path and an invalid DB configuration. Other cases must
            # not emit partial diagnostic JSON.
            if not option:
                failed = subprocess.run([str(paths[1]), '--bundle', 'private-does-not-exist'], env=env, capture_output=True, timeout=10)
                if failed.returncode != 1 or failed.stdout or b'Legacy v1 import disabled.' not in failed.stderr:
                    raise RuntimeError('Retired writer failed to reject before I/O')
            else:
                failed = subprocess.run([str(paths[0]), '--snapshot', str(snapshot), '--sha256', PINNED_SHA, *option], env=env, capture_output=True, timeout=120)
                if failed.returncode != 1 or failed.stdout or b'private-' in failed.stderr:
                    raise RuntimeError('Invalid or over-limit input leaked partial results')
        files = ['retail/scripts/audit-v2-preflight.py', 'retail/backend/cmd/check-snapshot/main.go',
                 'retail/backend/cmd/check-snapshot/main_test.go', 'retail/backend/cmd/import-medusa/main.go',
                 'retail/backend/internal/importer/catalog.go', 'retail/backend/internal/snapshotcheck/check.go',
                 'retail/backend/internal/snapshotcheck/check_test.go']
        files += ['retail/backend/internal/rowimport/' + name for name in
                  ['inspect.go', 'convert.go', 'stream.go', 'record.go', 'relationships.go', 'field_plans.json', 'number_pair.go', 'source_check.go']]
        evidence = {'format_version': 1, 'scope': 'Offline diagnostic CLI and disabled v1 write entry only; not complete migration acceptance',
                    'snapshot_sha256': PINNED_SHA, 'counts': expected, 'unresolved_references': 375,
                    'diagnostic_exit_code': 2, 'invalid_input_exit_code': 1, 'retired_writer_exit_code': 1,
                    'writes_enabled': False, 'complete_import_accepted': False, 'private_values_in_report': False,
                    'source_database_connection': False, 'target_database_connection': False,
                    'implementation_sha256': {file: digest(ROOT / file) for file in files}}
    finally:
        for path in owned:
            if path.exists():
                path.unlink()
    evidence['temporary_executables_removed'] = True
    (ROOT / 'docs/migration/v2-preflight-report.json').write_text(json.dumps(evidence, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'counts': expected, 'diagnostic_exit_code': 2, 'unresolved_references': 375, 'cleanup': 'complete'}))


if __name__ == '__main__':
    main()
