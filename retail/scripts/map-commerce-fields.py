"""Explicit order/payment/fulfillment/return field design. No database or row reads."""
import argparse
from importlib.util import module_from_spec, spec_from_file_location
from pathlib import Path

spec = spec_from_file_location("catalog_mapping", Path(__file__).with_name("map-catalog-fields.py"))
catalog = module_from_spec(spec)
spec.loader.exec_module(catalog)
T = catalog.TIMES
TABLES = {
    "order": ("orders", f"id region_id display_id customer_id version sales_channel_id status is_draft_order email currency_code shipping_address_id billing_address_id no_notification metadata canceled_at custom_display_id locale {T}"),
    "order_address": ("order_addresses", f"id customer_id company first_name last_name address_1 address_2 city country_code province postal_code phone metadata {T}"),
    "order_cart": ("order_carts", f"id order_id cart_id {T}"),
    "order_item": ("order_items", f"id order_id version item_id quantity raw_quantity fulfilled_quantity raw_fulfilled_quantity shipped_quantity raw_shipped_quantity return_requested_quantity raw_return_requested_quantity return_received_quantity raw_return_received_quantity return_dismissed_quantity raw_return_dismissed_quantity written_off_quantity raw_written_off_quantity metadata delivered_quantity raw_delivered_quantity unit_price raw_unit_price compare_at_unit_price raw_compare_at_unit_price {T}"),
    "order_line_item": ("order_line_items", f"id totals_id title subtitle thumbnail variant_id product_id product_title product_description product_subtitle product_type product_collection product_handle variant_sku variant_barcode variant_title variant_option_values requires_shipping is_discountable is_tax_inclusive compare_at_unit_price raw_compare_at_unit_price unit_price raw_unit_price metadata is_custom_price product_type_id is_giftcard {T}"),
    "order_line_item_adjustment": ("order_line_adjustments", f"id description promotion_id code amount raw_amount provider_id item_id is_tax_inclusive version {T}"),
    "order_line_item_tax_line": ("order_line_tax_lines", f"id description tax_rate_id code rate raw_rate provider_id item_id metadata data {T}"),
    "order_shipping": ("order_shipping_versions", f"id order_id version shipping_method_id return_id claim_id exchange_id {T}"),
    "order_shipping_method": ("order_shipping_methods", f"id name description amount raw_amount is_tax_inclusive shipping_option_id data metadata is_custom_amount {T}"),
    "order_shipping_method_adjustment": ("order_shipping_adjustments", f"id description promotion_id code amount raw_amount provider_id shipping_method_id version {T}"),
    "order_shipping_method_tax_line": ("order_shipping_tax_lines", f"id description tax_rate_id code rate raw_rate provider_id shipping_method_id metadata data {T}"),
    "order_summary": ("order_summaries", f"id order_id version totals {T}"),
    "order_transaction": ("order_transactions", f"id order_id version amount raw_amount currency_code reference reference_id return_id claim_id exchange_id {T}"),
    "order_credit_line": ("order_credit_lines", f"id order_id reference reference_id amount raw_amount metadata version {T}"),
    "order_payment_collection": ("order_payment_collections", f"id order_id payment_collection_id {T}"),
    "order_fulfillment": ("order_fulfillments", f"id order_id fulfillment_id {T}"),
    "order_promotion": ("order_promotions", f"id order_id promotion_id {T}"),
    "order_change": ("order_changes", f"id order_id version description status internal_note created_by requested_by requested_at confirmed_by confirmed_at declined_by declined_reason metadata declined_at canceled_by canceled_at change_type return_id claim_id exchange_id carry_over_promotions no_notification {T}"),
    "order_change_action": ("order_change_actions", f"id order_id version ordering order_change_id reference reference_id action details amount raw_amount internal_note applied return_id claim_id exchange_id {T}"),
    "order_claim": ("order_claims", f"id order_id return_id order_version display_id type no_notification refund_amount raw_refund_amount metadata canceled_at created_by {T}"),
    "order_claim_item": ("order_claim_items", f"id claim_id item_id is_additional_item reason quantity raw_quantity note metadata {T}"),
    "order_claim_item_image": ("order_claim_item_images", f"id claim_item_id url metadata {T}"),
    "order_exchange": ("order_exchanges", f"id order_id return_id order_version display_id no_notification allow_backorder difference_due raw_difference_due metadata canceled_at created_by {T}"),
    "order_exchange_item": ("order_exchange_items", f"id exchange_id item_id quantity raw_quantity note metadata {T}"),
    "return": ("returns", f"id order_id claim_id exchange_id order_version display_id status no_notification refund_amount raw_refund_amount metadata received_at canceled_at location_id requested_at created_by {T}"),
    "return_item": ("return_items", f"id return_id reason_id item_id quantity raw_quantity received_quantity raw_received_quantity note metadata damaged_quantity raw_damaged_quantity {T}"),
    "return_reason": ("return_reasons", f"id value label description metadata parent_return_reason_id {T}"),
    "return_fulfillment": ("return_fulfillments", f"id return_id fulfillment_id {T}"),
    "payment_collection": ("payment_collections", f"id currency_code amount raw_amount authorized_amount raw_authorized_amount captured_amount raw_captured_amount refunded_amount raw_refunded_amount completed_at status metadata {T}"),
    "payment_session": ("payment_sessions", f"id currency_code amount raw_amount provider_id data context status authorized_at payment_collection_id metadata {T}"),
    "payment": ("payments", f"id amount raw_amount currency_code provider_id data captured_at canceled_at payment_collection_id payment_session_id metadata {T}"),
    "capture": ("payment_captures", f"id amount raw_amount payment_id created_by metadata {T}"),
    "refund": ("payment_refunds", f"id amount raw_amount payment_id created_by metadata refund_reason_id note {T}"),
    "refund_reason": ("payment_refund_reasons", f"id label description metadata code {T}"),
    "payment_provider": ("payment_providers", f"id is_enabled {T}"),
    "payment_collection_payment_providers": ("payment_collection_providers", "payment_collection_id payment_provider_id"),
    "account_holder": ("payment_account_holders", f"id provider_id external_id email data metadata {T}"),
    "fulfillment": ("fulfillments", f"id location_id packed_at shipped_at delivered_at canceled_at data provider_id shipping_option_id metadata delivery_address_id marked_shipped_by created_by requires_shipping {T}"),
    "fulfillment_address": ("fulfillment_addresses", f"id company first_name last_name address_1 address_2 city country_code province postal_code phone metadata {T}"),
    "fulfillment_item": ("fulfillment_items", f"id title sku barcode quantity raw_quantity line_item_id inventory_item_id fulfillment_id {T}"),
    "fulfillment_label": ("fulfillment_labels", f"id tracking_number tracking_url label_url fulfillment_id {T}"),
    "fulfillment_provider": ("fulfillment_providers", f"id is_enabled {T}"),
}
REFS = {}
# These are reviewed semantic references, not column-name guesses.
def refs(tables, field, target):
    for table in tables.split():
        REFS[table, field] = target

