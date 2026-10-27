"""Verify captured positions and execute only read-only artificial SQL expressions."""
import argparse
from collections import Counter
from datetime import datetime, timedelta, timezone
import hashlib
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'docs/migration'
BUNDLE_HASH = 'aa18ee30d5a7dcb11efe20a5c8785abfdbe561c362db4503160b262df6a2f132'
BUSINESS = {
    'order_display_id_seq': ('order', 'display_id'),
    'order_change_action_ordering_seq': ('order_change_action', 'ordering'),
    'order_claim_display_id_seq': ('order_claim', 'display_id'),
    'order_exchange_display_id_seq': ('order_exchange', 'display_id'),
    'return_display_id_seq': ('return', 'display_id'),
}
EXCLUDED = {'link_module_migrations_id_seq', 'mikro_orm_migrations_id_seq', 'script_migrations_id_seq'}


def next_sequence(position, sequence):
    last = int(position['last_value'])
    return last + int(sequence['increment_by']) if position['is_called'] else last


def format_tick(tick):
    seconds, suffix = divmod(tick, 100000)
    instant = datetime(1970, 1, 1, tzinfo=timezone.utc) + timedelta(seconds=seconds)
    return instant.astimezone(timezone(timedelta(hours=8))).strftime('%Y%m%d%H%M%S') + f'{suffix:05d}'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bundle', required=True)
    parser.add_argument('--positions', required=True)
    parser.add_argument('--source-container', default='medusa-trial-postgres-1')
    args = parser.parse_args()
    for file in [args.bundle, args.positions]:
        if subprocess.run(['git', 'check-ignore', '--quiet', str(Path(file).resolve())], cwd=ROOT).returncode:
            raise ValueError('Private inputs must be Git ignored')
    raw = Path(args.bundle).read_bytes()
    if hashlib.sha256(raw).hexdigest() != BUNDLE_HASH:
        raise ValueError('Unexpected fixed snapshot')
    tables = json.loads(raw)['tables']
    metadata_raw = (OUT / 'source-numbering-database.json').read_bytes().replace(b'\r\n', b'\n')
    schema_hash = hashlib.sha256(metadata_raw).hexdigest()
    metadata = json.loads(metadata_raw)
    private = json.loads(Path(args.positions).read_text(encoding='utf-8'))
    if private['schema_sha256'] != schema_hash:
        raise ValueError('Position observation and schema evidence differ')
    positions = {p['name']: p for p in private['positions']}
    sequences = {s['sequence_name']: s for s in metadata['sequences']}
    if set(sequences) != set(BUSINESS) | EXCLUDED or set(positions) != set(sequences):
        raise ValueError('Unknown/missing sequence requires explicit disposition')
    checks, issues, holds = Counter(), Counter(), Counter()
    for name, sequence in sequences.items():
        checks['sequence_definition'] += 1
        if sequence['cycles'] or sequence['increment_by'] != '1' or sequence['cache_size'] != '1':
            issues['sequence:unreviewed-allocation-policy'] += 1
        if name in BUSINESS:
            table, column = BUSINESS[name]
            if sequence['owned_columns'] != [{'table': table, 'column': column}]:
                issues['sequence:owner-mismatch'] += 1
            maximum = max((int(r[column]) for r in tables[table] if r[column] is not None), default=0)
            checks['sequence_next_after_fixed_snapshot_maximum'] += 1
            if next_sequence(positions[name], sequence) <= maximum:
                issues['sequence:next-not-above-fixed-historical-max'] += 1
            if next_sequence(positions[name], sequence) > int(sequence['max_value']):
                issues['sequence:exhausted'] += 1
        else:
            holds['legacy_migration_sequence_not_imported'] += 1
    for table, field in [('order_number_clock', 'last_tick'), ('sku_number_counter', 'last_id')]:
        observed = private['counters'][table]
        checks['counter_observation_not_below_fixed_snapshot'] += 1
        if len(observed) != 1 or observed[0]['id'] is not True or int(observed[0]['value']) < tables[table][0][field]:
            issues[table + ':counter-regressed-or-invalid'] += 1
    trigger_pairs = {(t['source_table'], t['trigger_name']) for t in metadata['triggers']}
    if trigger_pairs != {('order', 'order_timestamp_number'), ('product_variant', 'product_variant_erp_sku')}:
        raise ValueError('Unknown/missing project trigger')
    if any(t['enabled'] != 'O' for t in metadata['triggers']):
        issues['trigger:not-enabled-for-normal-writes'] += 1
    order_fn = next(t['function_definition'] for t in metadata['triggers'] if t['source_table'] == 'order')
    # Extract the actual formatting expression. Replacing tick by an artificial
    # CTE column executes its SQL without calling the trigger or touching clocks.
    match = re.search(r'NEW\.custom_display_id\s*:=\s*(.*?)\s*;\s*RETURN NEW;', order_fn, re.S)
    if not match:
        raise ValueError('Order SQL format changed; review expression extraction')
    expression = match.group(1)
    if not re.fullmatch(r"[A-Za-z0-9_\s()/%:.,'|]+", expression):
        raise ValueError('Unexpected SQL expression syntax')
    inputs = [(0, 0), (0, 1), (99998, 99999), (99999, 100000),
              (100000, 99999), (170000000000000, 170000000000000),
              (170000000000010, 170000000000000), (170000009999999, 170000010000000),
              (176719679999999, 176719680000000), (179308800012345, 179308800012346)]
    values = ','.join(f'({index},{last}::bigint,{now}::bigint)' for index, (last, now) in enumerate(inputs))
    sql = f"""BEGIN READ ONLY;
WITH input(case_id,last_tick,now_tick) AS (VALUES {values}),
 evaluated AS (SELECT case_id,GREATEST(last_tick+1,now_tick) AS tick FROM input)
SELECT json_agg(json_build_object('case',case_id,'tick',tick::text,'number',({expression})) ORDER BY case_id) FROM evaluated;
COMMIT;"""
    result = subprocess.run(['docker', 'exec', '-i', args.source_container, 'psql', '-X', '-q', '-t', '-A',
                             '-v', 'ON_ERROR_STOP=1', '-U', 'medusa', '-d', 'medusa_trial'], input=sql,
                            capture_output=True, encoding='utf-8', check=True)
    output = json.loads(result.stdout)
    if len(output) != len(inputs):
        raise ValueError('SQL artificial case coverage changed')
    for row in output:
        last, now = inputs[row['case']]
        expected = max(last + 1, now)
        if row['tick'] != str(expected) or row['number'] != format_tick(expected):
            raise ValueError('Actual SQL order format differs from independent integer/UTC expectation')
        checks['artificial_actual_sql_order_expression'] += 1
    # Independent explicit sequence seed boundary expectations; no nextval call.
    for last, called, increment, expected in [(1, False, 1, 1), (1, True, 1, 2),
                                            (100, False, 1, 100), (100, True, 1, 101)]:
        if next_sequence({'last_value': str(last), 'is_called': called}, {'increment_by': str(increment)}) != expected:
            raise ValueError('Sequence is_called expectation failed')
        checks['artificial_sequence_is_called'] += 1
    report = {'format_version': 1, 'source_snapshot_sha256': BUNDLE_HASH, 'source_schema_sha256': schema_hash,
              'scope': 'Live private sequence/counter observations compared to older fixed snapshot; actual order format SELECT expressions with artificial tick inputs only. No trigger execution, nextval/setval, counter mutation or MySQL/concurrency acceptance.',
              'checks': dict(sorted(checks.items())), 'blocking_findings': dict(sorted(issues.items())),
              'migration_holds': dict(sorted(holds.items())),
              'remaining': ['Reobserve under final source write freeze; live sequence reads are nontransactional',
                            'Execute full SKU and order trigger behavior in isolated database, including overflow/rollback/concurrency',
                            'Implement MySQL persistent allocator and import seed policy without replaying old migration sequences'],
              'status': 'failed-needs-review' if issues else 'read-only-numbering-evidence-not-accepted'}
    (OUT / 'numbering-database-report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'checks': sum(checks.values()), 'blocking_findings': report['blocking_findings'],
                      'migration_holds': report['migration_holds'], 'status': report['status']}))
    return 1 if issues else 0


if __name__ == '__main__':
    raise SystemExit(main())
