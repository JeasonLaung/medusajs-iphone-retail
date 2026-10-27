"""Generate explicit catalog/configuration field design; no DB or business-row input."""
import argparse
import csv
import hashlib
import io
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "docs/migration"
TIMES = "created_at updated_at deleted_at"
# Each source column must be explicitly named. No fallback copying of unknown columns.
TABLES = {
    "currency": ("commerce_currencies", f"code symbol symbol_native decimal_digits rounding raw_rounding name {TIMES}"),
    "product": ("catalog_products", f"id title handle subtitle description is_giftcard status thumbnail weight length height width origin_country hs_code mid_code material collection_id type_id discountable external_id metadata {TIMES}"),
    "product_variant": ("catalog_variants", f"id title sku barcode ean upc allow_backorder manage_inventory hs_code origin_country mid_code material weight length height width metadata variant_rank product_id thumbnail {TIMES}"),
    "product_category": ("catalog_categories", f"id name description handle mpath is_active is_internal rank parent_category_id metadata external_id {TIMES}"),
    "product_category_product": ("catalog_category_products", "product_id product_category_id"),
    "product_collection": ("catalog_collections", f"id title handle metadata external_id {TIMES}"),
    "product_type": ("catalog_product_types", f"id value metadata external_id {TIMES}"),
    "product_tag": ("catalog_tags", f"id value metadata external_id {TIMES}"),
    "product_tags": ("catalog_product_tags", "product_id product_tag_id"),
    "product_option": ("catalog_options", f"id title metadata is_exclusive {TIMES}"),
    "product_option_value": ("catalog_option_values", f"id value option_id metadata rank {TIMES}"),
    "product_product_option": ("catalog_product_options", f"id product_id product_option_id {TIMES}"),
    "product_product_option_value": ("catalog_product_option_values", f"id product_product_option_id product_option_value_id {TIMES}"),
    "product_variant_option": ("catalog_variant_options", "variant_id option_value_id"),
    "image": ("catalog_images", f"id url metadata rank product_id {TIMES}"),
    "product_variant_product_image": ("catalog_variant_images", f"id variant_id image_id {TIMES}"),
    "price_set": ("pricing_price_sets", f"id {TIMES}"),
    "product_variant_price_set": ("catalog_variant_price_sets", f"id variant_id price_set_id {TIMES}"),
    "price": ("pricing_prices", f"id title price_set_id currency_code raw_amount rules_count price_list_id amount min_quantity max_quantity raw_min_quantity raw_max_quantity {TIMES}"),
    "price_rule": ("pricing_price_rules", f"id value priority price_id attribute operator {TIMES}"),
    "price_list": ("pricing_price_lists", f"id status starts_at ends_at rules_count title description type metadata {TIMES}"),
    "price_list_rule": ("pricing_price_list_rules", f"id price_list_id value attribute {TIMES}"),
    "price_preference": ("pricing_preferences", f"id attribute value is_tax_inclusive {TIMES}"),
    "product_sales_channel": ("catalog_product_channels", f"id product_id sales_channel_id {TIMES}"),
    "product_shipping_profile": ("catalog_product_shipping_profiles", f"id product_id shipping_profile_id {TIMES}"),
    "product_variant_inventory_item": ("catalog_variant_inventory_items", f"id variant_id inventory_item_id required_quantity {TIMES}"),
    "sales_channel": ("commerce_sales_channels", f"id name description is_disabled metadata {TIMES}"),
    "sales_channel_stock_location": ("commerce_channel_locations", f"id sales_channel_id stock_location_id {TIMES}"),
    "region": ("commerce_regions", f"id name currency_code metadata automatic_taxes {TIMES}"),
    "region_country": ("commerce_countries", f"iso_2 iso_3 num_code name display_name region_id metadata {TIMES}"),
    "region_payment_provider": ("commerce_region_payment_providers", f"id region_id payment_provider_id {TIMES}"),
    "store": ("commerce_stores", f"id name default_sales_channel_id default_region_id default_location_id metadata {TIMES}"),
    "store_currency": ("commerce_store_currencies", f"id currency_code is_default store_id {TIMES}"),
    "store_locale": ("commerce_store_locales", f"id locale_code store_id {TIMES}"),
    "shipping_profile": ("fulfillment_shipping_profiles", f"id name type metadata {TIMES}"),
    "shipping_option_price_set": ("fulfillment_option_price_sets", f"id shipping_option_id price_set_id {TIMES}"),
}
MONEY = {("price", "amount"), ("currency", "rounding")}
QUANTITY = {("price", "min_quantity"), ("price", "max_quantity"), ("product_variant_inventory_item", "required_quantity")}
# Explicit joins; cross-domain targets remain references, not fake empty records.
REFS = {
    ("product", "collection_id"): "catalog_collections.id", ("product", "type_id"): "catalog_product_types.id",
    ("product_variant", "product_id"): "catalog_products.id", ("product_category", "parent_category_id"): "catalog_categories.id",
    ("product_category_product", "product_id"): "catalog_products.id", ("product_category_product", "product_category_id"): "catalog_categories.id",
    ("product_tags", "product_id"): "catalog_products.id", ("product_tags", "product_tag_id"): "catalog_tags.id",
    ("product_option_value", "option_id"): "catalog_options.id",
    ("product_product_option", "product_id"): "catalog_products.id", ("product_product_option", "product_option_id"): "catalog_options.id",
    ("product_product_option_value", "product_product_option_id"): "catalog_product_options.id", ("product_product_option_value", "product_option_value_id"): "catalog_option_values.id",
    ("product_variant_option", "variant_id"): "catalog_variants.id", ("product_variant_option", "option_value_id"): "catalog_option_values.id",
    ("image", "product_id"): "catalog_products.id", ("product_variant_product_image", "variant_id"): "catalog_variants.id", ("product_variant_product_image", "image_id"): "catalog_images.id",
    ("product_variant_price_set", "variant_id"): "catalog_variants.id", ("product_variant_price_set", "price_set_id"): "pricing_price_sets.id",
    ("price", "price_set_id"): "pricing_price_sets.id", ("price", "price_list_id"): "pricing_price_lists.id", ("price", "currency_code"): "commerce_currencies.code",
    ("price_rule", "price_id"): "pricing_prices.id", ("price_list_rule", "price_list_id"): "pricing_price_lists.id",
    ("product_sales_channel", "product_id"): "catalog_products.id", ("product_sales_channel", "sales_channel_id"): "commerce_sales_channels.id",
    ("product_shipping_profile", "product_id"): "catalog_products.id", ("product_shipping_profile", "shipping_profile_id"): "fulfillment_shipping_profiles.id",
    ("product_variant_inventory_item", "variant_id"): "catalog_variants.id", ("product_variant_inventory_item", "inventory_item_id"): "inventory_items.id (T031)",
    ("sales_channel_stock_location", "sales_channel_id"): "commerce_sales_channels.id", ("sales_channel_stock_location", "stock_location_id"): "inventory_locations.id (T031)",
    ("region", "currency_code"): "commerce_currencies.code", ("region_country", "region_id"): "commerce_regions.id",
    ("region_payment_provider", "region_id"): "commerce_regions.id", ("region_payment_provider", "payment_provider_id"): "payment_providers.id (T035)",
    ("store", "default_sales_channel_id"): "commerce_sales_channels.id", ("store", "default_region_id"): "commerce_regions.id", ("store", "default_location_id"): "inventory_locations.id (T031)",
    ("store_currency", "store_id"): "commerce_stores.id", ("store_currency", "currency_code"): "commerce_currencies.code", ("store_locale", "store_id"): "commerce_stores.id",
    ("shipping_option_price_set", "shipping_option_id"): "fulfillment_shipping_options.id (T033)", ("shipping_option_price_set", "price_set_id"): "pricing_price_sets.id",
}
RENAMES = {("product_category_product", "product_category_id"): "category_id", ("product_tags", "product_tag_id"): "tag_id"}


