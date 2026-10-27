"""Reviewed custom business fields; metadata only, never reads secrets or sends messages."""
import argparse
from importlib.util import module_from_spec, spec_from_file_location
from pathlib import Path

spec = spec_from_file_location('commerce', Path(__file__).with_name('map-commerce-fields.py'))
commerce = module_from_spec(spec)
spec.loader.exec_module(commerce)
T = commerce.T
TABLES = {
    'buyback_request': ('buyback_requests', f'id customer_id submission_key contact_name email phone return_address items status reference_amount final_amount currency_code shipping_instructions tracking_number return_tracking payment_reference review_note history closed_at raw_reference_amount raw_final_amount {T}'),
    'buyback_photo': ('buyback_photos', f'id request_id item_id photo_type uploaded_at storage_key mime_type review_status review_note {T}'),
    'buyback_price': ('buyback_prices', f'id product_id capacity amount currency_code raw_amount pricing_rules {T}'),
    'buyback_device_link': ('buyback_device_links', f'id request_id item_id device_id {T}'),
    'mail_account': ('mail_accounts', f'id name email reply_to enabled api_key webhook_secret {T}'),
    'mail_campaign': ('mail_campaigns', f'id name account_id subject text_body html_body status created_by {T}'),
    'mail_contact': ('mail_contacts', f'id email name subscribed consent_source consent_at suppressed unsubscribe_token {T}'),
    'mail_delivery': ('mail_deliveries', f'id campaign_id contact_id email status provider_id attempts error sent_at opened_at clicked_at first_attempt_at {T}'),
    'mail_event': ('mail_events', f'id event_id account_id type provider_id occurred_at {T}'),
    'mail_message': ('mail_messages', f'id account_id folder previous_folder direction from_address to cc bcc subject text_body html_body attachments provider_id message_id in_reply_to reply_to read starred status error attempts sent_at received_at created_by first_attempt_at {T}'),
    'mail_template': ('mail_templates', f'id name subject text_body html_body {T}'),
    'seo_crawler_visit': ('seo_crawler_visits', f'id source_id crawler_name request_path status_code client_ip_hash occurred_at event_hash {T}'),
    'seo_evidence': ('seo_evidence', f'id run_id source_url source_title evidence_type {T}'),
    'seo_monitor': ('seo_monitors', f'id monitor_name brand_name provider query_text interval_minutes enabled last_run_at {T}'),
    'seo_page': ('seo_pages', f'id path title description canonical og_image hreflang structured_data robots {T}'),
    'seo_provider': ('seo_providers', f'id provider api_key base_url model wire_api {T}'),
    'seo_run': ('seo_runs', f'id monitor_id provider model prompt_text answer_text status brand_mentioned citation_count started_at finished_at error_message {T}'),
    'seo_setting': ('seo_settings', f'id key value is_secret group {T}'),
    'seo_source': ('seo_sources', f'id source_name source_type file_path interval_minutes enabled collect_ai_crawlers collect_traffic read_offset read_lines file_fingerprint matched_lines traffic_matched_lines last_scanned_at last_error raw_read_offset {T}'),
    'seo_submission': ('seo_submissions', f'id success submitted_count http_status urls response_headers response_body error submitted_at {T}'),
    'seo_traffic_visit': ('seo_traffic_visits', f'id log_source_id platform source medium campaign click_id_type matched_by landing_path referrer_host status_code client_ip location occurred_at event_hash {T}'),
    'product_review': ('product_reviews', f'id product_id customer_id author rating title content status order_id verified_purchase {T}'),
    'customer_notice': ('customer_notices', f'id customer_id event_key kind resource_id reference action read_at {T}'),
    'webhook_endpoint': ('webhook_endpoints', f'id name provider url secret token keyword enabled events language last_status last_sent_at {T}'),
}
REFS = {
    ('buyback_request', 'customer_id'): 'historical_customer_subjects.id (history only; no new-login ownership)',
    ('buyback_photo', 'request_id'): 'buyback_requests.id',
    ('buyback_device_link', 'request_id'): 'buyback_requests.id',
    ('buyback_device_link', 'device_id'): 'devices.id',
    ('buyback_price', 'product_id'): 'catalog_products.id',
    ('mail_campaign', 'account_id'): 'mail_accounts.id',
    ('mail_message', 'account_id'): 'mail_accounts.id',
    ('mail_event', 'account_id'): 'mail_accounts.id',
    ('mail_delivery', 'campaign_id'): 'mail_campaigns.id',
    ('mail_delivery', 'contact_id'): 'mail_contacts.id',
    ('seo_crawler_visit', 'source_id'): 'seo_sources.id',
    ('seo_traffic_visit', 'log_source_id'): 'seo_sources.id',
    ('seo_evidence', 'run_id'): 'seo_runs.id',
    ('seo_run', 'monitor_id'): 'seo_monitors.id',
    ('product_review', 'product_id'): 'catalog_products.id',
    ('product_review', 'customer_id'): 'historical_customer_subjects.id (history only; no new-login ownership)',
    ('product_review', 'order_id'): 'orders.id',
    ('customer_notice', 'customer_id'): 'historical_customer_subjects.id (history only; no new-login ownership)',
}
ENUMS = {
    ('buyback_request', 'status'): 'awaiting_photos photo_review ready_to_ship in_transit received final_offer accepted paid return_requested returning returned canceled',
    ('buyback_photo', 'photo_type'): 'front back edges damage packaging sealed',
    ('buyback_photo', 'review_status'): 'pending approved retake expired',
    ('mail_message', 'folder'): 'inbox draft outbox sent archive trash',
    ('mail_message', 'direction'): 'inbound outbound',
    ('mail_campaign', 'status'): 'draft queued sending paused completed',
    ('seo_provider', 'provider'): 'chatgpt deepseek',
    ('seo_monitor', 'provider'): 'chatgpt deepseek',
    ('seo_provider', 'wire_api'): 'responses chat chat_input',
    ('seo_run', 'status'): 'running completed failed',
    ('seo_evidence', 'evidence_type'): 'provider_citation answer_link',
    ('product_review', 'status'): 'pending approved rejected',
    ('webhook_endpoint', 'provider'): 'feishu wecom dingtalk slack mattermost generic',
    ('webhook_endpoint', 'language'): 'zh en',
}
SECRETS = {('mail_account', 'api_key'), ('mail_account', 'webhook_secret'),
           ('mail_contact', 'unsubscribe_token'), ('seo_provider', 'api_key'),
           ('seo_setting', 'value'), ('webhook_endpoint', 'secret'),
           ('webhook_endpoint', 'token'), ('webhook_endpoint', 'url')}


