"""Explicit historical subjects, integration state, notifications and number clocks."""
import argparse
from importlib.util import module_from_spec, spec_from_file_location
from pathlib import Path

spec = spec_from_file_location('commerce', Path(__file__).with_name('map-commerce-fields.py'))
commerce = module_from_spec(spec)
spec.loader.exec_module(commerce)
T = commerce.T
TABLES = {
    'customer': ('historical_customer_subjects', f'id company_name first_name last_name email phone has_account metadata created_by {T}'),
    'customer_group': ('historical_customer_groups', f'id name metadata created_by {T}'),
    'customer_group_customer': ('historical_customer_memberships', f'id customer_id customer_group_id metadata created_by {T}'),
    'mollie_config': ('mollie_configs', f'id environment api_key redirect_url enabled {T}'),
    'paypal_connection': ('paypal_connections', f'id environment status shared_id auth_code seller_client_id seller_client_secret seller_merchant_id seller_email app_access_token app_access_token_expires_at metadata {T}'),
    'paypal_settings': ('paypal_settings', f'id data {T}'),
    'paypal_metric': ('paypal_legacy_metrics', f'id name data {T}'),
    'paypal_webhook_event': ('paypal_webhook_events', f'id event_id event_type event_version transmission_id transmission_time status attempt_count next_retry_at processed_at last_error resource_id payload {T}'),
    'notification': ('historical_notifications', f'id to channel template data trigger_type resource_id resource_type receiver_id original_notification_id idempotency_key external_id provider_id status from provider_data {T}'),
    'notification_provider': ('notification_providers', f'id handle name is_enabled channels {T}'),
    'order_number_clock': ('order_number_clock', 'id last_tick'),
    'sku_number_counter': ('sku_number_counter', 'id last_id'),
}
REFS = {
    ('customer_group_customer', 'customer_id'): 'historical_customer_subjects.id',
    ('customer_group_customer', 'customer_group_id'): 'historical_customer_groups.id',
    ('notification', 'original_notification_id'): 'historical_notifications.id',
    ('notification', 'provider_id'): 'notification_providers.id',
}
RENAMES = {('customer', 'has_account'): 'legacy_had_account'}
SECRETS = {('mollie_config', 'api_key'), ('paypal_connection', 'auth_code'),
           ('paypal_connection', 'seller_client_secret'), ('paypal_connection', 'app_access_token')}
PROTECTED_JSON = {('customer', 'metadata'), ('customer_group', 'metadata'), ('customer_group_customer', 'metadata'),
                  ('paypal_connection', 'metadata'), ('paypal_settings', 'data'), ('paypal_metric', 'data'),
                  ('paypal_webhook_event', 'payload'), ('notification', 'data'), ('notification', 'provider_data')}


def convert(table, column):
    name, udt = column['column_name'], column['udt_name']
    pair = (table, name)
    if pair in SECRETS:
        detail = 'Decrypt source Mollie AES-GCM using domain key before new envelope' if table == 'mollie_config' else 'Treat source provider string as secret; never assume Mollie/mail encryption format; no credential exchange or token refresh on import'
        return 'LONGBLOB (versioned authenticated envelope)', 'protected_string', detail + '; preserve NULL/empty; no log or DTO; business merchant credentials are distinct from excluded customer/staff authentication'
    if pair in PROTECTED_JSON:
        return 'LONGBLOB (versioned authenticated envelope)', 'protected_exact_json', 'Preserve exact JSON/decimal lexemes privately; extract only reviewed business fields; exclude old login grants/reset/verification capabilities before new persistence; no live provider calls/replay; raw snapshot remains private source evidence, not new account data'
    if udt == '_text':
        return 'LONGTEXT utf8mb4_0900_bin + CHECK JSON_VALID', 'text_array_json', 'Preserve ordered text array including empty and NULL elements if source allows; no comma joining; source PostgreSQL array replaced with JSON array'
    target, transform, assertion = commerce.convert(table, column)
    if table.startswith('historical_') or table.startswith('customer'):
        assertion += '; historical business subject only, no account creation/email matching/ownership or price-group entitlement for new login'
    if table == 'customer' and name == 'has_account':
        assertion += '; legacy_had_account is audit-only; never consulted by authentication or registration uniqueness'
    if table in {'order_number_clock', 'sku_number_counter'}:
        assertion += '; singleton id must be true; transaction row lock; preserve current counter, no rescaling or reset; compare all historical allocated numbers before enabling writes'
    if table.startswith('paypal_'):
        assertion += '; isolated import hold: no retry, token refresh, telemetry, webhook processing or outbound call; provider operational metric is not sales ledger'
    if table == 'notification':
        assertion += '; historical recipient reference, no new login FK; authentication-related payloads excluded; no historical retry or delivery'
    if table == 'mollie_config':
        assertion += '; preserve configuration but separate migration hold blocks external calls and redirects until explicit cutover'
    return target, transform, assertion


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    commerce.catalog.generate(TABLES, convert, REFS, RENAMES, 'history-integration',
                              'Historical subjects and protected integration/numbering design only; no legacy login restoration, provider activity or accepted import', args.check)
