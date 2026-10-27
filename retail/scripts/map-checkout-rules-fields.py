"""Explicit cart/promotion/tax/shipping configuration field design, without source data access."""
import argparse
from importlib.util import module_from_spec, spec_from_file_location
from pathlib import Path

spec = spec_from_file_location("commerce_mapping", Path(__file__).with_name("map-commerce-fields.py"))
commerce = module_from_spec(spec)
spec.loader.exec_module(commerce)
T = commerce.T
TABLES = {
    "cart": ("carts", f"id region_id customer_id sales_channel_id email currency_code shipping_address_id billing_address_id metadata completed_at locale {T}"),
    "cart_address": ("cart_addresses", f"id customer_id company first_name last_name address_1 address_2 city country_code province postal_code phone metadata {T}"),
    "cart_line_item": ("cart_line_items", f"id cart_id title subtitle thumbnail quantity variant_id product_id product_title product_description product_subtitle product_type product_collection product_handle variant_sku variant_barcode variant_title variant_option_values requires_shipping is_discountable is_tax_inclusive compare_at_unit_price raw_compare_at_unit_price unit_price raw_unit_price metadata product_type_id is_custom_price is_giftcard {T}"),
    "cart_line_item_adjustment": ("cart_line_adjustments", f"id description promotion_id code amount raw_amount provider_id metadata item_id is_tax_inclusive {T}"),
    "cart_line_item_tax_line": ("cart_line_tax_lines", f"id description tax_rate_id code rate provider_id metadata item_id data {T}"),
    "cart_payment_collection": ("cart_payment_collections", f"id cart_id payment_collection_id {T}"),
    "cart_promotion": ("cart_promotions", f"id cart_id promotion_id {T}"),
    "cart_shipping_method": ("cart_shipping_methods", f"id cart_id name description amount raw_amount is_tax_inclusive shipping_option_id data metadata {T}"),
    "cart_shipping_method_adjustment": ("cart_shipping_adjustments", f"id description promotion_id code amount raw_amount provider_id metadata shipping_method_id {T}"),
    "cart_shipping_method_tax_line": ("cart_shipping_tax_lines", f"id description tax_rate_id code rate provider_id metadata shipping_method_id data {T}"),
    "credit_line": ("cart_credit_lines", f"id cart_id reference reference_id amount raw_amount metadata {T}"),
    "promotion": ("promotions", f"id code campaign_id is_automatic type status is_tax_inclusive limit used metadata {T}"),
    "promotion_application_method": ("promotion_application_methods", f"id value raw_value max_quantity apply_to_quantity buy_rules_min_quantity type target_type allocation promotion_id currency_code {T}"),
    "promotion_campaign": ("promotion_campaigns", f"id name description campaign_identifier starts_at ends_at {T}"),
    "promotion_campaign_budget": ("promotion_campaign_budgets", f"id type campaign_id limit raw_limit used raw_used currency_code attribute {T}"),
    "promotion_campaign_budget_usage": ("promotion_budget_usages", f"id attribute_value used budget_id raw_used {T}"),
    "promotion_promotion_rule": ("promotion_order_rules", "promotion_id promotion_rule_id"),
    "promotion_rule": ("promotion_rules", f"id description attribute operator {T}"),
    "promotion_rule_value": ("promotion_rule_values", f"id promotion_rule_id value {T}"),
    "application_method_buy_rules": ("promotion_buy_rules", "application_method_id promotion_rule_id"),
    "application_method_target_rules": ("promotion_target_rules", "application_method_id promotion_rule_id"),
    "tax_provider": ("tax_providers", f"id is_enabled {T}"),
    "tax_region": ("tax_regions", f"id provider_id country_code province_code parent_id metadata created_by {T}"),
    "tax_rate": ("tax_rates", f"id rate code name is_default is_combinable tax_region_id metadata created_by {T}"),
    "tax_rate_rule": ("tax_rate_rules", f"id tax_rate_id reference_id reference metadata created_by {T}"),
    "fulfillment_set": ("fulfillment_sets", f"id name type metadata {T}"),
    "service_zone": ("fulfillment_service_zones", f"id name metadata fulfillment_set_id {T}"),
    "geo_zone": ("fulfillment_geo_zones", f"id type country_code province_code city service_zone_id postal_expression metadata {T}"),
    "shipping_option": ("fulfillment_shipping_options", f"id name price_type service_zone_id shipping_profile_id provider_id data metadata shipping_option_type_id {T}"),
    "shipping_option_rule": ("fulfillment_option_rules", f"id attribute operator value shipping_option_id {T}"),
    "shipping_option_type": ("fulfillment_option_types", f"id label description code {T}"),
    "location_fulfillment_provider": ("fulfillment_location_providers", f"id stock_location_id fulfillment_provider_id {T}"),
    "location_fulfillment_set": ("fulfillment_location_sets", f"id stock_location_id fulfillment_set_id {T}"),
}
REFS = {}
def refs(tables, field, target):
    for table in tables.split():
        REFS[table, field] = target