def mapping(table, column):
    name, udt = column["column_name"], column["udt_name"]
    pair = (table, name)
    if udt == "timestamptz":
        return "DATETIME(6)", "utc_datetime6", "UTC instant; reject invalid/out-of-range; no epoch/default-now fallback"
    if udt in {"json", "jsonb"}:
        assertion = "exact_json_use_number; preserve JSON null distinct from SQL NULL; internal only"
        if name.startswith("raw_"):
            assertion += "; compare value/precision to scalar; mismatch blocks import"
        return "LONGTEXT utf8mb4_0900_bin + CHECK JSON_VALID", "exact_json_text", assertion + "; preserve numeric lexemes; no MySQL JSON numeric reserialization"
    if udt == "bool":
        return "BOOLEAN", "strict_boolean", "Only actual true/false; preserve false; no truthy coercion"
    if udt == "int4":
        return "INT", "checked_int32", "Exact signed int32; preserve NULL; no zero fallback"
    if udt == "float4":
        return "DOUBLE", "finite_float32_to_float64", "Dimensions only; preserve source float32; reject NaN/Infinity; never use for money"
    if pair in MONEY | QUANTITY:
        kind = "money_major_decimal" if pair in MONEY else "quantity_decimal"
        return "DECIMAL(38,18)", kind, "Lexical decimal; max20 integer/18 fraction digits; reject overflow or scale loss; raw pair must agree; no float64"
    if udt in {"text", "varchar"}:
        if name == "id" or (name.endswith("_id") and name != "external_id"):
            return "VARCHAR(191) utf8mb4_0900_bin", "identity_string", "Preserve case and bytes as UTF-8 text; <=191 characters; no trim/rekey; check reference incl soft-deleted rows"
        if name in {"handle", "sku", "barcode", "ean", "upc"}:
            return "VARCHAR(512) utf8mb4_0900_bin", "binary_text", "Preserve case/blank/NULL; <=512 characters; active-only uniqueness where source requires"
        if name in {"code", "currency_code", "iso_2", "iso_3", "num_code"}:
            return "VARCHAR(16) utf8mb4_0900_bin", "code_string", "Preserve source text incl leading zeros/case; <=16 characters; normalize only new validated writes"
        return "LONGTEXT", "utf8_text", "Preserve exact UTF-8 text incl empty/whitespace; no truncation; indexed fields need explicit key design"
    raise ValueError(f"No reviewed conversion for {table}.{name}: {udt}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    generate(TABLES, mapping, REFS, RENAMES, "catalog",
             "Explicit catalog/pricing/region/channel/store field design only; not accepted DDL/import or all193 tables", args.check)