refs("order order_address", "customer_id", "historical_customer_subjects.id (history only; no new-login ownership)")
refs("order", "region_id", "commerce_regions.id")
refs("order", "sales_channel_id", "commerce_sales_channels.id")
refs("order", "shipping_address_id", "order_addresses.id")
refs("order", "billing_address_id", "order_addresses.id")
refs("order order_transaction payment payment_session payment_collection", "currency_code", "commerce_currencies.code")
refs("order_cart order_item order_shipping order_summary order_transaction order_credit_line order_payment_collection order_fulfillment order_promotion order_change order_change_action order_claim order_exchange return", "order_id", "orders.id")
refs("order_cart", "cart_id", "carts.id (in-flight/history boundary T032)")
refs("order_item order_line_item_adjustment order_line_item_tax_line order_claim_item order_exchange_item return_item", "item_id", "order_line_items.id")
refs("order_shipping order_shipping_method_adjustment order_shipping_method_tax_line", "shipping_method_id", "order_shipping_methods.id")
refs("order_payment_collection payment payment_session payment_collection_payment_providers", "payment_collection_id", "payment_collections.id")
refs("order_fulfillment return_fulfillment fulfillment_item fulfillment_label", "fulfillment_id", "fulfillments.id")
refs("payment", "payment_session_id", "payment_sessions.id")
refs("payment payment_session account_holder", "provider_id", "payment_providers.id")
refs("payment_collection_payment_providers", "payment_provider_id", "payment_providers.id")
refs("capture refund", "payment_id", "payments.id")
refs("refund", "refund_reason_id", "payment_refund_reasons.id")
refs("order_change_action", "order_change_id", "order_changes.id")
refs("order_claim_item", "claim_id", "order_claims.id")
refs("order_claim_item_image", "claim_item_id", "order_claim_items.id")
refs("order_exchange_item", "exchange_id", "order_exchanges.id")
refs("return_item return_fulfillment order_change order_change_action order_claim order_exchange order_shipping order_transaction", "return_id", "returns.id")
refs("return order_change order_change_action order_shipping order_transaction", "claim_id", "order_claims.id")
refs("return order_change order_change_action order_shipping order_transaction", "exchange_id", "order_exchanges.id")
refs("return_item", "reason_id", "return_reasons.id")
refs("return_reason", "parent_return_reason_id", "return_reasons.id")
refs("fulfillment return", "location_id", "inventory_locations.id")
refs("fulfillment", "delivery_address_id", "fulfillment_addresses.id")
refs("fulfillment", "provider_id", "fulfillment_providers.id")
refs("fulfillment order_shipping_method", "shipping_option_id", "fulfillment_shipping_options.id")
refs("fulfillment_item", "line_item_id", "order_line_items.id")
refs("fulfillment_item", "inventory_item_id", "inventory_items.id")
# Snapshot product/variant/promotion/tax and polymorphic reference IDs preserve original identifiers,
# not mandatory live-catalog FKs. Source structural FKs are separately recorded by generate().
STATUS = {
    ("order", "status"): "pending completed draft archived canceled requires_action",
    ("order_change", "status"): "confirmed declined requested pending canceled",
    ("order_claim", "type"): "refund replace",
    ("order_claim_item", "reason"): "missing_item wrong_item production_failure other",
    ("return", "status"): "open requested received partially_received canceled",
    ("payment_collection", "status"): "not_paid awaiting authorized partially_authorized canceled failed partially_captured completed",
    ("payment_session", "status"): "authorized captured pending requires_more error canceled pending_authorization",
}
ACTOR_FIELDS = {"created_by", "requested_by", "confirmed_by", "declined_by", "canceled_by", "marked_shipped_by"}