def convert(table, column):
    name = column['column_name']
    pair = (table, name)
    if pair in ENUMS:
        return 'VARCHAR(32) utf8mb4_0900_bin + CHECK', 'enum_text', 'Allowed: ' + ENUMS[pair] + '; preserve state; command transition rules separately enforced'
    if pair in SECRETS:
        return 'LONGBLOB (versioned authenticated envelope)', 'protected_string', 'Source string only in private importer; decrypt legacy authenticated ciphertext using domain key then re-encrypt (mail_account/seo_provider/webhook_endpoint and seo_setting when is_secret); preserve plaintext semantics for nonsecret seo_setting and unsubscribe_token inside new envelope; no logs/public DTO; preflight checks source type only, not decryption; opt-out token is business capability, not login token'
    if pair == ('seo_source', 'read_offset'):
        return 'DECIMAL(38,18)', 'quantity_decimal', 'Preserve exact legacy byte offset/raw pair; require nonnegative integer for collector use; source path/fingerprint must match before resuming; never read old production paths during import'
    if table.startswith('buyback_') and name in {'amount', 'reference_amount', 'final_amount'}:
        return 'DECIMAL(38,18)', 'money_major_decimal', 'Exact major currency decimal; NULL reference/final price remains NULL; raw pair must agree; no float64 or cents conversion'
    target, transform, assertion = commerce.convert(table, column)
    if name == 'storage_key':
        assertion += '; private file locator only; verify file manifest/checksum and rekey separately; never expose in DTO'
    if name == 'item_id' and table in {'buyback_photo', 'buyback_device_link'}:
        assertion += '; embedded buyback_requests.items item ID, not a standalone SQL entity; validate request membership'
    if name == 'created_by':
        assertion += '; retain legacy actor only; never FK or grant to new login'
    if table.startswith('mail_'):
        assertion += '; preserve business history/consent; imported queued or sending work held until reconciliation; invalidate old registration/reset mail, never replay on import'
    if table == 'seo_source':
        assertion += '; import collection disabled by migration hold until host path and fingerprint reviewed'
    return target, transform, assertion


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    commerce.catalog.generate(TABLES, convert, REFS, {}, 'operations',
                              'Custom business field design only; secrets/files/queue holds require isolated import verification; not an accepted implementation', args.check)
