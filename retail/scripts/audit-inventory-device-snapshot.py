"""Audit an ignored legacy snapshot without mutation; publish aggregate issues, never row IDs."""
import argparse
import hashlib
import json
import subprocess
from collections import Counter, defaultdict
from decimal import Decimal
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
ACTIVE_ASSIGNMENTS = {"reserved", "packed", "shipped"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bundle", required=True)
    args = parser.parse_args()
    bundle_path = Path(args.bundle).resolve()
    if subprocess.run(["git", "check-ignore", "--quiet", str(bundle_path)], cwd=ROOT, capture_output=True).returncode:
        raise ValueError("Source bundle must be Git-ignored")
    raw = bundle_path.read_bytes()
    manifest = json.loads((ROOT / "docs/migration/data-source-manifest.json").read_text(encoding="utf-8-sig"))
    if hashlib.sha256(raw).hexdigest() != manifest["bundle_sha256"]:
        raise ValueError("Snapshot differs from recorded evidence")
    tables = json.loads(raw, parse_float=Decimal)["tables"]
    active = {name: [r for r in rows if r.get("deleted_at") is None] for name, rows in tables.items()}
    indexes = {name: {r["id"]: r for r in rows if "id" in r} for name, rows in tables.items()}
    checks, blockers, history, observations = Counter(), Counter(), Counter(), Counter()
    unique_specs = [
        ("inventory_item", ("sku",), None), ("inventory_level", ("inventory_item_id", "location_id"), None),
        ("stock_location", ("address_id",), None), ("device_source", ("location_id",), None),
        ("device_variant", ("variant_id",), None), ("device_variant", ("inventory_item_id",), None),
        ("device_supply", ("source_id", "variant_id"), None),
        ("device", ("serial_number",), None), ("device", ("imei",), None), ("device", ("imei2",), None), ("device", ("eid",), None),
        ("device_assignment", ("device_id",), lambda r: r["status"] in ACTIVE_ASSIGNMENTS),
        ("after_sales_request", ("order_id",), lambda r: r["status"] in {"pending", "processing"}),
    ]
    for table, fields, predicate in unique_specs:
        seen = set()
        for row in active[table]:
            if predicate and not predicate(row):
                continue
            key = tuple(row[field] for field in fields)
            if None in key:  # PostgreSQL default NULLS DISTINCT.
                continue
            checks["active_unique_keys"] += 1
            if key in seen:
                blockers[f"{table}:{'+'.join(fields)}:duplicate_active_key"] += 1
            seen.add(key)

    radio_owners = {}
    for device in active["device"]:
        checks["active_device_identities"] += 1
        sn = device.get("serial_number")
        if not sn or not sn.strip():
            blockers["device:missing_serial"] += 1
        values = []
        for field, length in [("imei", 15), ("imei2", 15), ("eid", 32)]:
            value = device.get(field)
            if value:
                normalized = value.strip()
                if len(normalized) != length or not normalized.isascii() or not normalized.isdecimal():
                    blockers[f"device:{field}:invalid_digits"] += 1
                values.append(normalized)
                owner = radio_owners.get(normalized)
                if owner and owner != device["id"]:
                    blockers["device:cross_column_radio_conflict"] += 1
                radio_owners[normalized] = device["id"]
        if len(values) != len(set(values)):
            blockers["device:same_device_duplicate_radio"] += 1
        source = indexes["device_source"].get(device["source_id"])
        if source and source["location_id"] != device["location_id"]:
            observations["device:location_differs_from_source_location"] += 1

    reserved = defaultdict(Decimal)
    for row in active["reservation_item"]:
        key = (row["inventory_item_id"], row["location_id"])
        reserved[key] += Decimal(str(row["quantity"]))
    levels = {(r["inventory_item_id"], r["location_id"]): r for r in active["inventory_level"]}
    for key in reserved:
        checks["active_reservation_location_pairs"] += 1
        if key not in levels:
            blockers["reservation_item:no_active_level"] += 1
    for key, level in levels.items():
        checks["level_reserved_vs_reservations"] += 1
        if Decimal(str(level["reserved_quantity"])) != reserved[key]:
            blockers["inventory_level:reserved_total_differs_from_active_reservations"] += 1
        if Decimal(str(level["reserved_quantity"])) < 0:
            blockers["inventory_level:negative_reserved"] += 1
        if Decimal(str(level["stocked_quantity"])) < 0:
            observations["inventory_level:negative_stocked"] += 1
        if Decimal(str(level["stocked_quantity"])) < Decimal(str(level["reserved_quantity"])):
            observations["inventory_level:reserved_exceeds_stocked_check_backorder"] += 1

    for row in active["device_assignment"]:
        checks["assignment_order_line_ownership"] += 1
        # Source order_item contains order/version membership; order_line_item is the snapshot.
        membership = [m for m in tables["order_item"] if m["item_id"] == row["item_id"] and m["order_id"] == row["order_id"]]
        if not membership:
            blockers["device_assignment:line_not_in_order"] += 1
        original = indexes["device_assignment"].get(row.get("replacement_of"))
        if row.get("replacement_of"):
            checks["replacement_links"] += 1
            if not original or original["order_id"] != row["order_id"] or original["id"] == row["id"]:
                blockers["device_assignment:invalid_replacement_link"] += 1
        if row.get("after_sales_request_id"):
            request = indexes["after_sales_request"].get(row["after_sales_request_id"])
            if not request or request["order_id"] != row["order_id"]:
                blockers["device_assignment:after_sales_order_mismatch"] += 1

    for request in active["after_sales_request"]:
        checks["after_sales_envelopes"] += 1
        envelope = request["items"]
        if not isinstance(envelope, dict) or not isinstance(envelope.get("lines"), list):
            blockers["after_sales_request:invalid_items_envelope"] += 1
            continue
        seen = set()
        for line in envelope["lines"]:
            checks["after_sales_lines"] += 1
            item_id = line.get("item_id")
            if item_id in seen:
                blockers["after_sales_request:duplicate_item"] += 1
            seen.add(item_id)
            if not any(m["item_id"] == item_id and m["order_id"] == request["order_id"] for m in tables["order_item"]):
                blockers["after_sales_request:line_not_in_order"] += 1

    for table, field, targets in [("device", "source_id", set(indexes["device_source"])),
                                   ("reservation_item", "line_item_id", set(indexes["order_line_item"]))]:
        for row in tables[table]:
            if row.get(field) and row[field] not in targets:
                key = f"{table}.{field}:unresolved_{'deleted' if row.get('deleted_at') else 'active'}"
                (history if row.get("deleted_at") else blockers)[key] += 1
    report = {"format_version": 1, "bundle_sha256": manifest["bundle_sha256"],
              "scope": "Snapshot inventory/device semantic audit only; no source writes or MySQL acceptance; counts contain no business rows or identifiers",
              "checks": dict(sorted(checks.items())), "blocking_findings": dict(sorted(blockers.items())),
              "historical_unresolved_references": dict(sorted(history.items())), "observations": dict(sorted(observations.items())),
              "source_repair_performed": False,
              "disposition": "Preserve mismatches and unresolved history; no automatic quantity recomputation, fake sources or silent discard; isolated import must record issues and prevent affected new writes until reconciled"}
    target = ROOT / "docs/migration/inventory-device-semantic-report.json"
    target.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"checks": sum(checks.values()), "blocking_findings": dict(sorted(blockers.items())),
                      "historical_unresolved_references": sum(history.values()), "observations": dict(sorted(observations.items()))}))
    if blockers:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
