"""Pure SQL builder for v2 per-cell records. No database or row access."""
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]


def quote(name):
    if not re.fullmatch(r'[a-zA-Z_][a-zA-Z_0-9]*', name):
        raise ValueError('Unreviewed source identifier')
    return '"' + name + '"'


def plans():
    path = ROOT / 'retail/backend/internal/rowimport/field_plans.json'
    program = json.loads(path.read_text(encoding='utf-8'))
    assert program['version'] == 1 and len(program['tables']) == 159
    return program['tables']


def query_for(plan):
    """Use to_json(column)::text INSIDE each cell; never JS numeric decoding.

    Scalar IS NULL is evaluated before JSON construction. A JSON document null
    therefore has sql_null=false,value_json='null'. Caller must supply the
    exact reviewed plan and one read-only consistent transaction for all tables.
    """
    table = plan['source']
    quote(table)
    cells = []
    for field in plan['fields']:
        name = field['source']
        column = 't.' + quote(name)
        cells.append("'" + name + "',CASE WHEN " + column + " IS NULL THEN json_build_object('sql_null',true) ELSE json_build_object('sql_null',false,'value_json',to_json(" + column + ")::text) END")
    # Avoid PostgreSQL's function argument limit on wide tables: aggregate
    # key/value cell records rather than passing 2*N function arguments.
    entries = []
    for cell in cells:
        key, expression = cell.split(',', 1)
        entries.append('(' + key + ',' + expression + ')')
    return ("SELECT json_build_object('format_version',2,'table','" + table + "','cells',"
            "(SELECT json_object_agg(k,v) FROM (VALUES " + ','.join(entries) + ") fields(k,v)))::text FROM public."
            + quote(table) + ' t;')


if __name__ == '__main__':
    statements = [query_for(plan) for plan in plans()]
    # Metadata only; do not print potentially executable source export SQL or rows.
    print(json.dumps({'tables': len(statements), 'mode': 'pure-reviewed-v2-cell-query-builder-no-database-access'}))
