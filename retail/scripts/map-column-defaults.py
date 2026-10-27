"""Review source defaults against every field disposition; emit design, not DDL."""
import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'docs/migration'
DOMAINS = ['catalog', 'inventory-device', 'commerce', 'checkout-rules', 'operations', 'history-integration']


def classify(column, imported):
    default = column['column_default']
    if not imported:
        return 'not-imported', None, 'No target column; old runtime/auth defaults must not create replacement identities or jobs.'
    if column['is_identity'] != 'NO' or column['is_generated'] != 'NEVER':
        raise ValueError('Unreviewed identity/generated column')
    if default is None:
        return 'no-source-default', None, 'Explicit writes required when nonnullable; nullable omission policy belongs to command schema. Import never substitutes a default for a missing field.'
    if (column['source_table'], column['source_column']) in {('order_number_clock', 'id'), ('sku_number_counter', 'id')}:
        if default != 'true':
            raise ValueError('Singleton counter default changed')
        return 'singleton-boolean-key', True, 'Retain single-row boolean key enforced true; lock existing counter row for allocation. Not a generated business ID and not multiple per-actor rows; preserve counter position including soft-deleted/manual historical numbers.'
    if column['source_column'] == 'id':
        return 'application-identifier', None, 'Preserve imported IDs exactly; generate fresh new-write identifiers. Never copy a source literal ID default into target DDL.'
    if default in {'now()', 'CURRENT_TIMESTAMP'}:
        return 'utc-time-command', 'UTC DATETIME(6)', 'Imported source timestamps remain exact; new command supplies one authoritative UTC clock instant. updated_at is explicit on mutation, never implicit ON UPDATE or reset on replay.'
    if default.startswith('nextval('):
        return 'persistent-sequence-command', None, 'Preserve historical values and allocate new ones using locked persistent counter. Inspect source triggers/current sequence position and numbering domain; no MySQL auto_increment substitute or MAX(id)+1.'
    if default in {'true', 'false'}:
        return 'boolean-literal', default == 'true', 'Candidate explicit new-write default only; source NULL distinct. Public command must not use enabled/default true to start imported integrations.'
    if re.fullmatch(r'-?\d+(?:\.\d+)?', default):
        return 'exact-number-literal', default, 'Keep decimal lexeme, field units and bounds. Explicit command value, never float parsing or implicit missing-field repair; raw/value paired defaults must agree.'
    match = re.fullmatch(r"'((?:[^']|'')*)'::(text|jsonb|text\[\]|order_status_enum|return_status_enum)", default)
    if match:
        value, cast = match.groups()
        value = value.replace("''", "'")
        if cast == 'jsonb':
            json.loads(value)  # validate without normalizing exact numeric text
            return 'exact-json-literal', value, 'Source JSON text retained as valid exact JSON; target LONGTEXT+JSON_VALID. New-write default explicit; MySQL JSON implicit conversion or double encoding prohibited.'
        if cast == 'text[]':
            if value != '{}':
                raise ValueError('Unreviewed PostgreSQL array default')
            return 'empty-text-array', [], 'Explicit empty string array in target exact JSON representation; never literal PostgreSQL braces or NULL.'
        return 'text-or-enum-literal', value, 'Candidate command default preserving exact case/empty text; validate domain state and allowed transitions, no arbitrary enum fallback or task replay.'
    raise ValueError('Unreviewed default expression: ' + column['source_table'] + '.' + column['source_column'])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    input_hashes = {}

    def read(file):
        raw = (OUT / file).read_bytes().replace(b'\r\n', b'\n')
        input_hashes[file] = hashlib.sha256(raw).hexdigest()
        return json.loads(raw)

    source = read('source-column-defaults.json')
    mapped = {}
    excluded = {}
    for domain in DOMAINS:
        for field in read(domain + '-field-mapping.json')['fields']:
            key = (field['source_table'], field['source_column'])
            if key in mapped:
                raise ValueError('Duplicate mapped field')
            mapped[key] = field
    for field in read('nonimport-field-disposition.json')['fields']:
        key = (field['source_table'], field['source_column'])
        excluded[key] = field
    seen, rows = set(), []
    for column in source['columns']:
        key = (column['source_table'], column['source_column'])
        if key in seen or (key in mapped) == (key in excluded):
            raise ValueError('Default field coverage must be exclusive and exact')
        seen.add(key)
        policy, value, rule = classify(column, key in mapped)
        field = mapped.get(key, {})
        rows.append({'source_table': key[0], 'source_column': key[1], 'source_default': column['column_default'],
                     'target_table': field.get('target_table'), 'target_column': field.get('target_column'),
                     'policy': policy, 'candidate_new_write_value': value, 'rule': rule,
                     'status': 'designed-not-implemented',
                     'import_policy': 'Every source field must be present; NULL only if mapping allows. Never invoke new-write defaults during import.'})
    if seen != set(mapped) | set(excluded):
        raise ValueError('Field/default sets differ')
    report = {'format_version': 1, 'scope': 'Per-column source default dispositions and explicit new-write candidates; not target DDL, trigger/sequence capture or command acceptance',
              'input_sha256': input_hashes, 'policy_counts': dict(sorted(Counter(r['policy'] for r in rows).items())), 'fields': rows}
    text = json.dumps(report, ensure_ascii=False, indent=2) + '\n'
    file = OUT / 'column-default-design.json'
    if args.check:
        if file.read_text(encoding='utf-8-sig') != text:
            raise ValueError('Column default design drift')
    else:
        file.write_text(text, encoding='utf-8')
    print(json.dumps({'columns': len(rows), 'policies': report['policy_counts'], 'status': 'design-only'}))


if __name__ == '__main__':
    main()
