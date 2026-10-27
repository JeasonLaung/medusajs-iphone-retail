"""Explicit inventory, device and after-sales field design; no business data reads."""
import argparse
from importlib.util import module_from_spec, spec_from_file_location
from pathlib import Path

spec = spec_from_file_location("catalog_mapping", Path(__file__).with_name("map-catalog-fields.py"))
catalog = module_from_spec(spec)
spec.loader.exec_module(catalog)
TIMES = catalog.TIMES
TABLES = {
    "inventory_item": ("inventory_items", f"id sku origin_country hs_code mid_code material weight length height width requires_shipping description title thumbnail metadata unit_of_measure {TIMES}"),
    "inventory_level": ("inventory_levels", f"id inventory_item_id location_id stocked_quantity reserved_quantity incoming_quantity metadata raw_stocked_quantity raw_reserved_quantity raw_incoming_quantity {TIMES}"),
    "reservation_item": ("inventory_reservations", f"id line_item_id location_id quantity external_id description created_by metadata inventory_item_id allow_backorder raw_quantity {TIMES}"),
    "stock_location": ("inventory_locations", f"id name address_id metadata {TIMES}"),
    "stock_location_address": ("inventory_location_addresses", f"id address_1 address_2 company city country_code phone province postal_code metadata {TIMES}"),
    "device": ("devices", f"id serial_number imei imei2 eid variant_id source_id location_id grade battery_health cost_cents currency_code bin notes category photos state {TIMES}"),
    "device_source": ("device_sources", f"id name type location_id {TIMES}"),
    "device_variant": ("device_variant_tracking", f"id variant_id inventory_item_id {TIMES}"),
    "device_supply": ("device_supply_commitments", f"id source_id variant_id quantity {TIMES}"),
    "device_assignment": ("device_assignments", f"id device_id order_id item_id source_id status fulfillment_id return_id after_sales_request_id replacement_of tracking_number {TIMES}"),
    "device_event": ("device_events", f"id device_id order_id actor_id action data {TIMES}"),
    "after_sales_request": ("after_sales_requests", f"id order_id customer_id type reason items status {TIMES}"),
}
REFS = {
    ("inventory_level", "inventory_item_id"): "inventory_items.id", ("inventory_level", "location_id"): "inventory_locations.id",
    ("reservation_item", "inventory_item_id"): "inventory_items.id", ("reservation_item", "location_id"): "inventory_locations.id",
    ("reservation_item", "line_item_id"): "order_line_items.id (T034; generic reservation linkage reviewed separately)",
    ("stock_location", "address_id"): "inventory_location_addresses.id",
    ("device", "variant_id"): "catalog_variants.id", ("device", "source_id"): "device_sources.id", ("device", "location_id"): "inventory_locations.id",
    ("device", "currency_code"): "commerce_currencies.code",
    ("device_source", "location_id"): "inventory_locations.id",
    ("device_variant", "variant_id"): "catalog_variants.id", ("device_variant", "inventory_item_id"): "inventory_items.id",
    ("device_supply", "source_id"): "device_sources.id", ("device_supply", "variant_id"): "catalog_variants.id",
    ("device_assignment", "device_id"): "devices.id", ("device_assignment", "order_id"): "orders.id",
    ("device_assignment", "item_id"): "order_line_items.id", ("device_assignment", "source_id"): "device_sources.id",
    ("device_assignment", "fulfillment_id"): "fulfillments.id", ("device_assignment", "return_id"): "returns.id",
    ("device_assignment", "after_sales_request_id"): "after_sales_requests.id", ("device_assignment", "replacement_of"): "device_assignments.id",
    ("device_event", "device_id"): "devices.id", ("device_event", "order_id"): "orders.id",
    ("after_sales_request", "order_id"): "orders.id",
    # actor/customer/created_by intentionally do not refer to newly created login accounts.
}
QUANTITIES = {("inventory_level", name) for name in ("stocked_quantity", "reserved_quantity", "incoming_quantity")} | {("reservation_item", "quantity")}
STATUS = {
    ("device", "state"): "pending inspection available reserved packed shipped repair repair_ready retired supplier_returned",
    ("device", "category"): "sale returned recycled", ("device_source", "type"): "self supplier",
    ("device_assignment", "status"): "reserved packed shipped returned canceled",
    ("after_sales_request", "type"): "return exchange repair", ("after_sales_request", "status"): "pending processing completed rejected",
}


def convert(table, column):
    name = column["column_name"]
    pair = (table, name)
    if pair in QUANTITIES:
        return "DECIMAL(38,18)", "quantity_decimal", "Exact source quantity; max20 integer/18 fraction digits; no rounding; preserve raw pair; not derived from device count"
    if pair in STATUS:
        return "VARCHAR(32) utf8mb4_0900_bin + CHECK", "enum_text", "Allowed: " + STATUS[pair] + "; state transitions enforced by domain commands, not arbitrary PATCH"
    if table == "device" and name in {"serial_number", "imei", "imei2", "eid"}:
        maximum = 191 if name == "serial_number" else 32 if name == "eid" else 15
        return f"VARCHAR({maximum}) utf8mb4_0900_bin", "device_identity_text", f"Preserve historical text and NULL; max{maximum}; new writes normalize SN/validate digits; active cross-column radio uniqueness; invalid historical rows reported without fabrication"
    if pair in {("device_event", "actor_id"), ("after_sales_request", "customer_id"), ("reservation_item", "created_by")}:
        return "VARCHAR(191) utf8mb4_0900_bin", "historical_subject_ref", "Original actor/customer reference only; <=191 characters; no FK or ownership grant to new login accounts; system/sentinel references may exist"
    if pair == ("device", "cost_cents"):
        return "BIGINT", "minor_unit_integer", "Source signed int32 retained exactly as integer minor units; no major-unit division during import; new writes nonnegative and checked int64"
    if name == "photos" and table == "device":
        return "LONGTEXT utf8mb4_0900_bin + CHECK JSON_VALID", "exact_json_text", "Preserve photo array and file IDs/URLs; private objects require manifest/hash and authorization; no static exposure"
    if name == "items" and table == "after_sales_request":
        return "LONGTEXT utf8mb4_0900_bin + CHECK JSON_VALID", "exact_json_text", "Preserve envelope including lines/tracking/events and all keys; serializeAfterSales unwraps lines only for DTO; do not flatten storage; nested IDs validated separately"
    if pair == ("device_event", "data"):
        return "LONGTEXT utf8mb4_0900_bin + CHECK JSON_VALID", "exact_json_text", "Preserve immutable audit evidence incl nonce/assignment/operation context; mask private fields in DTO/logs; no float64; do not replay events"
    return catalog.mapping(table, column)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    catalog.generate(TABLES, convert, REFS, {}, "inventory-device",
                     "Explicit inventory/device/after-sales field design only; historical login references are not new accounts; not accepted DDL/import", args.check,
                     {("device", "source_id"): "preserve-unresolved-on-soft-deleted-row-only",
                      ("reservation_item", "line_item_id"): "preserve-unresolved-on-soft-deleted-row-only"})


if __name__ == "__main__":
    main()