def generate(declarations, convert, references, renames, prefix, scope, check, reference_policies=None):
    manifest_path = OUT / "data-source-manifest.json"
    constraints_path = OUT / "source-schema-constraints.json"
    raw = manifest_path.read_bytes()
    manifest = json.loads(raw)
    constraints_raw = constraints_path.read_bytes()
    constraints = json.loads(constraints_raw)
    source = {t["source_table"]: t for t in manifest["tables"]}
    records = []
    for table, (target, declared) in declarations.items():
        columns = source[table]["columns"]
        names = declared.split()
        if len(names) != len(set(names)) or set(names) != {c["column_name"] for c in columns}:
            raise ValueError(f"Explicit field list no longer covers {table}; review schema change")
        for c in columns:
            name = c["column_name"]
            target_type, transform, assertions = convert(table, c)
            records.append({"source_table": table, "source_column": name, "source_type": c["udt_name"],
                            "source_expression": f"{table}.{name}", "target_table": target,
                            "target_column": renames.get((table, name), name), "target_type_design": target_type,
                            "nullable": c["is_nullable"] == "YES", "missing_policy": "reject missing field; preserve explicit NULL only when nullable",
                            "transform": transform, "reference": references.get((table, name), ""),
                            "assertions": assertions, "status": "designed-not-implemented"})
            if reference_policies is not None:
                records[-1]["reference_policy"] = reference_policies.get((table, name), "require-existing-if-nonnull") if (table, name) in references else "not-an-entity-reference"
    tables = []
    for name, (target, _) in declarations.items():
        tables.append({"source_table": name, "target_table": target, "status": "designed-not-implemented",
                       "source_columns": len(source[name]["columns"]),
                       "source_constraint_names": [r["constraint_name"] for r in constraints["constraints"] if r["source_table"] == name],
                       "source_index_names": [r["index_name"] for r in constraints["indexes"] if r["source_table"] == name]})
    result = {"format_version": 1, "source_commit": manifest["source_commit"],
              "scope": scope,
              "source_manifest_sha256": hashlib.sha256(raw.replace(b'\r\n', b'\n')).hexdigest(),
              "source_constraints_sha256": hashlib.sha256(constraints_raw.replace(b'\r\n', b'\n')).hexdigest(),
              "tables": tables, "fields": records}
    csv_output = io.StringIO(newline="")
    writer = csv.DictWriter(csv_output, fieldnames=list(records[0]), lineterminator="\n")
    writer.writeheader()
    writer.writerows(records)
    outputs = {OUT / f"{prefix}-field-mapping.json": json.dumps(result, ensure_ascii=False, indent=2) + "\n",
               OUT / f"{prefix}-field-mapping.csv": csv_output.getvalue()}
    for target, content in outputs.items():
        if check:
            if target.read_text(encoding="utf-8-sig") != content:
                raise ValueError(f"Mapping changed; review before regenerating {target.name}")
        else:
            target.write_text(content, encoding="utf-8", newline="")
    print(json.dumps({"tables": len(tables), "fields": len(records), "remaining_tables": len(source) - len(tables),
                      "status": "design only; no MySQL import", "mode": "check" if check else "generate"}))


if __name__ == "__main__":
    main()
