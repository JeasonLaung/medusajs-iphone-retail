"""Read-only trigger/function/sequence metadata and private current positions."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'docs/migration'
SQL = """BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT json_build_object(
 'triggers', (SELECT coalesce(json_agg(x ORDER BY source_table,trigger_name),'[]'::json) FROM (
   SELECT c.relname AS source_table,t.tgname AS trigger_name,t.tgenabled AS enabled,
          pg_get_triggerdef(t.oid,true) AS definition,pn.nspname AS function_schema,p.proname AS function_name,
          pg_get_function_identity_arguments(p.oid) AS function_arguments,pg_get_functiondef(p.oid) AS function_definition
   FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace
   JOIN pg_proc p ON p.oid=t.tgfoid JOIN pg_namespace pn ON pn.oid=p.pronamespace
   WHERE n.nspname='public' AND NOT t.tgisinternal
 ) x),
 'sequences', (SELECT coalesce(json_agg(x ORDER BY sequence_name),'[]'::json) FROM (
   SELECT c.relname AS sequence_name,format_type(s.seqtypid,NULL) AS data_type,
          s.seqstart::text AS start_value,s.seqincrement::text AS increment_by,
          s.seqmin::text AS min_value,s.seqmax::text AS max_value,s.seqcache::text AS cache_size,
          s.seqcycle AS cycles,
          (SELECT coalesce(json_agg(json_build_object('table',r.relname,'column',a.attname) ORDER BY r.relname,a.attname),'[]'::json)
           FROM pg_depend d JOIN pg_class r ON r.oid=d.refobjid
           JOIN pg_attribute a ON a.attrelid=r.oid AND a.attnum=d.refobjsubid
           WHERE d.classid='pg_class'::regclass AND d.objid=c.oid AND d.deptype IN ('a','i')) AS owned_columns
   FROM pg_sequence s JOIN pg_class c ON c.oid=s.seqrelid JOIN pg_namespace n ON n.oid=c.relnamespace
   WHERE n.nspname='public'
 ) x)
);
COMMIT;"""


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-container', default='medusa-trial-postgres-1')
    parser.add_argument('--positions', required=True, help='Git-ignored detailed current positions')
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    positions = Path(args.positions).resolve()
    if subprocess.run(['git', 'check-ignore', '--quiet', str(positions)], cwd=ROOT).returncode:
        raise ValueError('Positions must be Git ignored')

    def query(sql):
        result = subprocess.run(['docker', 'exec', '-i', args.source_container, 'psql', '-X', '-q', '-t', '-A',
                                 '-v', 'ON_ERROR_STOP=1', '-U', 'medusa', '-d', 'medusa_trial'],
                                input=sql, capture_output=True, encoding='utf-8', check=True)
        return json.loads(result.stdout)

    metadata = query(SQL)
    source = json.loads((OUT / 'data-source-manifest.json').read_text(encoding='utf-8-sig'))
    names = {t['source_table'] for t in source['tables']}
    if any(t['source_table'] not in names for t in metadata['triggers']):
        raise ValueError('Trigger outside recorded table inventory')
    schema = {'format_version': 1, 'source_commit': source['source_commit'],
              'scope': 'Current noninternal public triggers and their function bodies plus sequence metadata; no business row or secret values; does not execute functions or allocate numbers', **metadata}
    schema_content = json.dumps(schema, ensure_ascii=False, indent=2) + '\n'
    schema_file = OUT / 'source-numbering-database.json'
    if args.check:
        if schema_file.read_text(encoding='utf-8-sig') != schema_content:
            raise ValueError('Trigger/function/sequence schema drift; review before refreshing')
    else:
        schema_file.write_text(schema_content, encoding='utf-8')

    # Read sequence relations themselves: pg_sequences.last_value cannot express
    # is_called=false. No nextval/setval/DDL or row locks are invoked.
    selects = []
    for sequence in metadata['sequences']:
        name = sequence['sequence_name']
        identifier = '"' + name.replace('"', '""') + '"'
        literal = "'" + name.replace("'", "''") + "'"
        selects.append(f'SELECT {literal} AS name,last_value::text AS last_value,is_called FROM public.{identifier}')
    position_query = """BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT json_build_object('observed_at',CURRENT_TIMESTAMP,'positions',
  (SELECT coalesce(json_agg(x ORDER BY name),'[]'::json) FROM (""" + ' UNION ALL '.join(selects) + """) x),
  'counters',(SELECT json_build_object(
    'order_number_clock',(SELECT coalesce(json_agg(json_build_object('id',id,'value',last_tick::text)),'[]'::json) FROM public.order_number_clock),
    'sku_number_counter',(SELECT coalesce(json_agg(json_build_object('id',id,'value',last_id::text)),'[]'::json) FROM public.sku_number_counter))));
COMMIT;"""
    observed = query(position_query)
    issues = []
    for table, rows in observed['counters'].items():
        if len(rows) != 1 or rows[0]['id'] is not True or int(rows[0]['value']) < 0:
            issues.append(table + ':invalid-singleton')
    sequence_names = {s['sequence_name'] for s in metadata['sequences']}
    if {p['name'] for p in observed['positions']} != sequence_names:
        issues.append('sequence-position-coverage')
    for position in observed['positions']:
        if not isinstance(position['is_called'], bool):
            issues.append('sequence-is-called-invalid')
    private = {'format_version': 1, 'schema_sha256': hashlib.sha256(schema_content.encode()).hexdigest(),
               'scope': 'Live observations, not transactional snapshot of nontransactional sequence state; recapture at final write freeze. No allocation performed.', **observed}
    positions.parent.mkdir(parents=True, exist_ok=True)
    positions.write_text(json.dumps(private, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'triggers': len(metadata['triggers']),
                      'trigger_functions': len({(t['function_schema'], t['function_name'], t['function_arguments']) for t in metadata['triggers']}),
                      'sequences': len(metadata['sequences']), 'observed_positions': len(observed['positions']),
                      'counter_tables': len(observed['counters']), 'issues': issues,
                      'status': 'read-only-observed-not-mysql-acceptance'}))
    return 1 if issues else 0


if __name__ == '__main__':
    raise SystemExit(main())
