"""Explicit field-by-field exclusions and rebuild decisions, no row or credential reads."""
import argparse
import csv
import hashlib
import io
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'docs/migration'
T = 'created_at updated_at deleted_at'
# Explicit columns ensure a source schema addition cannot be silently discarded.
TABLES = {
    'api_key': f'id token salt redacted title type last_used_at created_by revoked_by revoked_at {T}',
    'auth_identity': f'id app_metadata {T}',
    'auth_mfa_factor': f'id auth_identity_id provider status provider_metadata metadata {T}',
    'auth_mfa_recovery_code': f'id auth_identity_id code_hash {T}',
    'auth_password_reset_token': f'id auth_identity_id provider_identity_id entity_id token_hash expires_at {T}',
    'auth_verification': f'id auth_identity_id entity_id entity_type code_provider verified_at requested_at provider_metadata metadata {T}',
    'provider_identity': f'id entity_id provider auth_identity_id user_metadata provider_metadata {T}',
    'invite': f'id email accepted token expires_at metadata {T}',
    'invite_rbac_role': f'invite_id rbac_role_id id {T}',
    'publishable_api_key_sales_channel': f'publishable_key_id sales_channel_id id {T}',
    'user': f'id first_name last_name email avatar_url metadata {T}',
    'user_preference': f'id user_id key value {T}',
    'user_rbac_role': f'user_id rbac_role_id id {T}',
    'customer_address': f'id customer_id address_name is_default_shipping is_default_billing company first_name last_name address_1 address_2 city country_code province postal_code phone metadata {T}',
    'customer_account_holder': f'customer_id account_holder_id id {T}',
    'rbac_policy': f'id key resource operation name description metadata {T}',
    'rbac_role': f'id name description metadata {T}',
    'rbac_role_inheritance': f'id role_id inherited_role_id metadata {T}',
    'rbac_role_parent': f'id role_id parent_id metadata {T}',
    'rbac_role_policy': f'id role_id policy_id metadata {T}',
    'layout_configuration': f'id zone user_id is_system_default configuration {T}',
    'view_configuration': f'id entity name user_id is_system_default configuration {T}',
    'property_label': f'id entity property label description {T}',
    'link_module_migrations': 'id table_name link_descriptor created_at',
    'locking': 'id owner_id expiration',
    'mikro_orm_migrations': 'id name executed_at',
    'script_migrations': 'id script_name created_at finished_at',
    'workflow_execution': f'id workflow_id transaction_id execution context state retention_time run_id {T}',
    'search_index': f'id name active_version {T}',
    'search_index_sync': f'id job_id status filters last_key documents_synced started_at completed_at error search_index_version_id resumable {T}',
    'search_index_version': f'id version provider physical_name definition_hash status search_index_id {T}',
    'search_pg_product_v10': 'id document indexed search_vector search_text created_at updated_at',
    'search_pg_product_v11': 'id document indexed search_vector search_text created_at updated_at',
    'search_postgres_index': 'name table_name schema_hash plan document_count created_at updated_at',
}
DECISIONS = {
    'exclude-login': ('exclude', 'User decision: create new staff/customer credentials and grants. Do not import old credentials, identifiers, invites, preferences or key-channel grants; create new public store configuration from migrated channel facts, not old API key values. Historical actor text remains only in separately mapped business records.'),
    'exclude-account-profile': ('exclude', 'Do not restore old login address books or provider ownership bindings. Historical order/cart/fulfillment addresses and payment facts are separately retained; a new account cannot inherit this profile by email.'),
    'rebuild-config': ('reference-only-rebuild', 'Use old definitions only to verify action coverage against authorization contract. Rebuild explicit Gin permissions and scopes; no automatic role equivalence, inherited grants or account assignments.'),
    'rebuild-runtime': ('reconcile-then-rebuild', 'Do not restore Medusa execution instances, locks or migration ledgers. Inspect pending operations for external effects before cutover; persist independent MySQL job/idempotency facts and versioned SQL. Source timestamp semantics need not be cast into a fake target because no field is imported.'),
    'rebuild-derived': ('rebuild-from-business', 'Rebuild search from accepted product/variant/price/channel/stock data under search contract; no import of PostgreSQL tsvector, physical tables, Medusa index versions, cursors or running syncs. Historical sync errors are diagnostics, not product facts.'),
    'review-ui-config': ('reference-only-rebuild-ui', 'Fixed snapshot has zero rows in all three UI configuration tables. Rebuild supported Ant Design table/layout configuration from Admin contract; no personal user_id inheritance. Before final cutover, recheck counts and explicitly map any newly created custom settings instead of dropping them.'),
}


def digest(path):
    return hashlib.sha256(path.read_bytes().replace(b'\r\n', b'\n')).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    source = json.loads((OUT / 'data-source-manifest.json').read_text(encoding='utf-8-sig'))
    by_table = {t['source_table']: t for t in source['tables']}
    fields, tables = [], []
    for name, declared in TABLES.items():
        table = by_table[name]
        names = declared.split()
        if len(names) != len(set(names)) or set(names) != {c['column_name'] for c in table['columns']}:
            raise ValueError(f'Nonimport decision needs schema review: {name}')
        action, reason = DECISIONS[table['disposition']]
        if table['disposition'] == 'review-ui-config' and table['snapshot_rows']:
            raise ValueError('UI configuration is no longer empty; map actual customization before exclusion')
        tables.append({'source_table': name, 'source_columns': len(names), 'source_rows': table['snapshot_rows'],
                       'action': action, 'reason': reason, 'tasks': table['tasks'], 'status': 'designed-not-implemented'})
        for col in table['columns']:
            fields.append({'source_table': name, 'source_column': col['column_name'],
                           'source_expression': name + '.' + col['column_name'], 'source_type': col['udt_name'],
                           'nullable': col['is_nullable'] == 'YES', 'action': action,
                           'target_table': None, 'target_column': None, 'target_type_design': None,
                           'reason': reason, 'source_disposition': table['disposition'],
                           'assertions': 'No importer reads/writes this source field; no runtime Medusa compatibility table. Recheck source schema/counts and reconcile side effects before final cutover.',
                           'status': 'designed-not-implemented'})
    result = {'format_version': 1, 'source_manifest_sha256': digest(OUT / 'data-source-manifest.json'),
              'source_constraints_sha256': digest(OUT / 'source-schema-constraints.json'),
              'scope': 'Explicit nonimport decisions only; not implementation, account exclusion proof, pending side-effect reconciliation or permission/search acceptance',
              'tables': tables, 'fields': fields}
    output = io.StringIO(newline='')
    writer = csv.DictWriter(output, fieldnames=list(fields[0]), lineterminator='\n')
    writer.writeheader()
    writer.writerows(fields)
    files = {'nonimport-field-disposition.json': json.dumps(result, ensure_ascii=False, indent=2) + '\n',
             'nonimport-field-disposition.csv': output.getvalue()}
    for name, content in files.items():
        path = OUT / name
        if args.check:
            if path.read_text(encoding='utf-8-sig') != content:
                raise ValueError('Nonimport design drift: ' + name)
        else:
            path.write_text(content, encoding='utf-8', newline='')
    print(json.dumps({'nonimport_tables': len(tables), 'nonimport_fields': len(fields), 'mode': 'check' if args.check else 'generate'}))


if __name__ == '__main__':
    main()
