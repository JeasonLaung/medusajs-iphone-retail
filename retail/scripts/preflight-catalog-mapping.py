"""Check ignored source snapshot against a field design; output aggregates only."""
import argparse
import hashlib
import json
import re
import subprocess
from collections import Counter
from datetime import datetime, timezone
from decimal import Decimal
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bundle", required=True)
    parser.add_argument("--domain", choices=["catalog", "inventory-device", "commerce", "checkout-rules"], default="catalog")
    args = parser.parse_args()
    bundle_path = Path(args.bundle).resolve()
    ignored = subprocess.run(["git", "check-ignore", "--quiet", str(bundle_path)], cwd=ROOT, capture_output=True)
    if ignored.returncode != 0:
        raise ValueError("Snapshot must be Git-ignored")
    raw = bundle_path.read_bytes()
    bundle = json.loads(raw, parse_float=Decimal)
    manifest = json.loads((ROOT / "docs/migration/data-source-manifest.json").read_text(encoding="utf-8-sig"))
    if hashlib.sha256(raw).hexdigest() != manifest["bundle_sha256"]:
        raise ValueError("Snapshot hash differs from recorded evidence; review inventory first")
    mapping_path = ROOT / "docs/migration" / f"{args.domain}-field-mapping.json"
    mapping = json.loads(mapping_path.read_text(encoding="utf-8-sig"))
    for key, source in [("source_manifest_sha256", "data-source-manifest.json"), ("source_constraints_sha256", "source-schema-constraints.json")]:
        content = (ROOT / "docs/migration" / source).read_bytes().replace(b'\r\n', b'\n')
        if hashlib.sha256(content).hexdigest() != mapping[key]:
            raise ValueError("Field mapping evidence is stale; regenerate and review before preflight")
    fields = mapping["fields"]
    source_by_target = {t["target_table"]: t["source_table"] for t in mapping["tables"]}
    for related in ("catalog", "inventory-device", "commerce", "checkout-rules"):
        related_mapping = json.loads((ROOT / "docs/migration" / f"{related}-field-mapping.json").read_text(encoding="utf-8-sig"))
        source_by_target.update({t["target_table"]: t["source_table"] for t in related_mapping["tables"]})
    source_by_target.update({"inventory_items": "inventory_item", "inventory_locations": "stock_location",
                             "payment_providers": "payment_provider", "fulfillment_shipping_options": "shipping_option",
                             "catalog_variants": "product_variant", "commerce_currencies": "currency",
                             "orders": "order", "order_line_items": "order_line_item", "fulfillments": "fulfillment", "returns": "return",
                             "historical_customer_subjects": "customer", "carts": "cart"})
    rename = {(f["target_table"], f["target_column"]): f["source_column"] for f in fields}
    failures = Counter()
    historical_unresolved = Counter()
    checks = Counter()
    reference_indexes = {}

    def fail(field, reason):
        failures[f"{field['source_table']}.{field['source_column']}:{reason}"] += 1

    for field in fields:
        name, table, transform = field["source_column"], field["source_table"], field["transform"]
        for row in bundle["tables"][table]:
            checks["field_values"] += 1
            if name not in row:
                fail(field, "missing")
                continue
            value = row[name]
            if value is None:
                if not field["nullable"]:
                    fail(field, "unexpected_null")
                continue
            if transform in {"identity_string", "binary_text", "code_string", "utf8_text", "historical_subject_ref", "device_identity_text", "enum_text"}:
                if not isinstance(value, str):
                    fail(field, "expected_string")
                    continue
                value.encode("utf-8", errors="strict")
                maximum = {"identity_string": 191, "historical_subject_ref": 191, "binary_text": 512, "code_string": 16, "enum_text": 32}.get(transform)
                if transform == "device_identity_text":
                    maximum = 191 if name == "serial_number" else 32 if name == "eid" else 15
                if maximum and len(value) > maximum:
                    fail(field, "length_overflow")
                if transform == "enum_text":
                    allowed = field["assertions"].split(";", 1)[0].removeprefix("Allowed: ").split()
                    if value not in allowed:
                        fail(field, "unknown_enum")
            elif transform == "strict_boolean":
                if not isinstance(value, bool):
                    fail(field, "expected_boolean")
            elif transform in {"checked_int32", "minor_unit_integer", "checked_int64"}:
                bits = 64 if transform == "checked_int64" else 32
                if isinstance(value, bool) or not isinstance(value, int) or not -(2**(bits-1)) <= value < 2**(bits-1):
                    fail(field, f"expected_int{bits}")
            elif transform == "utc_datetime6":
                try:
                    parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
                    if parsed.tzinfo is None or not 1000 <= parsed.astimezone(timezone.utc).year <= 9999:
                        raise ValueError("Timezone or range invalid")
                    fraction = re.search(r"\.(\d+)", value)
                    if fraction and len(fraction.group(1).rstrip('0')) > 6:
                        raise ValueError("Submicrosecond timestamp would lose precision")
                except (ValueError, TypeError, AttributeError):
                    fail(field, "invalid_timestamp")
            elif transform in {"money_major_decimal", "quantity_decimal", "rate_decimal", "finite_float32_to_float64", "legacy_float_rate_decimal", "budget_decimal", "promotion_value_decimal"}:
                try:
                    number = Decimal(str(value))
                    if not number.is_finite():
                        raise ValueError("Nonfinite")
                    if transform != "finite_float32_to_float64":
                        # Decimal.normalize() uses the current context and can round long inputs.
                        parts = number.as_tuple()
                        digits, exponent = list(parts.digits), parts.exponent
                        while len(digits) > 1 and digits[-1] == 0:
                            digits.pop()
                            exponent += 1
                        if max(1, number.adjusted() + 1) > 20 or max(0, -exponent) > 18:
                            fail(field, "decimal_precision_overflow")
                except (ValueError, ArithmeticError):
                    fail(field, "invalid_numeric")
            elif transform in {"exact_json_text", "protected_exact_json"}:
                # Already decoded with Decimal; lexical round-trip still needs importer/MySQL tests.
                if name.startswith("raw_"):
                    scalar = name.removeprefix("raw_")
                    if scalar in row and row[scalar] is not None:
                        checks["raw_scalar_pairs"] += 1
                        try:
                            if not isinstance(value, dict) or Decimal(str(value["value"])) != Decimal(str(row[scalar])):
                                fail(field, "raw_scalar_mismatch")
                        except (ValueError, ArithmeticError, KeyError):
                            fail(field, "invalid_raw_numeric")
            else:
                raise ValueError("Unimplemented preflight transform")
            if field["reference"]:
                reference = field["reference"].split(" ")[0]
                target_table, target_column = reference.split(".")
                source_table = source_by_target[target_table]
                source_column = rename.get((target_table, target_column), target_column)
                key = (source_table, source_column)
                if key not in reference_indexes:
                    reference_indexes[key] = {r[source_column] for r in bundle["tables"][source_table] if r.get(source_column) is not None}
                checks["references_including_soft_deleted"] += 1
                if value not in reference_indexes[key]:
                    if field.get("reference_policy") == "preserve-unresolved-on-soft-deleted-row-only" and row.get("deleted_at") is not None:
                        historical_unresolved[f"{table}.{name}"] += 1
                    else:
                        fail(field, "missing_reference")
    report = {"format_version": 1, "bundle_sha256": manifest["bundle_sha256"],
              "mapping_sha256": hashlib.sha256(mapping_path.read_bytes().replace(b'\r\n', b'\n')).hexdigest(),
              "scope": "Source snapshot scalar type/length/precision/raw-pair/reference preflight only; not MySQL import, active uniqueness, semantic joins, public DTO or JSON lexical acceptance",
              "tables": len(mapping["tables"]), "fields": len(fields),
              "checks": dict(checks), "failures": dict(sorted(failures.items()))}
    if args.domain != "catalog":
        report["historical_unresolved_references"] = dict(sorted(historical_unresolved.items()))
        report["history_disposition"] = "Preserve raw legacy reference in history plus nullable resolved reference; no fabricated entity or new-login FK; unresolved history is not relational acceptance"
    target = ROOT / "docs/migration" / f"{args.domain}-preflight-report.json"
    target.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"tables": report["tables"], "fields": report["fields"], "checks": dict(checks),
                      "failure_categories": len(failures), "failed_values": sum(failures.values()),
                      "historical_unresolved_references": sum(historical_unresolved.values())}))
    if failures:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
