"""Pinned schema-column verification in the SAME locked export transaction."""
import json
import functools
import hashlib
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
KEYS = ('column_name', 'ordinal_position', 'data_type', 'udt_name', 'is_nullable',
        'character_maximum_length', 'numeric_precision', 'numeric_scale', 'datetime_precision')


def literal(value):
    return "'" + value.replace("'", "''") + "'"


@functools.cache
def columns():
    manifest_path = ROOT / 'docs/migration/data-source-manifest.json'
    plan = json.loads((ROOT / 'retail/backend/internal/rowimport/field_plans.json').read_text(encoding='utf-8'))
    digest = hashlib.sha256(manifest_path.read_bytes().replace(b'\r\n', b'\n')).hexdigest()
    if digest != plan['source_columns_sha256']:
        raise ValueError('Pinned source column metadata drift')
    manifest = json.loads(manifest_path.read_text(encoding='utf-8'))
    return {table['source_table']: table['columns'] for table in manifest['tables']}


def guard(table):
    # table comes from the reviewed generator; quote values regardless.
    expected = [{key: column[key] for key in KEYS} for column in columns()[table]]
    projection = ','.join(key + ('::text AS ' + key if key in ('column_name', 'data_type', 'udt_name', 'is_nullable') else '') for key in KEYS)
    encoded = json.dumps(expected, ensure_ascii=True, separators=(',', ':'))
    return ("DO $$ BEGIN IF (SELECT jsonb_agg(to_jsonb(c) ORDER BY ordinal_position) FROM (SELECT " + projection +
            " FROM information_schema.columns WHERE table_schema='public' AND table_name=" + literal(table) +
            ") c) IS DISTINCT FROM " + literal(encoded) + "::jsonb THEN RAISE EXCEPTION 'source column shape changed'; END IF; END $$;")


def fixture_column(column):
    """Artificial-only source type DDL derived from pinned metadata, not SQL
    migrations. It recreates types/nullability/order, not source constraints.
    """
    udt = column['udt_name']
    if udt == 'varchar':
        size = column['character_maximum_length']
        design = 'varchar' if size is None else 'varchar(' + str(int(size)) + ')'
    elif udt in ('timestamptz', 'timestamp'):
        design = udt + '(' + str(int(column['datetime_precision'])) + ')'
    elif udt == 'numeric':
        design = 'numeric'
        if column['numeric_precision'] is not None:
            design += '(' + str(int(column['numeric_precision'])) + ',' + str(int(column['numeric_scale'])) + ')'
    elif udt == '_text':
        design = 'text[]'
    elif udt in ('text', 'bool', 'int4', 'int8', 'float4', 'json', 'jsonb', 'order_status_enum',
                 'order_claim_type_enum', 'claim_reason_enum', 'return_status_enum'):
        design = udt
    else:
        raise ValueError('Unreviewed artificial source type')
    return design + (' NOT NULL' if column['is_nullable'] == 'NO' else '')