def convert(table, column):
    name, udt = column["column_name"], column["udt_name"]
    if (table, name) in STATUS:
        return "VARCHAR(32) utf8mb4_0900_bin + CHECK", "enum_text", "Allowed: " + STATUS[table, name] + "; preserve source status; legal transitions require domain command"
    if udt == "numeric":
        kind = "quantity_decimal" if "quantity" in name else "rate_decimal" if name == "rate" else "money_major_decimal"
        return "DECIMAL(38,18)", kind, "Exact source decimal; max20 integer/18 fraction digits; reject scale loss; compare raw pair; signed credit/difference preserved; no float64 or cents conversion"
    if udt == "int8":
        return "BIGINT", "checked_int64", "Exact signed64; JSON API string; no JS Number; preserve action ordering"
    if name in ACTOR_FIELDS:
        return "VARCHAR(191) utf8mb4_0900_bin", "historical_subject_ref", "Preserve actor/system reference; <=191 characters; no new-login account FK or copied authorization"
    if table == "order" and name == "custom_display_id":
        return "VARCHAR(191) utf8mb4_0900_bin", "historical_subject_ref", "Preserve display number exactly as text; never numeric conversion; retain legacy display_id separately"
    if table == "refund_reason" and name == "code":
        return "VARCHAR(191) utf8mb4_0900_bin", "identity_string", "Refund reason code is a business key, not a currency/country code; preserve up to191 characters without truncation"
    if table in {"payment_session", "payment", "account_holder"} and name in {"data", "context", "metadata"}:
        return "LONGTEXT utf8mb4_0900_bin encrypted envelope", "protected_exact_json", "Exact JSON retained in protected migration envelope; provider refs/state allowlist extracted with provenance; tokens/customer/provider secrets encrypted or omitted only after explicit reconciliation; never log/public DTO; no callbacks/replay during import"
    if table == "order_summary" and name == "totals":
        return "LONGTEXT utf8mb4_0900_bin + CHECK JSON_VALID", "exact_json_text", "Exact versioned totals snapshot; preserve all decimal lexemes; compare against detailed ledger separately; never replace using current catalog prices"
    return catalog.mapping(table, column)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    catalog.generate(TABLES, convert, REFS, {}, "commerce",
                     "Explicit order/payment/fulfillment/return field design only; protected provider state and historical subjects are not login accounts; not accepted DDL/import", args.check)


if __name__ == "__main__":
    main()