refs("cart", "region_id", "commerce_regions.id")
refs("cart cart_address", "customer_id", "historical_customer_subjects.id (not login identity)")
refs("cart", "sales_channel_id", "commerce_sales_channels.id")
refs("cart", "currency_code", "commerce_currencies.code")
refs("cart", "shipping_address_id", "cart_addresses.id")
refs("cart", "billing_address_id", "cart_addresses.id")
refs("cart_line_item cart_payment_collection cart_promotion cart_shipping_method credit_line", "cart_id", "carts.id")
refs("cart_line_item_adjustment cart_line_item_tax_line", "item_id", "cart_line_items.id")
refs("cart_shipping_method_adjustment cart_shipping_method_tax_line", "shipping_method_id", "cart_shipping_methods.id")
refs("cart_payment_collection", "payment_collection_id", "payment_collections.id")
refs("cart_promotion", "promotion_id", "promotions.id")
refs("cart_shipping_method", "shipping_option_id", "fulfillment_shipping_options.id")
refs("promotion promotion_campaign_budget", "campaign_id", "promotion_campaigns.id")
refs("promotion_application_method promotion_promotion_rule", "promotion_id", "promotions.id")
refs("promotion_application_method promotion_campaign_budget", "currency_code", "commerce_currencies.code")
refs("promotion_campaign_budget_usage", "budget_id", "promotion_campaign_budgets.id")
refs("promotion_promotion_rule promotion_rule_value application_method_buy_rules application_method_target_rules", "promotion_rule_id", "promotion_rules.id")
refs("application_method_buy_rules application_method_target_rules", "application_method_id", "promotion_application_methods.id")
refs("tax_region", "provider_id", "tax_providers.id")
refs("tax_region", "parent_id", "tax_regions.id")
refs("tax_rate", "tax_region_id", "tax_regions.id")
refs("tax_rate_rule", "tax_rate_id", "tax_rates.id")
refs("service_zone location_fulfillment_set", "fulfillment_set_id", "fulfillment_sets.id")
refs("geo_zone shipping_option", "service_zone_id", "fulfillment_service_zones.id")
refs("shipping_option", "shipping_profile_id", "fulfillment_shipping_profiles.id")
refs("shipping_option", "provider_id", "fulfillment_providers.id")
refs("shipping_option", "shipping_option_type_id", "fulfillment_option_types.id")
refs("shipping_option_rule", "shipping_option_id", "fulfillment_shipping_options.id")
refs("location_fulfillment_provider location_fulfillment_set", "stock_location_id", "inventory_locations.id")
refs("location_fulfillment_provider", "fulfillment_provider_id", "fulfillment_providers.id")
STATUS = {
    ("promotion", "status"): "draft active inactive", ("promotion", "type"): "standard buyget",
    ("promotion_application_method", "allocation"): "each across once",
    ("promotion_application_method", "type"): "fixed percentage",
    ("promotion_application_method", "target_type"): "order shipping_methods items",
    ("promotion_campaign_budget", "type"): "spend usage use_by_attribute spend_by_attribute",
    ("promotion_rule", "operator"): "gte lte gt lt eq ne in",
    ("shipping_option_rule", "operator"): "in eq ne gt gte lt lte nin",
    ("shipping_option", "price_type"): "calculated flat", ("geo_zone", "type"): "country province city zip",
}


def convert(table, column):
    name, udt = column['column_name'], column['udt_name']
    if (table, name) in STATUS:
        return "VARCHAR(32) utf8mb4_0900_bin + CHECK", "enum_text", "Allowed: " + STATUS[table, name] + "; unsupported runtime types must fail explicitly, not silently apply"
    if udt == 'float4' and name == 'rate':
        return "DECIMAL(38,18)", "legacy_float_rate_decimal", "Read source float32 decimal rendering exactly; preserve source precision evidence; reject nonfinite/scale loss; new tax writes exact decimal; never reuse dimension-float conversion"
    if table == 'promotion_campaign_budget' and name in {'limit', 'used'} or table == 'promotion_campaign_budget_usage' and name == 'used':
        return "DECIMAL(38,18)", "budget_decimal", "Exact decimal; count or currency unit depends on budget.type; no automatic cents conversion; preserve raw pair; commit under durable budget lock"
    if name == 'code':
        return "VARCHAR(191) utf8mb4_0900_bin", "identity_string", "Business code, not ISO currency/country; preserve case/text up to191; active uniqueness follows source; no truncation"
    if table == 'promotion_application_method' and name == 'value':
        return "DECIMAL(38,18)", "promotion_value_decimal", "Exact value; percentage or major currency units according to type; keep raw value; no float64; validate type-specific ranges on new writes"
    return commerce.convert(table, column)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    commerce.catalog.generate(TABLES, convert, REFS, {}, 'checkout-rules',
                              'Explicit cart/promotion/tax/fulfillment configuration field design only; no session/identity restoration or accepted MySQL implementation', args.check)


if __name__ == '__main__':
    main()
